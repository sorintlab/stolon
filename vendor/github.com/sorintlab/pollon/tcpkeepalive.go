// +build go1.9

package pollon

import (
	"net"

	"github.com/sorintlab/tcpkeepalive"
)

func (p *Proxy) SetupKeepAlive(conn *net.TCPConn) error {
	if err := conn.SetKeepAlive(true); err != nil {
		return err
	}
	if p.keepAliveIdle != 0 {
		if err := tcpkeepalive.SetKeepAliveIdle(conn, p.keepAliveIdle); err != nil {
			return err
		}
	}
	if p.keepAliveCount != 0 {
		if err := tcpkeepalive.SetKeepAliveCount(conn, p.keepAliveCount); err != nil {
			return err
		}
	}
	if p.keepAliveInterval != 0 {
		if err := tcpkeepalive.SetKeepAliveInterval(conn, p.keepAliveInterval); err != nil {
			return err
		}
	}
	return nil
}
