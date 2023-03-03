package codec

import (
	"bufio"
	"encoding/gob"
	"io"
	"log"
)

//Gob 由发送端使用 Encoder 对数据结构进行编码。在接收端收到消息之后，接收端使用 Decoder 将序列化的数据变化成本地变量。

type GobCodec struct {
	conn io.ReadWriteCloser //构建函数传入，通常通过
	buf  *bufio.Writer      //防止阻塞的带缓冲Writer
	dec  *gob.Decoder       //对应Decoder和Encoder
	enc  *gob.Encoder
}

// 确保某个类型实现了某个接口的所有方法
// 将空值转化成*GobCodec类型，再转换为Codec接口，如果转换失败说明gob没有实现接口的所有方法
var _ Codec = (*GobCodec)(nil)

// NewGobCodec GobCodec的构造函数
func NewGobCodec(conn io.ReadWriteCloser) Codec {
	buf := bufio.NewWriter(conn)
	return &GobCodec{
		conn: conn,
		buf:  buf,
		dec:  gob.NewDecoder(conn),
		enc:  gob.NewEncoder(buf),
	}
}

//下面是接口方法的实现

func (c *GobCodec) ReadHeader(h *Header) error {
	return c.dec.Decode(h)
}

func (c *GobCodec) ReadBody(body interface{}) error {
	return c.dec.Decode(body)
}

func (c *GobCodec) Write(h *Header, body interface{}) (err error) {
	defer func() {
		_ = c.buf.Flush()
		if err != nil {
			_ = c.Close()
		}
	}()
	if err = c.enc.Encode(h); err != nil {
		log.Println("rpc: gob error encoding header:", err)
		return
	}
	if err = c.enc.Encode(body); err != nil {
		log.Println("rpc: gob error encoding body:", err)
		return
	}
	return
}

func (c *GobCodec) Close() error {
	return c.conn.Close()
}
