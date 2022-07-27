package gatews

import (
	"context"
	"encoding/json"
	"io"
	"log"
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
	SUBSCRIBE_ID   = 1996
	UNSUBSCRIBE_ID = 1997
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
	endPoint  string
	channel   string
	streams   []string
	reconnect bool
	// cancelFunc  context.CancelFunc
	dataHandler func(msg []byte)

	metric Metric
	// dataMetricStart time.Time
	// //killo bytes
	// dataMetricReceived int64
	c *websocket.Conn
}

func NewWsStream(endPoint string, channel string, handler func(msg []byte)) *WsStream {
	stream := &WsStream{
		endPoint:    endPoint,
		channel:     channel,
		dataHandler: handler,
		reconnect:   true,
	}
	return stream
}

func (s *WsStream) handleError(err error) {
	if s.reconnect {
		// s.cancelFunc()
		for s.reconnect {
			err := s.dial()
			if err != nil {
				time.Sleep(3 * time.Second)
				continue
			}
			break
		}
	}
}

func (s *WsStream) dial() error {
	var err error
	for i := 0; i < MaxRetryConn; i++ {
		url := s.endPoint
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
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
			Id      int64    `json:"id"`
			Channel string   `json:"channel"`
			Event   string   `json:"event"`
			Payload []string `json:"payload"`
		}{
			Time:    time.Now().Unix(),
			Id:      SUBSCRIBE_ID,
			Channel: s.channel,
			Event:   "subscribe",
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
	var unsubscribes []string
	var remain []string
	for _, stream := range streams {
		if slices.Index(s.streams, stream) >= 0 {
			unsubscribes = append(unsubscribes, stream)
		} else {
			remain = append(remain, stream)
		}
	}
	if len(unsubscribes) > 0 {
		request, err := json.Marshal(struct {
			Time    int64    `json:"time"`
			Id      int64    `json:"id"`
			Channel string   `json:"channel"`
			Event   string   `json:"event"`
			Payload []string `json:"payload"`
		}{
			Time:    time.Now().Unix(),
			Id:      UNSUBSCRIBE_ID,
			Channel: s.channel,
			Event:   "unsubscribe",
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
		id, err := jsonparser.GetInt(message, "id")
		if err == nil {
			if id == SUBSCRIBE_ID || id == UNSUBSCRIBE_ID {
				errorMsg, err := jsonparser.GetString(message, "message", "error", "message")
				if err == nil && len(errorMsg) > 0 {
					log.Printf("error: id=%d, msg=%s\n", id, errorMsg)
				}
			}
		}
		s.dataHandler(message[:totalN])
	}
}
