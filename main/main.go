package main

import (
	"fmt"
	"log"
	"myrpc"
	"net"
	"sync"
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
	/* day1
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
	}*/
	log.SetFlags(0)
	addr := make(chan string)
	go startServer(addr)
	client, _ := myrpc.Dial("tcp", <-addr)
	defer func() { _ = client.Close() }()

	time.Sleep(time.Second)
	// send request & receive response
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			args := fmt.Sprintf("geerpc req %d", i)
			var reply string
			if err := client.Call("Foo.Sum", args, &reply); err != nil {
				log.Fatal("call Foo.Sum error:", err)
			}
			log.Println("reply:", reply)
		}(i)
		go func(i int) {

		}(i)
	}
	wg.Wait()
}
