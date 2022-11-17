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

	BidPrice    []byte
	BidQuantity []byte
	AskPrice    []byte
	AskQuantity []byte
}

func (t *BookTick) UnsafeBidPrice() string {
	return unsafeString(t.BidPrice)
}

func (t *BookTick) SafeBidPrice() string {
	return string(t.BidPrice)
}

func (t *BookTick) UnsafeBidQuantity() string {
	return unsafeString(t.BidQuantity)
}

func (t *BookTick) SafeBidQuantity() string {
	return string(t.BidQuantity)
}

func (t *BookTick) UnsafeAskPrice() string {
	return unsafeString(t.AskPrice)
}

func (t *BookTick) SafeAskPrice() string {
	return string(t.AskPrice)
}

func (t *BookTick) UnsafeAskQuantity() string {
	return unsafeString(t.AskQuantity)
}

func (t *BookTick) SafeAskQuantity() string {
	return string(t.AskQuantity)
}

func unsafeString(b []byte) string {
	sliceHeader := (*reflect.SliceHeader)(unsafe.Pointer(&b))
	stringHeader := reflect.StringHeader{Data: sliceHeader.Data, Len: sliceHeader.Len}
	return *(*string)(unsafe.Pointer(&stringHeader))
}
