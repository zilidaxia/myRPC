package codec

import "io"

type Header struct { //  将请求和响应中的参数和返回值抽象为body，剩余信息放在header中
	ServiceMethod string // 服务名和方法名，通常与Go语言中结构体和方法相映射
	Seq           uint64 // 请求的序号，也可以认为是请求id，区分不同请求
	Error         string // 错误信息
}

// Codec 抽象出对消息体进行编解码的接口Codec
type Codec interface {
	io.Closer
	ReadHeader(*Header) error
	ReadBody(interface{}) error
	Write(*Header, interface{}) error
}

// NewCodecFunc 抽象出codec的构造函数

type NewCodecFunc func(closer io.ReadWriteCloser) Codec

type Type string

const (
	GobType  Type = "application/gob"
	JsonType Type = "application/json" //未实现
)

var NewCodecFuncMap map[Type]NewCodecFunc

func init() {
	NewCodecFuncMap = make(map[Type]NewCodecFunc)
	NewCodecFuncMap[GobType] = NewGobCodec
}
