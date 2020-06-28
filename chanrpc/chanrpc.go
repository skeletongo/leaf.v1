package chanrpc

import (
	"errors"
	"fmt"
	"runtime"

	"github.com/skeletongo/leaf.v1/conf"
	"github.com/skeletongo/leaf.v1/log"
)

// Server 服务端
// 方法注册，通过消息处理方法
type Server struct {
	// 支持的方法：
	// func([]interface{})
	// func([]interface{}) interface{}
	// func([]interface{}) []interface{}
	functions map[interface{}]interface{}

	// 待处理消息队列
	ChanCall chan *CallInfo
}

// CallInfo 消息体
type CallInfo struct {
	f       interface{}   // 方法体
	args    []interface{} // 参数
	chanRet chan *RetInfo // 结果接收通道
	cb      interface{}   // 回调方法
}

// RetInfo 消息处理结果
type RetInfo struct {
	ret interface{} // 返回结果
	err error       // 错误
	cb  interface{} // 回调方法
}

// NewServer 创建服务端
// l 待处理消息队列长度
func NewServer(l int) *Server {
	return &Server{
		functions: make(map[interface{}]interface{}),
		ChanCall:  make(chan *CallInfo, l),
	}
}

// Register 注册处理方法
func (s *Server) Register(id interface{}, f interface{}) {
	if _, ok := s.functions[id]; ok {
		panic(fmt.Sprintf("function id %v: already registered", id))
	}
	switch f.(type) {
	case func([]interface{}):
	case func([]interface{}) interface{}:
	case func([]interface{}) []interface{}:
	default:
		panic(fmt.Sprintf("function id %v: invalid", id))
	}
	s.functions[id] = f
}

func assert(i interface{}) []interface{} {
	if i == nil {
		return nil
	}
	if ret, ok := i.([]interface{}); ok {
		return ret
	}
	return []interface{}{i}
}

// 将返回结果发送给接收队列
func ret(ci *CallInfo, ri *RetInfo) (err error) {
	if ci.chanRet == nil {
		return
	}
	ri.cb = ci.cb

	// chanRet 接收通道可能已经关闭
	defer func() {
		if r := recover(); r != nil {
			err = r.(error)
		}
	}()

	ci.chanRet <- ri
	return
}

// 执行队列中的方法
// 执行出错也通知客户端
// err 方法执行抛异常、接收通道已经关闭
func exec(ci *CallInfo) (err error) {
	// 异常恢复
	defer func() {
		if r := recover(); r != nil {
			if conf.LenStackBuf > 0 {
				buf := make([]byte, conf.LenStackBuf)
				l := runtime.Stack(buf, false)
				err = fmt.Errorf("%v: %s", r, buf[:l])
			} else {
				err = fmt.Errorf("%v", r)
			}

			_ = ret(ci, &RetInfo{err: fmt.Errorf("%v", r)})
		}
	}()

	switch f := ci.f.(type) {
	case func([]interface{}):
		f(ci.args)
		return ret(ci, &RetInfo{})
	case func([]interface{}) interface{}:
		return ret(ci, &RetInfo{ret: f(ci.args)})
	case func([]interface{}) []interface{}:
		return ret(ci, &RetInfo{ret: f(ci.args)})
	}
	panic("bug")
}

func (s *Server) Exec(ci *CallInfo) {
	if err := exec(ci); err != nil {
		log.Error("%v", err)
	}
}

// Close 停止消息接收，处理完已经入队列的消息后关闭
func (s *Server) Close() {
	close(s.ChanCall)
	for v := range s.ChanCall {
		//_ = ret(v, &RetInfo{err: errors.New("channel rpc server closed")})
		s.Exec(v)
	}
}

// Go 异步处理
// 线程安全
func (s *Server) Go(id interface{}, args ...interface{}) {
	f := s.functions[id]
	if f == nil {
		return
	}

	defer func() {
		recover()
	}()

	s.ChanCall <- &CallInfo{
		f:    f,
		args: args,
	}
}

func (s *Server) Call0(id interface{}, args ...interface{}) error {
	return s.Open(0).Call0(id, args...)
}

func (s *Server) Call1(id interface{}, args ...interface{}) (interface{}, error) {
	return s.Open(0).Call1(id, args...)
}

func (s *Server) CallN(id interface{}, args ...interface{}) ([]interface{}, error) {
	return s.Open(0).CallN(id, args...)
}

// 创建客户端
func (s *Server) Open(l int) *Client {
	c := NewClient(l)
	c.Attach(s)
	return c
}

// Client 客户端
type Client struct {
	s            *Server       // 服务端
	ChanSyncRet  chan *RetInfo // 同步接收通道
	ChanAsyncRet chan *RetInfo // 异步接收通道
	pendingAsync int           // 待处理异步消息数量
}

