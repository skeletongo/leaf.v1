package network

import (
	"errors"
	"net"
)

type TCPConn struct {
	conn      net.Conn      // tcp链接
	writeChan chan []byte   // 消息发送缓冲队列
	closeFlag chan struct{} // 关闭标志
	pkgParser *PkgParser    // 封包拆包规则
}

func newTCPConn(conn net.Conn, l int, pkgParser *PkgParser) *TCPConn {
	c := new(TCPConn)
	c.conn = conn
	c.writeChan = make(chan []byte, l)
	c.closeFlag = make(chan struct{})
	c.pkgParser = pkgParser

	go func() {
		for v := range c.writeChan {
			if v == nil {
				break
			}
			if _, err := conn.Write(v); err != nil {
				break
			}
		}
		select {
		case <-c.closeFlag:
		default:
			close(c.closeFlag)
		}
		_ = conn.Close()
	}()

	return c
}

// implement Conn

func (c *TCPConn) ReadMsg() ([]byte, error) {
	return c.pkgParser.Read(c)
}

func (c *TCPConn) WriteMsg(args ...[]byte) error {
	select {
	case <-c.closeFlag:
		return errors.New("connection closed")
	default:
		err := c.pkgParser.Write(c, args...)
		if err != nil {
			c.Destroy()
		}
		return err
	}
}

func (c *TCPConn) LocalAddr() net.Addr {
	return c.conn.LocalAddr()
}

func (c *TCPConn) RemoteAddr() net.Addr {
	return c.conn.RemoteAddr()
}

// Close 正常关闭，等待消息发送完成
func (c *TCPConn) Close() {
	select {
	case <-c.closeFlag:
		return
	default:
		close(c.closeFlag)
	}

	select {
	case c.writeChan <- nil: // 为了关闭消息发送协程
	default:
		_ = c.conn.(*net.TCPConn).SetLinger(0) // 调用Close()方法关闭链接时，立即停止数据发送
		_ = c.conn.Close()
	}
}

func (c *TCPConn) Destroy() {
	select {
	case <-c.closeFlag:
		return
	default:
		close(c.closeFlag)
	}

	select {
	case c.writeChan <- nil: // 为了关闭消息发送协程
	default:
	}
	_ = c.conn.(*net.TCPConn).SetLinger(0) // 调用Close()方法关闭链接时，立即停止数据发送
	_ = c.conn.Close()
}

// implement io.ReadWriter

func (c *TCPConn) Read(p []byte) (n int, err error) {
	return c.conn.Read(p)
}

func (c *TCPConn) Write(p []byte) (n int, err error) {
	if len(p) == 0 {
		return 0, nil
	}
	select {
	case c.writeChan <- p:
	default:
		err = errors.New("write channel full")
	}
	return
}
