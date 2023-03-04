package myrpc

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"myrpc/codec"
	"net"
	"sync"
)

/*
rpc库对注册的方法有一定的限制，方法必须满足签名func (t *T) MethodName(argType T1, replyType *T2) error：

首先，方法必须是导出的（名字首字母大写）；
其次，方法接受两个参数，必须是导出的或内置类型。第一个参数表示客户端传递过来的请求参数，第二个是需要返回给客户端的响应。第二个参数必须为指针类型（需要修改）；
最后，方法必须返回一个error类型的值。返回非nil的值，表示调用出错。
*/

// Call 封装Call来承载一次RPC调用所需要的信息
type Call struct {
	Seq           uint64
	ServiceMethod string      //format "<service>.<method>"
	Args          interface{} //arguments to the function
	Reply         interface{} //reply from the function
	Error         error       //
	Done          chan *Call  //为支持异步调用，当调用结束时会调用call。done() 通知调用方
}

func (call *Call) done() {
	call.Done <- call
}

// Client 客户端核心Client的实现
type Client struct {
	cc      codec.Codec  //消息的解码器，与服务端类似，序列化发送的请求，反序列化接受的响应
	opt     *Option      //储存消息的解码方式
	sending sync.Mutex   // 这个互斥锁是为了保证请求的有序发送，防止多个请求报文混淆
	header  codec.Header //复用解码器的请求头，因为发送请求是互斥的，只有在发送请求时才需要
	mu      sync.Mutex
	seq     uint64           //发送请求的请求编号，每个请求编号是唯一的
	pending map[uint64]*Call //储存未处理完的请求 key是编号，值是Call实例
	//closing 和shutdown任一为true 表示Client不可用
	closing  bool //用户是否关闭连接？true一般为客户主动关闭
	shutdown bool //服务端是否通知关闭连接，true一般为错误发生
}

var ErrShutdown = errors.New("连接已经关闭！")

// Close 方法来关闭连接
func (client *Client) Close() error {
	client.mu.Lock()
	defer client.mu.Unlock()
	if client.closing {
		return ErrShutdown
	}
	client.closing = true
	return client.cc.Close()
}

// IsAvailable return true if the client does work 检测客户端是否可用
func (client *Client) IsAvailable() bool {
	client.mu.Lock()
	defer client.mu.Unlock()
	return !client.shutdown && !client.closing //任一为true表示为不可用
}

var _ io.Closer = (*Client)(nil) //检查Client是否实现Closer接口所有方法

// 将参数 call 添加到 client.pending 中，并更新 client.seq
func (client *Client) registerCall(call *Call) (uint64, error) {
	client.mu.Lock()
	defer client.mu.Unlock()
	if client.closing || client.shutdown {
		return 0, ErrShutdown
	}
	call.Seq = client.seq
	client.pending[call.Seq] = call
	client.seq++
	return call.Seq, nil
}

// 根据 seq，从 client.pending 中移除对应的 call，并返回。
func (client *Client) removeCall(seq uint64) *Call {
	client.mu.Lock()
	defer client.mu.Unlock()
	call := client.pending[seq]
	delete(client.pending, seq)
	return call
}

// 服务端或客户端发生错误时调用，将 shutdown 设置为 true，且将错误信息通知所有 pending 状态的 call
func (client *Client) terminateCalls(err error) {
	client.sending.Lock()
	defer client.sending.Unlock()
	client.mu.Lock()
	defer client.mu.Unlock()
	client.shutdown = true
	for _, call := range client.pending {
		call.Error = err
		call.done()
	}
}

// 接受响应的三种情况
func (client *Client) receive() {
	var err error
	for err == nil {
		var h codec.Header
		if err = client.cc.ReadHeader(&h); err != nil {
			break
		}
		call := client.removeCall(h.Seq)
		switch {
		case call == nil:
			// it usually means that Write partially failed
			// and call was already removed.
			err = client.cc.ReadBody(nil)
		case h.Error != "":
			call.Error = fmt.Errorf(h.Error)
			err = client.cc.ReadBody(nil)
			call.done()
		default:
			err = client.cc.ReadBody(call.Reply)
			if err != nil {
				call.Error = errors.New("reading body " + err.Error())
			}
			call.done()
		}
	}
	// error occurs, so terminateCalls pending calls
	client.terminateCalls(err)
}

