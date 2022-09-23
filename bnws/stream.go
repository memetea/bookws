package bnws

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/buger/jsonparser"
	"golang.org/x/exp/slices"
	"nhooyr.io/websocket"
)

const (
	BINANCE_SPOT_ENDPOINT      = "wss://stream.binance.com:9443/stream"
	BINANCE_SPOT_ENDPOINT_TEST = "wss://testnet.binance.vision/stream"

	BINANCE_FUTURE_ENDPOINT      = "wss://fstream.binance.com/stream"
	BINANCE_FUTURE_ENDPOINT_TEST = "wss://stream.binancefuture.com/stream"
	MaxRetryConn                 = 10
)

const (
	SUBSCRIBE_ID   = 1996
	UNSUBSCRIBE_ID = 1997
)

var (
	UseTestNet bool
)

func GetSpotWebsocketEndpoint() string {
	if UseTestNet {
		return BINANCE_SPOT_ENDPOINT_TEST
	}
	return BINANCE_SPOT_ENDPOINT
}

func GetFutureWebsocketEndpoint() string {
	if UseTestNet {
		return BINANCE_FUTURE_ENDPOINT_TEST
	}
	return BINANCE_FUTURE_ENDPOINT
}

type WsStream struct {
	sync.Mutex
	c           *websocket.Conn
	endPoint    string
	streams     []string
	reconnect   bool
	dataHandler func(msg []byte)
	metric      Metric
}

func NewWsStream(endPoint string, handler func(msg []byte)) *WsStream {
	stream := &WsStream{
		endPoint:    endPoint,
		dataHandler: handler,
		reconnect:   true,
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
			subscribes := s.streams
			s.streams = nil
			if err := s.Subscribe(subscribes); err != nil {
				s.streams = subscribes
				log.Printf("websocket subscribe after dial error:%v\n", err)
			}
			break
		}
	}
}

func (s *WsStream) dial() error {
	var err error
	url := s.endPoint
	if len(s.streams) > 0 {
		url = url + "?streams=" + strings.Join(s.streams, "/")
	}
	for i := 0; i < MaxRetryConn; i++ {
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
			Method string   `json:"method"`
			Params []string `json:"params"`
			Id     uint64   `json:"id"`
		}{
			Method: "SUBSCRIBE",
			Params: subscribes,
			Id:     SUBSCRIBE_ID,
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
			Method string   `json:"method"`
			Params []string `json:"params"`
			Id     uint64   `json:"id"`
		}{
			Method: "UNSUBSCRIBE",
			Params: unsubscribes,
			Id:     UNSUBSCRIBE_ID,
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
				s.c.Write(context.Background(), websocket.MessageText, []byte(`{
					"method": "LIST_SUBSCRIPTIONS",
					"id": 3
					}`))
			}

			var streams []string
			_, err = jsonparser.ArrayEach(message,
				func(value []byte, dataType jsonparser.ValueType, offset int, err error) {
					streams = append(streams, string(value))
				}, "result")
			if err == nil {
				s.streams = streams
			}
			continue
		}
		s.dataHandler(message[:totalN])
	}
}
