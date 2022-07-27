## binance spot and future market websocket wrapper

```
package main

import (
	"log"
	"os"
	"os/signal"
	"sync"
	"time"

	"github.com/memetea/bnws"
)

var pool = &sync.Pool{
	New: func() any {
		return new(bnws.WsBookTick)
	},
}

func main() {
	log.SetFlags(log.Lmicroseconds)
	bnws.UseTestNet = true
	ws := bnws.NewFutureBookTickStream(func(tick *bnws.WsBookTick) {
		if tick.Symbol == "BNBUSDT" {
			log.Printf("%+v\n", tick)
		}
		pool.Put(tick)
	}, func(err error) {
		log.Panic(err)
	}, pool)

	err := ws.Run()
	if err != nil {
		panic(err)
	}

	ws.Subscribe([]string{"btcusdt@bookTicker"})
	ws.Subscribe([]string{"ethusdt@bookTicker"})
	//ws.Subscribe([]string{"!bookTicker"})

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
	close(quit)
	ws.Stop()
}


```
