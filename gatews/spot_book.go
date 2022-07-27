package gatews

import (
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

// //最优挂单
// type WsBookTick struct {
// 	Symbol    string
// 	LocalTime time.Time

// 	//server order book update time in milliseconds
// 	ServerTime time.Time

// 	//these four strings can't assign to other string.
// 	//they only alive during the callback.
// 	//call CopyString if need save a copy
// 	BidPrice    string
// 	BidQuantity string
// 	AskPrice    string
// 	AskQuantity string
// }

// func (t *WsBookTick) CopyString() (bp string, bq string, ap string, aq string) {
// 	bp = strClone(t.BidPrice)
// 	bq = strClone(t.BidQuantity)
// 	ap = strClone(t.AskPrice)
// 	aq = strClone(t.AskQuantity)
// 	return
// }

// func (t *WsBookTick) CopyOut(bp, bq, ap, aq *string) {
// 	strCpy(bp, t.BidPrice)
// 	strCpy(bq, t.BidQuantity)
// 	strCpy(ap, t.AskPrice)
// 	strCpy(aq, t.AskQuantity)
// }

func unsafeString(b []byte) string {
	return *(*string)(unsafe.Pointer(&b))
}

//按Symbol的最优挂单信息
func NewSpotBookTickStream(dataHandler func(tick *bookws.BookTick), errorHandler func(err error)) *WsStream {
	var symbols []string
	tick := &bookws.BookTick{}
	ws := NewWsStream(GetSpotWebsocketEndpoint(), ChannelSpotBookTicker, func(msg []byte) {
		channel, err := jsonparser.GetUnsafeString(msg, "channel")
		if err != nil {
			return
		}
		if channel != ChannelSpotBookTicker {
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
				case "t":
					//Book ticker generated timestamp in milliseconds
					ms, _ := jsonparser.ParseInt(value)
					tick.ServerTime = time.Unix(ms/1000, ms%1000*1e6)
				case "b":
					tick.BidPrice = unsafeString(value)
				case "B":
					tick.BidQuantity = unsafeString(value)
				case "a":
					tick.AskPrice = unsafeString(value)
				case "A":
					tick.AskQuantity = unsafeString(value)
				}
				return nil
			}, "result")
		if err != nil {
			errorHandler(err)
			return
		}
		dataHandler(tick)
	})
	return ws
}
