package gate

import (
	"time"

	"github.com/skeletongo/leaf.v1/chanrpc"
	"github.com/skeletongo/leaf.v1/log"
	"github.com/skeletongo/leaf.v1/network"
)

type Gate struct {
	MaxConnNum      int
	PendingWriteNum int
	MaxPkgLen       uint32
	Processor       network.Processor
	AgentChanRPC    *chanrpc.Server

	// websocket
	WSAddr      string
	HTTPTimeout time.Duration
	CertFile    string
	KeyFile     string

	// tcp
	TCPAddr      string
	ByteLen      int
	LittleEndian bool
}

func (g *Gate) Run(closeSig chan struct{}) {
	if g.Processor == nil {
		log.Fatal("message Processor required")
	}
	// tcpServer
	var tcpServer *network.TCPServer
	if g.TCPAddr != "" {
		tcpServer = &network.TCPServer{
			Addr:            g.TCPAddr,
			MaxConnNum:      g.MaxConnNum,
			PendingWriteNum: g.PendingWriteNum,
			ByteLen:         g.ByteLen,
			MaxPkgLen:       g.MaxPkgLen,
			LittleEndian:    g.LittleEndian,
			NewAgent: func(conn *network.TCPConn) network.Agent {
				a := &agent{conn: conn, gate: g}
				if g.AgentChanRPC != nil {
					g.AgentChanRPC.Go("NewAgent", a)
				}
				return a
			},
		}
	}
	//wsServer
	var wsServer *network.WSServer
	if g.WSAddr != "" {
		wsServer = &network.WSServer{
			Addr:            g.WSAddr,
			MaxConnNum:      g.MaxConnNum,
			PendingWriteNum: g.PendingWriteNum,
			MaxPkgLen:       g.MaxPkgLen,
			HTTPTimeout:     g.HTTPTimeout,
			CertFile:        g.CertFile,
			KeyFile:         g.KeyFile,
			NewAgent: func(conn *network.WSConn) network.Agent {
				a := &agent{conn: conn, gate: g}
				if g.AgentChanRPC != nil {
					g.AgentChanRPC.Go("NewAgent", a)
				}
				return a
			},
		}
	}

	if tcpServer != nil {
		tcpServer.Start()
	}
	if wsServer != nil {
		wsServer.Start()
	}
	<-closeSig
	if tcpServer != nil {
		tcpServer.Close()
	}
	if wsServer != nil {
		wsServer.Close()
	}
}

func (g *Gate) OnDestroy() {}

type agent struct {
	conn     network.Conn
	gate     *Gate
	userData interface{}
}

func (a *agent) Run() {
	for {
		data, err := a.conn.ReadMsg()
		if err != nil {
			log.Debug("read message: %v", err)
			return
		}
		msg, err := a.gate.Processor.Unmarshal(data)
		if err != nil {
			log.Debug("unmarshal message error: %v", err)
			return
		}
		if err = a.gate.Processor.Route(msg, a.userData); err != nil {
			log.Debug("route message error: %v", err)
			return
		}
	}
}

func (a *agent) OnClose() {
	if a.gate.AgentChanRPC == nil {
		return
	}
	// 同步请求
	// 网关是最先关闭的服务，这里等待其它服务做好处理后关闭
	// 例如玩家缓存数据持久化
	if err := a.gate.AgentChanRPC.Call0("CloseAgent", a); err != nil {
		log.Error("CloseAgent error: %v", err)
	}
}
