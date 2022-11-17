package bnws

import (
	"strconv"
	"strings"
	"time"

	"github.com/buger/jsonparser"
	"github.com/memetea/bookws"
	"golang.org/x/exp/slices"
)

//按Symbol的最优挂单信息
func NewFutureBookTickStream(dataHandler func(tick *bookws.BookTick), errorHandler func(err error)) *WsStream {
	var symbols []string
	tick := &bookws.BookTick{}
	return NewWsStream(GetFutureWebsocketEndpoint(), func(msg []byte) {
		stream, err := jsonparser.GetUnsafeString(msg, "stream")
		if err != nil {
			return
		}
		if !strings.HasSuffix(stream, "@bookTicker") &&
			!strings.HasSuffix(stream, "!bookTicker") {
			return
		}

		// tick := bookPool.Get().(*WsBookTick)
		tick.LocalTime = time.Now()
		err = jsonparser.ObjectEach(msg,
			func(key, value []byte, dataType jsonparser.ValueType, offset int) error {
				switch string(key) {
				case "s":
					//avoid memory allocation for every tick
					tick.Symbol = unsafeString(value)
					i, ok := slices.BinarySearch(symbols, tick.Symbol)
					if !ok {
						symbols = slices.Insert(symbols, i, string(value))
					}
					tick.Symbol = symbols[i]
				case "E":
					//avoid []byte -> string heap escape
					ms, err := strconv.ParseInt(unsafeString(value), 10, 64)
					if err != nil {
						return err
					}
					tick.ServerTime = time.Unix(ms/1000, ms%1000*1e6)
				case "b":
					tick.BidPrice = value
				case "B":
					tick.BidQuantity = value
				case "a":
					tick.AskPrice = value
				case "A":
					tick.AskQuantity = value
				}
				return nil
			}, "data")
		if err != nil {
			errorHandler(err)
			return
		}
		dataHandler(tick)
	})
}
