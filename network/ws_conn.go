package network

import (
	"errors"
	"net"

	"github.com/gorilla/websocket"
)

type WebsocketConnSet map[*websocket.Conn]struct{}

type WSConn struct {
	conn      *websocket.Conn
	writeChan chan []byte
	closeFlag chan struct{} // 关闭标志
	maxPkgLen uint32
}

func newWSConn(conn *websocket.Conn, pendingWriteNum int, maxPkgLen uint32) *WSConn {
	c := new(WSConn)
	c.conn = conn
	c.writeChan = make(chan []byte, pendingWriteNum)
	c.closeFlag = make(chan struct{})
	c.maxPkgLen = maxPkgLen

	go func() {
		for b := range c.writeChan {
			if b == nil {
				break
			}
			err := conn.WriteMessage(websocket.BinaryMessage, b)
			if err != nil {
				break
			}
		}
		select {
		case <-c.closeFlag:
			return
		default:
			close(c.closeFlag)
		}
		_ = conn.Close()
	}()

	return c
}

func (c *WSConn) Close() {
	select {
	case <-c.closeFlag:
		return
	default:
		close(c.closeFlag)
	}

	select {
	case c.writeChan <- nil:
	default:
		_ = c.conn.UnderlyingConn().(*net.TCPConn).SetLinger(0)
		_ = c.conn.Close()
	}
}

func (c *WSConn) Destroy() {
	select {
	case <-c.closeFlag:
		return
	default:
		close(c.closeFlag)
	}

	select {
	case c.writeChan <- nil:
	default:
	}
	_ = c.conn.UnderlyingConn().(*net.TCPConn).SetLinger(0)
	_ = c.conn.Close()
}

func (c *WSConn) LocalAddr() net.Addr {
	return c.conn.LocalAddr()
}

func (c *WSConn) RemoteAddr() net.Addr {
	return c.conn.RemoteAddr()
}

func (c *WSConn) ReadMsg() ([]byte, error) {
	_, b, err := c.conn.ReadMessage()
	return b, err
}

func (c *WSConn) WriteMsg(args ...[]byte) error {
	select {
	case <-c.closeFlag:
		return errors.New("connection closed")
	default:
	}

	if len(c.writeChan) == cap(c.writeChan) {
		c.Destroy()
		return errors.New("write channel full")
	}

	// get len
	var pkgLen uint32
	for i := 0; i < len(args); i++ {
		pkgLen += uint32(len(args[i]))
	}

	// check len
	if pkgLen > c.maxPkgLen {
		return errors.New("message too long")
	} else if pkgLen < 1 {
		return errors.New("message too short")
	}

	var data []byte
	if len(args) > 1 {
		data = make([]byte, pkgLen)
		l := 0
		for i := 0; i < len(args); i++ {
			copy(data[l:], args[i])
			l += len(args[i])
		}
	} else {
		data = args[0]
	}

	select {
	case c.writeChan <- data:
	default:
		c.Destroy()
		return errors.New("write channel full")
	}
	return nil
}