// NewClient 创建Client实例-->协议交换（发送Option给服务端），
func NewClient(conn net.Conn, opt *Option) (*Client, error) {
	f := codec.NewCodecFuncMap[opt.CodecType]
	if f == nil {
		err := fmt.Errorf("invalid codec type %s", opt.CodecType)
		log.Println("rpc client: codec error:", err)
		return nil, err
	}
	// send options with server
	if err := json.NewEncoder(conn).Encode(opt); err != nil {
		log.Println("rpc client: options error: ", err)
		_ = conn.Close()
		return nil, err
	}
	return newClientCodec(f(conn), opt), nil
}

// 协商好Option-->创建子协程receive接收响应
func newClientCodec(cc codec.Codec, opt *Option) *Client {
	client := &Client{
		seq:     1, // seq starts with 1, 0 means invalid call
		cc:      cc,
		opt:     opt,
		pending: make(map[uint64]*Call),
	}
	go client.receive()
	return client
}

func parseOptions(opts ...*Option) (*Option, error) {
	// if opts is nil or pass nil as parameter
	if len(opts) == 0 || opts[0] == nil {
		return DefaultOption, nil
	}
	if len(opts) != 1 {
		return nil, errors.New("number of options is more than 1")
	}
	opt := opts[0]
	opt.MagicNumber = DefaultOption.MagicNumber
	if opt.CodecType == "" {
		opt.CodecType = DefaultOption.CodecType
	}
	return opt, nil
}

// Dial connects to an RPC server at the specified network address实现Dial函数
func Dial(network, address string, opts ...*Option) (client *Client, err error) { //简化用户调用，将Option实现为可选参数
	opt, err := parseOptions(opts...)
	if err != nil {
		return nil, err
	}
	conn, err := net.Dial(network, address)
	if err != nil {
		return nil, err
	}
	// close the connection if client is nil
	defer func() {
		if client == nil {
			_ = conn.Close()
		}
	}()
	return NewClient(conn, opt)
}

// 实现发送请求功能
func (client *Client) send(call *Call) {
	// make sure that the client will send a complete request
	client.sending.Lock()
	defer client.sending.Unlock()

	// register this call.
	seq, err := client.registerCall(call)
	if err != nil {
		call.Error = err
		call.done()
		return
	}

	// prepare request header
	client.header.ServiceMethod = call.ServiceMethod
	client.header.Seq = seq
	client.header.Error = ""

	// encode and send the request
	if err := client.cc.Write(&client.header, call.Args); err != nil {
		call := client.removeCall(seq)
		// call may be nil, it usually means that Write partially failed,
		// client has received the response and handled
		if call != nil {
			call.Error = err
			call.done()
		}
	}
}

// Go invokes the function asynchronously.
// 异步接口返回call实例
// It returns the Call structure representing the invocation.
func (client *Client) Go(serviceMethod string, args, reply interface{}, done chan *Call) *Call {
	if done == nil {
		done = make(chan *Call, 10)
	} else if cap(done) == 0 {
		log.Panic("rpc client: done channel is unbuffered")
	}
	call := &Call{
		ServiceMethod: serviceMethod,
		Args:          args,
		Reply:         reply,
		Done:          done,
	}
	client.send(call)
	return call
}

// Call invokes the named function, waits for it to complete,
// and returns its error status.
// call是对Go方法的封装，阻塞call.Done 等待响应返回，为同步接口
func (client *Client) Call(serviceMethod string, args, reply interface{}) error {
	call := <-client.Go(serviceMethod, args, reply, make(chan *Call, 1)).Done
	return call.Error
}
