package bnws

import (
	"strings"
	"time"
	"unsafe"

	"github.com/buger/jsonparser"
	"github.com/memetea/bookws"
	"golang.org/x/exp/slices"
)

type WsStreamData struct {
	Stream string `json:"stream"`
	Data   any
}

// //alloc new memory, and copy s to new allocated string
// func strClone(s string) string {
// 	//s will heap escape.
// 	//this will alloc memory and copy s to new string.
// 	return string([]byte(s))
// }

// //copy src to dst if dst has enough space. otherwise make a clone
// func strCpy(dst *string, src string) {
// 	if len(*dst) >= len(src) {
// 		dstBytes := unsafeBytes(*dst)
// 		copy(dstBytes, src)
// 		(*reflect.StringHeader)(unsafe.Pointer(dst)).Len = len(src)
// 	} else {
// 		*dst = string([]byte(src))
// 	}
// }

func unsafeString(b []byte) string {
	return *(*string)(unsafe.Pointer(&b))
}

func unsafeBytes(s string) []byte {
	return *(*[]byte)(unsafe.Pointer(&s))
}

//按Symbol的最优挂单信息
func NewSpotBookTickStream(dataHandler func(tick *bookws.BookTick), errorHandler func(err error)) *WsStream {
	var symbols []string
	tick := &bookws.BookTick{}
	return NewWsStream(GetSpotWebsocketEndpoint(), func(msg []byte) {
		stream, err := jsonparser.GetUnsafeString(msg, "stream")
		if err != nil {
			return
		}
		if !strings.HasSuffix(stream, "@bookTicker") &&
			!strings.HasSuffix(stream, "!bookTicker") {
			return
		}

		tick.LocalTime = time.Now()
		err = jsonparser.ObjectEach(msg,
			func(key, value []byte, dataType jsonparser.ValueType, offset int) error {
				switch string(key) {
				case "s":
					tick.Symbol = unsafeString(value)
					i, ok := slices.BinarySearch(symbols, tick.Symbol)
					if !ok {
						symbols = slices.Insert(symbols, i, string(value))
					}
					tick.Symbol = symbols[i]
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