func NewClient(l int) *Client {
	return &Client{
		ChanSyncRet:  make(chan *RetInfo, 1),
		ChanAsyncRet: make(chan *RetInfo, l),
	}
}

func (c *Client) Attach(s *Server) {
	c.s = s
}

func (c *Client) f(id interface{}, n int) (f interface{}, err error) {
	if c.s == nil {
		err = errors.New("server not attached")
		return
	}
	f = c.s.functions[id]
	if f == nil {
		err = fmt.Errorf("function id %v: function not registered", id)
		return
	}
	var ok bool
	switch n {
	case 0:
		_, ok = f.(func([]interface{}))
	case 1:
		_, ok = f.(func([]interface{}) interface{})
	case 2:
		_, ok = f.(func([]interface{}) []interface{})
	default:
		panic("bug")
	}
	if !ok {
		err = fmt.Errorf("function id %v: return type mismatch", id)
	}
	return
}

// block 服务端如果队列已满，是否阻塞
func (c *Client) call(ci *CallInfo, block bool) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = r.(error)
		}
	}()

	if block {
		c.s.ChanCall <- ci
		return
	}
	select {
	case c.s.ChanCall <- ci:
	default:
		err = errors.New("channel rpc server channel full")
	}
	return
}

/*
 同步调用: Call0 Call1 CallN
 线程安全
*/
func (c *Client) Call0(id interface{}, args ...interface{}) error {
	f, err := c.f(id, 0)
	if err != nil {
		return err
	}

	err = c.call(&CallInfo{
		f:       f,
		args:    args,
		chanRet: c.ChanSyncRet,
	}, true)
	if err != nil {
		return err
	}

	ri := <-c.ChanSyncRet
	return ri.err
}

func (c *Client) Call1(id interface{}, args ...interface{}) (interface{}, error) {
	f, err := c.f(id, 1)
	if err != nil {
		return nil, err
	}

	err = c.call(&CallInfo{
		f:       f,
		args:    args,
		chanRet: c.ChanSyncRet,
	}, true)
	if err != nil {
		return nil, err
	}

	ri := <-c.ChanSyncRet
	return ri.ret, ri.err
}

func (c *Client) CallN(id interface{}, args ...interface{}) ([]interface{}, error) {
	f, err := c.f(id, 2)
	if err != nil {
		return nil, err
	}

	err = c.call(&CallInfo{
		f:       f,
		args:    args,
		chanRet: c.ChanSyncRet,
	}, true)
	if err != nil {
		return nil, err
	}

	ri := <-c.ChanSyncRet
	return assert(ri.ret), ri.err
}

func (c *Client) asyncCall(id interface{}, args []interface{}, cb interface{}, n int) {
	f, err := c.f(id, n)
	if err != nil {
		c.ChanAsyncRet <- &RetInfo{err: err, cb: cb}
		return
	}

	err = c.call(&CallInfo{
		f:       f,
		args:    args,
		chanRet: c.ChanAsyncRet,
		cb:      cb,
	}, false)
	if err != nil {
		c.ChanAsyncRet <- &RetInfo{err: err, cb: cb}
		return
	}
}

// AsyncCall 异步回调
func (c *Client) AsyncCall(id interface{}, args ...interface{}) {
	if len(args) < 1 {
		panic("callback function not found")
	}

	cb := args[len(args)-1]

	var n int
	switch cb.(type) {
	case func(error):
	case func(interface{}, error):
		n = 1
	case func([]interface{}, error):
		n = 2
	default:
		panic("definition of callback function is invalid")
	}
	// 如果异步返回队列已满，直接返回错误信息，防止服务端阻塞
	if c.pendingAsync >= cap(c.ChanAsyncRet) {
		execCb(&RetInfo{err: errors.New("too many calls"), cb: cb})
		return
	}

	c.asyncCall(id, args[:len(args)-1], cb, n)
	c.pendingAsync++
}

func execCb(ri *RetInfo) {
	// 回调异常捕获
	defer func() {
		if r := recover(); r != nil {
			if conf.LenStackBuf > 0 {
				buf := make([]byte, conf.LenStackBuf)
				l := runtime.Stack(buf, false)
				log.Error("%v: %s", r, buf[:l])
			} else {
				log.Error("%v", r)
			}
		}
	}()

	switch f := ri.cb.(type) {
	case func(error):
		f(ri.err)
	case func(interface{}, error):
		f(ri.ret, ri.err)
	case func([]interface{}, error):
		f(assert(ri.ret), ri.err)
	default:
		panic("bug")
	}
}

// Cb 回调处理
func (c *Client) Cb(ri *RetInfo) {
	c.pendingAsync--
	execCb(ri)
}

// Close 等待所有异步调用执行后，关闭客户端
func (c *Client) Close() {
	for c.pendingAsync > 0 {
		c.Cb(<-c.ChanAsyncRet)
	}
}

// Idle 是否空闲
func (c *Client) Idle() bool {
	return c.pendingAsync == 0
}
