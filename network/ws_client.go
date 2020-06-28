package network

import (
	"sync"
	"time"

	"github.com/skeletongo/leaf.v1/log"
	"github.com/gorilla/websocket"
)

type WSClient struct {
	sync.Mutex
	Addr             string
	ConnNum          int
	ConnectInterval  time.Duration
	PendingWriteNum  int
	MaxPkgLen        uint32
	HandshakeTimeout time.Duration
	AutoReconnect    bool
	NewAgent         func(*WSConn) Agent
	dialer           websocket.Dialer
	connMap          map[*websocket.Conn]struct{}
	closeFlag        bool
	wg               sync.WaitGroup
}

func (c *WSClient) Start() {
	c.init()
	for i := 0; i < c.ConnNum; i++ {
		c.wg.Add(1)
		go c.connect()
	}
}

func (c *WSClient) init() {
	if c.NewAgent == nil {
		log.Fatal("NewAgent must not be nil")
	}
	if c.ConnNum <= 0 {
		c.ConnNum = 1
		log.Release("invalid ConnNum, reset to %v", c.ConnNum)
	}
	if c.ConnectInterval <= 0 {
		c.ConnectInterval = 3 * time.Second
		log.Release("invalid ConnectInterval, reset to %v", c.ConnectInterval)
	}
	if c.PendingWriteNum <= 0 {
		c.PendingWriteNum = 100
		log.Release("invalid PendingWriteNum, reset to %v", c.PendingWriteNum)
	}
	if c.MaxPkgLen <= 0 {
		c.MaxPkgLen = 4096
		log.Release("invalid MaxPkgLen, reset to %v", c.MaxPkgLen)
	}
	if c.HandshakeTimeout <= 0 {
		c.HandshakeTimeout = 10 * time.Second
		log.Release("invalid HandshakeTimeout, reset to %v", c.HandshakeTimeout)
	}

	c.closeFlag = false
	c.connMap = make(map[*websocket.Conn]struct{})
	c.dialer = websocket.Dialer{
		HandshakeTimeout: c.HandshakeTimeout,
	}
}

func (c *WSClient) dial() *websocket.Conn {
	conn, _, err := c.dialer.Dial(c.Addr, nil)
	if err == nil || c.closeFlag {
		return conn
	}
	log.Release("connect to %v error: %v", c.Addr, err)
	time.Sleep(c.ConnectInterval)
	return c.dial()
}

func (c *WSClient) connect() {
	defer c.wg.Done()

here:
	conn := c.dial()
	if conn == nil {
		return
	}
	conn.SetReadLimit(int64(c.MaxPkgLen))

	c.Lock()
	if c.closeFlag {
		c.Unlock()
		_ = conn.Close()
		return
	}
	c.connMap[conn] = struct{}{}
	c.Unlock()

	wsConn := newWSConn(conn, c.PendingWriteNum, c.MaxPkgLen)
	agent := c.NewAgent(wsConn)
	agent.Run()
	wsConn.Close()

	c.Lock()
	delete(c.connMap, conn)
	c.Unlock()

	agent.OnClose()
	if c.AutoReconnect {
		goto here
	}
}

func (c *WSClient) Close() {
	c.Lock()
	c.closeFlag = true
	for conn := range c.connMap {
		_ = conn.Close()
	}
	c.connMap = nil
	c.Unlock()
	c.wg.Wait()
}
