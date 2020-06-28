package network

import (
	"crypto/tls"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/skeletongo/leaf.v1/log"
	"github.com/gorilla/websocket"
)

type WSServer struct {
	Addr            string
	MaxConnNum      int
	PendingWriteNum int
	MaxPkgLen       uint32
	HTTPTimeout     time.Duration
	CertFile        string
	KeyFile         string
	NewAgent        func(*WSConn) Agent
	ln              net.Listener
	handler         *WSHandler
}

type WSHandler struct {
	sync.Mutex
	maxConnNum      int
	pendingWriteNum int
	maxPkgLen       uint32
	newAgent        func(*WSConn) Agent
	upgrade         websocket.Upgrader
	connMap         map[*websocket.Conn]struct{}
	wg              sync.WaitGroup
}

func (h *WSHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", 405)
		return
	}
	conn, err := h.upgrade.Upgrade(w, r, nil)
	if err != nil {
		log.Debug("upgrade error: %v", err)
		return
	}
	conn.SetReadLimit(int64(h.maxPkgLen))

	h.wg.Add(1)
	defer h.wg.Done()

	h.Lock()
	if h.connMap == nil {
		h.Unlock()
		_ = conn.Close()
		return
	}
	if len(h.connMap) >= h.maxConnNum {
		h.Unlock()
		_ = conn.Close()
		log.Debug("too many connections")
		return
	}
	h.connMap[conn] = struct{}{}
	h.Unlock()

	wsConn := newWSConn(conn, h.pendingWriteNum, h.maxPkgLen)
	agent := h.newAgent(wsConn)
	agent.Run()

	// cleanup
	wsConn.Close()
	h.Lock()
	delete(h.connMap, conn)
	h.Unlock()
	agent.OnClose()
}

func (s *WSServer) Start() {
	if s.NewAgent == nil {
		log.Fatal("NewAgent must not be nil")
	}
	ln, err := net.Listen("tcp", s.Addr)
	if err != nil {
		log.Fatal("%v", err)
	}
	if s.CertFile != "" || s.KeyFile != "" {
		config := &tls.Config{}
		config.NextProtos = []string{"http/1.1"}

		var err error
		config.Certificates = make([]tls.Certificate, 1)
		config.Certificates[0], err = tls.LoadX509KeyPair(s.CertFile, s.KeyFile)
		if err != nil {
			log.Fatal("%v", err)
		}

		ln = tls.NewListener(ln, config)
	}
	if s.MaxConnNum <= 0 {
		s.MaxConnNum = 100
		log.Release("invalid MaxConnNum, reset to %v", s.MaxConnNum)
	}
	if s.PendingWriteNum <= 0 {
		s.PendingWriteNum = 100
		log.Release("invalid PendingWriteNum, reset to %v", s.PendingWriteNum)
	}
	if s.MaxPkgLen <= 0 {
		s.MaxPkgLen = 4096
		log.Release("invalid MaxPkgLen, reset to %v", s.MaxPkgLen)
	}
	if s.HTTPTimeout <= 0 {
		s.HTTPTimeout = 10 * time.Second
		log.Release("invalid HTTPTimeout, reset to %v", s.HTTPTimeout)
	}

	s.ln = ln
	s.handler = &WSHandler{
		maxConnNum:      s.MaxConnNum,
		pendingWriteNum: s.PendingWriteNum,
		maxPkgLen:       s.MaxPkgLen,
		newAgent:        s.NewAgent,
		connMap:         make(map[*websocket.Conn]struct{}),
		upgrade: websocket.Upgrader{
			HandshakeTimeout: s.HTTPTimeout,
			CheckOrigin:      func(_ *http.Request) bool { return true },
		},
	}

	httpServer := &http.Server{
		Addr:           s.Addr,
		Handler:        s.handler,
		ReadTimeout:    s.HTTPTimeout,
		WriteTimeout:   s.HTTPTimeout,
		MaxHeaderBytes: 1024,
	}

	go httpServer.Serve(ln)
}

func (s *WSServer) Close() {
	_ = s.ln.Close()

	s.handler.Lock()
	for conn := range s.handler.connMap {
		_ = conn.Close()
	}
	s.handler.connMap = nil
	s.handler.Unlock()

	s.handler.wg.Wait()
}
