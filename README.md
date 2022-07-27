## binance spot and future market websocket wrapper

```
package main

import (
	"log"
	"os"
	"os/signal"
	"sync"
	"time"

	"github.com/memetea/bookws/bnws"
)

type Tick struct {
	Symbol   string
	BidPrice string
	BidVol   string
	AskPrice string
	AskVol   string
}

func main() {
	var t Tick
	bnws.UseTestNet = false
	ws := bnws.NewFutureBookTickStream(func(tick *bookws.BookTick) {
		if tick.Symbol == "BTCUSDT" {
			//caution: The four string(tick.BidPrice, tick.AskPrice, tick.BidQuantity, tick.AskQuantity)'s scope is this callback.
			//can't keep a reference to these string out of this scope.
			//if we use these strings out of this callback.  we can copy them out.
			//all this effort here is to reduce the memory allocation. so to reduce the gc overhead.
			tick.CopyOut(&t.BidPrice, &t.BidVol, &t.AskPrice, &t.AskVol)
			log.Printf("%s, %s, %s, %s\n", t.BidPrice, t.BidVol, t.AskPrice, t.AskVol)
		}
	}, func(err error) {
		log.Panic(err)
	})

	err := ws.Run()
	if err != nil {
		panic(err)
	}

	ws.Subscribe([]string{"btcusdt@bookTicker"})
	ws.Subscribe([]string{"ethusdt@bookTicker"})
	ws.Subscribe([]string{"!bookTicker"})

	go func() {
		time.Sleep(2 * time.Second)
		log.Printf("subscribed: %v", ws.CurrentSubscribes())
		ws.Subscribe([]string{"bnbusdt@bookTicker"})
		time.Sleep(2 * time.Second)
		log.Printf("subscribed: %v", ws.CurrentSubscribes())
		ws.Unsubscribe([]string{"bnbusdt@bookTicker"})
		time.Sleep(2 * time.Second)
		log.Printf("subscribed: %v", ws.CurrentSubscribes())
	}()
	metricChan := make(chan bnws.Metric)
	quit := make(chan struct{})
	go ws.Metric(5*time.Second, metricChan, quit)
	go func() {
		for m := range metricChan {
			kb := float64(m.BytesReceived) / 1024.0
			dur := time.Since(m.MetricBegin)
			kps := kb / dur.Seconds()
			log.Printf("bytes received: %f k,  speed: %f k/s\n", kb, kps)
		}
	}()
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	<-sig
	ws.Stop()
}


```
