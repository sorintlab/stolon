// +build go1.8,!go1.9

package pollon

import "net"

func (p *Proxy) SetupKeepAlive(conn *net.TCPConn) error {
	if err := conn.SetKeepAlive(true); err != nil {
		return err
	}
	// ignore fine grained keepalive parameters since they are not supported

	return nil
}
