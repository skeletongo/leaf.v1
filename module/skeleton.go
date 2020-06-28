package module

import (
	"time"

	"github.com/skeletongo/leaf.v1/chanrpc"
	"github.com/skeletongo/leaf.v1/console"
	g "github.com/skeletongo/leaf.v1/go"
	"github.com/skeletongo/leaf.v1/log"
	"github.com/skeletongo/leaf.v1/timer"
)

type Skeleton struct {
	// go
	GoLen int
	g     *g.Go

	// timer dispatcher
	TimerDispatcherLen int
	dispatcher         *timer.Dispatcher

	// channel rpc client
	AsyncCallLen int
	client       *chanrpc.Client

	ChanRPCServer *chanrpc.Server
	commandServer *chanrpc.Server
}

func (s *Skeleton) Init() {
	if s.GoLen <= 0 {
		s.GoLen = 0
	}
	if s.TimerDispatcherLen <= 0 {
		s.TimerDispatcherLen = 0
	}
	if s.AsyncCallLen <= 0 {
		s.AsyncCallLen = 0
	}

	s.g = g.New(s.GoLen)
	s.dispatcher = timer.NewDispatcher(s.TimerDispatcherLen)
	s.client = chanrpc.NewClient(s.AsyncCallLen)
	if s.ChanRPCServer == nil {
		log.Release("invalid ChanRPCServer, default channel len 100")
		s.ChanRPCServer = chanrpc.NewServer(100)
	}
	s.commandServer = chanrpc.NewServer(0)
}

func (s *Skeleton) Run(closeSig chan struct{}) {
	for {
		select {
		case <-closeSig:
			s.commandServer.Close()
			s.ChanRPCServer.Close()
			s.g.Close()
			s.client.Close()
			// dispatcher 没有关闭，可能会有定时器触发后往
			// dispatcher.ChanTimer通道发消息，但没什么影响
			return
		case cb := <-s.g.ChanCb:
			s.g.Cb(cb)
		case t := <-s.dispatcher.ChanTimer:
			t.Cb()
		case ri := <-s.client.ChanAsyncRet:
			s.client.Cb(ri)
		case ci := <-s.ChanRPCServer.ChanCall:
			s.ChanRPCServer.Exec(ci)
		case ci := <-s.commandServer.ChanCall:
			s.commandServer.Exec(ci)
		}
	}
}

func (s *Skeleton) AfterFunc(d time.Duration, cb func()) *timer.Timer {
	if s.TimerDispatcherLen <= 0 {
		panic("invalid TimerDispatcherLen")
	}

	return s.dispatcher.AfterFunc(d, cb)
}

func (s *Skeleton) CronFunc(expr *timer.CronExpr, cb func()) *timer.Cron {
	if s.TimerDispatcherLen <= 0 {
		panic("invalid TimerDispatcherLen")
	}

	return s.dispatcher.CronFunc(expr, cb)
}

func (s *Skeleton) Go(f, cb func()) {
	if s.GoLen <= 0 {
		panic("invalid GoLen")
	}

	s.g.Go(f, cb)
}

func (s *Skeleton) NewLinearContext() *g.LinearContext {
	if s.GoLen <= 0 {
		panic("invalid GoLen")
	}

	return s.g.NewLinearContext()
}

func (s *Skeleton) AsyncCall(server *chanrpc.Server, id interface{}, args ...interface{}) {
	if s.AsyncCallLen <= 0 {
		panic("invalid AsyncCallLen")
	}

	s.client.Attach(server)
	s.client.AsyncCall(id, args...)
}

func (s *Skeleton) RegisterChanRPC(id, f interface{}) {
	if s.ChanRPCServer == nil {
		panic("invalid ChanRPCServer")
	}

	s.ChanRPCServer.Register(id, f)
}

func (s *Skeleton) RegisterCommand(name, help string, f interface{}) {
	console.Register(name, help, f, s.commandServer)
}
