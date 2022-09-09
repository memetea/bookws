package gatews

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/buger/jsonparser"
	"golang.org/x/exp/slices"
	"nhooyr.io/websocket"
)

const (
	SpotBaseUrl = "wss://api.gateio.ws/ws/v4/"

	FuturesUsdtUrl     = "wss://fx-ws.gateio.ws/v4/ws/usdt"
	FuturesUsdtTestNet = "wss://fx-ws-testnet.gateio.ws/v4/ws/usdt"
	MaxRetryConn       = 10
)

// spot channels
const (
	ChannelSpotBookTicker   = "spot.book_ticker"
	ChannelFutureBookTicker = "futures.book_ticker"
)

const (
	EVENT_SUBSCRIBE   = "subscribe"
	EVENT_UNSUBSCRIBE = "unsubscribe"
	EVENT_UPDATE      = "update"
)

var UseTestNet bool = false

func GetSpotWebsocketEndpoint() string {
	return SpotBaseUrl
}

func GetFutureWebsocketEndpoint() string {
	if UseTestNet {
		return FuturesUsdtTestNet
	}
	return FuturesUsdtUrl
}

type WsStream struct {
	sync.Mutex
	c            *websocket.Conn
	endPoint     string
	channel      string
	streams      []string
	reconnect    bool
	dataHandler  func(msg []byte)
	errorHandler func(msg string)

	metric Metric
}

func NewWsStream(endPoint string, channel string, dataHandler func(msg []byte), errorHandler func(msg string)) *WsStream {
	stream := &WsStream{
		endPoint:     endPoint,
		channel:      channel,
		dataHandler:  dataHandler,
		errorHandler: errorHandler,
		reconnect:    true,
	}
	return stream
}

func (s *WsStream) handleError(err error) {
	log.Printf("websocket error: %v\n", err)
	s.c = nil
	if len(s.streams) > 0 {
		for s.reconnect {
			err := s.dial()
			if err != nil {
				log.Printf("websocket dial: %v\n", err)
				continue
			}
			if err := s.Subscribe(s.streams); err != nil {
				log.Printf("websocket subscribe after dial: %v\n", err)
			}
			break
		}
	}
}

func (s *WsStream) dial() error {
	var err error
	for i := 0; i < MaxRetryConn; i++ {
		url := s.endPoint
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		s.c, _, err = websocket.Dial(ctx, url, nil)
		if err != nil {
			time.Sleep(time.Duration(i+1) * time.Second)
			continue
		}
		break
	}
	if err != nil {
		return err
	}
	s.c.SetReadLimit(655350)
	go s.pump()
	return nil
}

func (s *WsStream) Run() error {
	if s.c != nil {
		return nil
	}
	s.reconnect = true
	return s.dial()
}

type Metric struct {
	MetricBegin   time.Time
	TimeElapsed   time.Duration
	BytesReceived uint64
}

func (s *WsStream) Metric(duration time.Duration, out chan Metric, quit chan struct{}) {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	metricBeginTime := time.Now()
	metricBeginReceived := s.metric.BytesReceived
	for {
		select {
		case <-quit:
			return
		case <-timer.C:
			bytesReceived := atomic.LoadUint64(&s.metric.BytesReceived)
			out <- Metric{
				MetricBegin:   metricBeginTime,
				TimeElapsed:   time.Since(metricBeginTime),
				BytesReceived: bytesReceived - metricBeginReceived,
			}
			timer.Reset(duration)
		}
	}
}

func (s *WsStream) Stop() {
	s.reconnect = false
	if s.c != nil {
		s.c.Close(websocket.StatusNormalClosure, "")
		s.c = nil
	}
}

func (s *WsStream) Subscribe(streams []string) error {
	s.Lock()
	defer s.Unlock()
	if err := s.Run(); err != nil {
		return err
	}
	var subscribes []string
	for _, stream := range streams {
		i := slices.Index(s.streams, stream)
		if i < 0 {
			subscribes = append(subscribes, stream)
		}
	}
	if len(subscribes) > 0 {
		request, err := json.Marshal(struct {
			Time    int64    `json:"time"`
			Channel string   `json:"channel"`
			Event   string   `json:"event"`
			Payload []string `json:"payload"`
		}{
			Time:    time.Now().Unix(),
			Channel: s.channel,
			Event:   EVENT_SUBSCRIBE,
			Payload: streams,
		})
		if err != nil {
			return err
		}
		err = s.c.Write(context.Background(), websocket.MessageText, request)
		if err != nil {
			return err
		}
		s.streams = append(s.streams, subscribes...)
	}
	return nil
}

func (s *WsStream) Unsubscribe(streams []string) error {
	s.Lock()
	defer s.Unlock()
	var unsubscribes []string
	var remain []string
	for _, stream := range s.streams {
		if slices.Index(streams, stream) >= 0 {
			unsubscribes = append(unsubscribes, stream)
		} else {
			remain = append(remain, stream)
		}
	}
	if len(unsubscribes) > 0 {
		request, err := json.Marshal(struct {
			Time    int64    `json:"time"`
			Channel string   `json:"channel"`
			Event   string   `json:"event"`
			Payload []string `json:"payload"`
		}{
			Time:    time.Now().Unix(),
			Channel: s.channel,
			Event:   EVENT_UNSUBSCRIBE,
			Payload: streams,
		})
		if err != nil {
			return err
		}
		err = s.c.Write(context.Background(), websocket.MessageText, request)
		if err != nil {
			return err
		}
		s.streams = remain
	}
	return nil
}

func (s *WsStream) CurrentSubscribes() []string {
	return s.streams
}

func (s *WsStream) pump() {
	s.metric.MetricBegin = time.Now()
	message := make([]byte, 4096)
	for {
		msgType, r, err := s.c.Reader(context.Background())
		if err != nil {
			if s.reconnect {
				s.handleError(err)
			}
			return
		}
		if msgType != websocket.MessageText {
			panic("not text message")
		}
		var totalN int
		for {
			n, err := r.Read(message[totalN:])
			totalN += n
			if err != nil {
				if err == io.EOF {
					break
				}
				if s.reconnect {
					s.handleError(err)
				}
				return
			}
		}
		if totalN == 0 {
			continue
		}
		atomic.AddUint64(&s.metric.BytesReceived, uint64(totalN))
		if errorMsg, err := jsonparser.GetString(message[:totalN], "error", "message"); err == nil {
			s.errorHandler(errorMsg)
			continue
		}
		s.dataHandler(message[:totalN])
	}
}
