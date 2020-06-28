package network

import (
	"net"
	"sync"
	"time"

	"github.com/skeletongo/leaf.v1/log"
)

type TCPClient struct {
	sync.Mutex
	Addr            string
	ConnNum         int
	ConnectInterval time.Duration // 连接失败后的重连时间间隔
	PendingWriteNum int           // 发送消息队列缓冲区长度
	AutoReconnect   bool
	NewAgent        func(conn *TCPConn) Agent
	connMap         map[net.Conn]struct{}
	closeFlag       bool
	wg              sync.WaitGroup

	// PkgParser
	ByteLen      int
	MinPkgLen    uint32
	MaxPkgLen    uint32
	LittleEndian bool
	pkgParser    *PkgParser
}

func (c *TCPClient) Start() {
	c.init()
	for i := 0; i < c.ConnNum; i++ {
		c.wg.Add(1)
		go c.connect()
	}
}

func (c *TCPClient) init() {
	if c.NewAgent == nil {
		log.Fatal("NewAgent must not be nil")
	}
	if c.ConnNum <= 0 {
		c.ConnNum = 1
		log.Release("invalid ConnNum, reset to %v", c.ConnNum)
	}
	if c.ConnectInterval <= 0 {
		c.ConnectInterval = 3 * time.Second
		log.Release("invalid ConnectInterval, reset to %vs", c.ConnectInterval.Seconds())
	}
	if c.PendingWriteNum <= 0 {
		c.PendingWriteNum = 100
		log.Release("invalid PendingWriteNum, reset to %v", c.PendingWriteNum)
	}

	c.closeFlag = false
	c.connMap = make(map[net.Conn]struct{})
	// PkgParser
	c.pkgParser = NewPkgParser()
	c.pkgParser.SetPkgLen(c.ByteLen, c.MinPkgLen, c.MaxPkgLen)
	c.pkgParser.SetEndian(c.LittleEndian)
}

func (c *TCPClient) dial() net.Conn {
	conn, err := net.Dial("tcp", c.Addr)
	if err == nil || c.closeFlag {
		return conn
	}
	log.Release("connect to %v error: %v", c.Addr, err)
	time.Sleep(c.ConnectInterval)
	return c.dial()
}

func (c *TCPClient) connect() {
	defer c.wg.Done()
here:
	conn := c.dial()
	if conn == nil {
		return
	}
	c.Lock()
	if c.closeFlag {
		c.Unlock()
		_ = conn.Close()
		return
	}
	c.connMap[conn] = struct{}{}
	c.Unlock()

	tcpConn := newTCPConn(conn, c.PendingWriteNum, c.pkgParser)
	agent := c.NewAgent(tcpConn)
	agent.Run()
	tcpConn.Close()

	c.Lock()
	delete(c.connMap, conn)
	c.Unlock()

	agent.OnClose()
	if c.AutoReconnect {
		goto here
	}
}

func (c *TCPClient) Close() {
	c.Lock()
	c.closeFlag = true
	for conn := range c.connMap {
		_ = conn.Close()
	}
	c.connMap = nil
	c.Unlock()
	c.wg.Wait()
}
