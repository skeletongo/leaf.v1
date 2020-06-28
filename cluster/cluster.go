package cluster

import (
	"math"
	"time"

	"github.com/skeletongo/leaf.v1/conf"
	"github.com/skeletongo/leaf.v1/network"
)

var (
	server  *network.TCPServer
	clients []*network.TCPClient
)

func Init() {
	if conf.ListenAddr != "" {
		server = new(network.TCPServer)
		server.Addr = conf.ListenAddr
		server.MaxConnNum = int(math.MaxInt32)
		server.PendingWriteNum = conf.PendingWriteNum
		server.ByteLen = 4
		server.MaxPkgLen = math.MaxUint32
		server.NewAgent = newAgent

		server.Start()
	}

	for _, addr := range conf.ConnAddrs {
		client := new(network.TCPClient)
		client.Addr = addr
		client.ConnNum = 1
		client.ConnectInterval = 3 * time.Second
		client.PendingWriteNum = conf.PendingWriteNum
		client.ByteLen = 4
		client.MaxPkgLen = math.MaxUint32
		client.NewAgent = newAgent

		client.Start()
		clients = append(clients, client)
	}
}

func Destroy() {
	if server != nil {
		server.Close()
	}

	for _, client := range clients {
		client.Close()
	}
}

type Agent struct {
	conn *network.TCPConn
}

func newAgent(conn *network.TCPConn) network.Agent {
	a := new(Agent)
	a.conn = conn
	return a
}

func (a *Agent) Run() {}

func (a *Agent) OnClose() {}
