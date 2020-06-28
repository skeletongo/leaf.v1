package g

import (
	"container/list"
	"runtime"
	"sync"

	"github.com/skeletongo/leaf.v1/conf"
	"github.com/skeletongo/leaf.v1/log"
)

type Go struct {
	ChanCb    chan func() // 回调函数队列
	pendingGo int         // 执行中的调用数量
}

func New(l int) *Go {
	return &Go{
		ChanCb:    make(chan func(), l),
		pendingGo: 0,
	}
}

func (g *Go) Go(f func(), cb func()) {
	g.pendingGo++
	go func() {
		defer func() {
			g.ChanCb <- cb
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

		f()
	}()
}

func (g *Go) Cb(cb func()) {
	defer func() {
		g.pendingGo--
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
	if cb != nil {
		cb()
	}
}

func (g *Go) Close() {
	for g.pendingGo > 0 {
		g.Cb(<-g.ChanCb)
	}
}

func (g *Go) Idle() bool {
	return g.pendingGo == 0
}

type LinearGo struct {
	f  func()
	cb func()
}

type LinearContext struct {
	g           *Go
	linearGo    *list.List
	muxLinearGo sync.Mutex
	muxExec     sync.Mutex
}

func (g *Go) NewLinearContext() *LinearContext {
	return &LinearContext{
		g:        g,
		linearGo: list.New(),
	}
}

// Go 按调用的顺序执行
func (c *LinearContext) Go(f, cb func()) {
	c.g.pendingGo++

	c.muxLinearGo.Lock()
	c.linearGo.PushBack(&LinearGo{f: f, cb: cb})
	c.muxLinearGo.Unlock()

	go func() {
		// 保证所有协程串行执行
		c.muxExec.Lock()
		defer c.muxExec.Unlock()

		// 先进队列的先执行
		c.muxLinearGo.Lock()
		e := c.linearGo.Remove(c.linearGo.Front()).(*LinearGo)
		c.muxLinearGo.Unlock()

		defer func() {
			c.g.ChanCb <- e.cb
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

		e.f()
	}()
}
