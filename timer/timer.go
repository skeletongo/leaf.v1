package timer

import (
	"runtime"
	"time"

	"github.com/skeletongo/leaf.v1/conf"
	"github.com/skeletongo/leaf.v1/log"
)

// one dispatcher per goroutine (goroutine not safe)
type Dispatcher struct {
	ChanTimer chan *Timer
}

func NewDispatcher(l int) *Dispatcher {
	d := new(Dispatcher)
	d.ChanTimer = make(chan *Timer, l)
	return d
}

// Timer
type Timer struct {
	t  *time.Timer
	cb func()
}

func (t *Timer) Stop() {
	t.t.Stop()
	t.cb = nil
}

func (t *Timer) Cb() {
	defer func() {
		t.cb = nil
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

	if t.cb != nil {
		t.cb()
	}
}

func (d *Dispatcher) AfterFunc(duration time.Duration, cb func()) *Timer {
	t := new(Timer)
	t.cb = cb
	t.t = time.AfterFunc(duration, func() {
		d.ChanTimer <- t
	})
	return t
}

// Cron
type Cron struct {
	t *Timer
}

func (c *Cron) Stop() {
	if c.t != nil {
		c.t.Stop()
	}
}

func (d *Dispatcher) CronFunc(cronExpr *CronExpr, cb func()) *Cron {
	c := new(Cron)

	now := time.Now()
	nextTime := cronExpr.Next(now)
	if nextTime.IsZero() {
		return c
	}

	// callback
	var _cb func()
	_cb = func() {
		defer cb()

		now := time.Now()
		nextTime := cronExpr.Next(now)
		if nextTime.IsZero() {
			return
		}
		c.t = d.AfterFunc(nextTime.Sub(now), _cb)
	}

	c.t = d.AfterFunc(nextTime.Sub(now), _cb)
	return c
}
