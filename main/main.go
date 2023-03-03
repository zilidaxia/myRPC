package main

import (
	"encoding/json"
	"fmt"
	"log"
	"myrpc"
	"myrpc/codec"
	"net"
	"time"
)

// 使用了信道 addr，确保服务端端口监听成功，客户端再发起请求。
func startServer(addr chan string) {
	l, err := net.Listen("tcp", ":0")
	if err != nil {
		log.Fatal("network error:", err)
	}
	log.Println("开启rpc服务", l.Addr())
	addr <- l.Addr().String()
	myrpc.Accept(l)
}

func main() {
	//设置打印时间格式
	log.SetFlags(1)
	addr := make(chan string)
	go startServer(addr)
	//Dial connects to the address on the named network.
	conn, _ := net.Dial("tcp", <-addr)
	//到时自动关闭连接
	defer func() { _ = conn.Close() }()

	time.Sleep(time.Second)

	_ = json.NewEncoder(conn).Encode(myrpc.DefaultOption)
	cc := codec.NewGobCodec(conn)
	for i := 0; i < 5; i++ {
		h := &codec.Header{
			ServiceMethod: "Foo.Sum",
			Seq:           uint64(i),
		}
		_ = cc.Write(h, fmt.Sprintf("myrpc req %d", h.Seq))
		_ = cc.ReadHeader(h)
		var reply string
		_ = cc.ReadBody(&reply)
		log.Println("reply:", reply)
	}
}
