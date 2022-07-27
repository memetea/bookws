package bookws

import (
	"reflect"
	"time"
	"unsafe"
)

// type BookStream interface {
// 	Subscribe(symbols []string) error
// 	UnSubscribe(symbols []string) error
// }

//最优挂单
type BookTick struct {
	Symbol     string
	LocalTime  time.Time
	ServerTime time.Time

	//these four strings can't assign to other string.
	//they only alive during the callback.
	//call CopyString if need save a copy
	BidPrice    string
	BidQuantity string
	AskPrice    string
	AskQuantity string
}

func (t *BookTick) CopyString() (bp string, bq string, ap string, aq string) {
	bp = strClone(t.BidPrice)
	bq = strClone(t.BidQuantity)
	ap = strClone(t.AskPrice)
	aq = strClone(t.AskQuantity)
	return
}

func (t *BookTick) CopyOut(bp, bq, ap, aq *string) {
	strCpy(bp, t.BidPrice)
	strCpy(bq, t.BidQuantity)
	strCpy(ap, t.AskPrice)
	strCpy(aq, t.AskQuantity)
}

//alloc new memory, and copy s to new allocated string
func strClone(s string) string {
	//s will heap escape.
	//this will alloc memory and copy s to new string.
	return string([]byte(s))
}

//copy src to dst if dst has enough space. otherwise make a clone
func strCpy(dst *string, src string) {
	if len(*dst) >= len(src) {
		dstBytes := unsafeBytes(*dst)
		copy(dstBytes, src)
		(*reflect.StringHeader)(unsafe.Pointer(dst)).Len = len(src)
	} else {
		*dst = string([]byte(src))
	}
}

func unsafeBytes(s string) []byte {
	return *(*[]byte)(unsafe.Pointer(&s))
}
