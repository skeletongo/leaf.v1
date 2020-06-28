package network

import (
	"net"
	"sync"
	"time"

	"github.com/skeletongo/leaf.v1/log"
)

type TCPServer struct {
	sync.Mutex
	Addr            string // 服务地址
	MaxConnNum      int    // 最大连接数
	PendingWriteNum int    // 消息发送队列缓冲区长度
	NewAgent        func(conn *TCPConn) Agent
	ln              net.Listener
	connMap         map[net.Conn]struct{}
	wgLn            sync.WaitGroup
	wgConn          sync.WaitGroup

	// PkgParser
	ByteLen      int
	MinPkgLen    uint32
	MaxPkgLen    uint32
	LittleEndian bool
	pkgParser    *PkgParser
}

func (s *TCPServer) Start() {
	s.init()
	go s.run()
}

func (s *TCPServer) init() {
	if s.NewAgent == nil {
		log.Fatal("NewAgent must not be nil")
	}
	ln, err := net.Listen("tcp", s.Addr)
	if err != nil {
		log.Fatal("%v", err)
	}
	s.ln = ln
	s.connMap = make(map[net.Conn]struct{})

	if s.MaxConnNum <= 0 {
		s.MaxConnNum = 100
		log.Release("invalid MaxConnNum, reset to %v", s.MaxConnNum)
	}
	if s.PendingWriteNum <= 0 {
		s.PendingWriteNum = 100
		log.Release("invalid PendingWriteNum, reset to %v", s.PendingWriteNum)
	}

	// PkgParser
	s.pkgParser = NewPkgParser()
	s.pkgParser.SetPkgLen(s.ByteLen, s.MinPkgLen, s.MaxPkgLen)
	s.pkgParser.SetEndian(s.LittleEndian)
}

func (s *TCPServer) run() {
	s.wgLn.Add(1)
	defer s.wgLn.Done()
	var tempDelay time.Duration
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Temporary() {
				if tempDelay == 0 {
					tempDelay = 5 * time.Millisecond
				} else {
					tempDelay *= 2
				}
				if max := 1 * time.Second; tempDelay > max {
					tempDelay = max
				}
				log.Release("accept error: %v; retrying in %v", err, tempDelay)
				time.Sleep(tempDelay)
				continue
			}
			return
		}
		tempDelay = 0
		s.Lock()
		if len(s.connMap) >= s.MaxConnNum {
			s.Unlock()
			_ = conn.Close()
			log.Error("too many connections")
			continue
		}
		s.connMap[conn] = struct{}{}
		s.Unlock()

		s.wgConn.Add(1)

		tcpConn := newTCPConn(conn, s.PendingWriteNum, s.pkgParser)
		agent := s.NewAgent(tcpConn)
		go func() {
			agent.Run()
			tcpConn.Close()
			s.Lock()
			delete(s.connMap, conn)
			s.Unlock()
			agent.OnClose()
			s.wgConn.Done()
		}()
	}
}

func (s *TCPServer) Close() {
	_ = s.ln.Close()
	// 等待监听结束后断开所有链接
	s.wgLn.Wait()

	s.Lock()
	for k := range s.connMap {
		_ = k.Close()
	}
	s.connMap = nil
	s.Unlock()
	// 等待所有接收协程关闭
	s.wgConn.Wait()
}
