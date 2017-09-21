// +build go1.9

package tcpkeepalive

import (
	"net"
	"time"
)

// SetKeepAliveIdle sets the time the connection must be idle before keepalive
// probes are sent.
func SetKeepAliveIdle(c *net.TCPConn, d time.Duration) error {
	return control(c, func(fd uintptr) error {
		return setIdle(fd, d)
	})
}

// SetKeepAliveCount sets the number of keepalive probes without an acknowledge
// TCP should send before dropping the connection.
func SetKeepAliveCount(c *net.TCPConn, n int) error {
	return control(c, func(fd uintptr) error {
		return setCount(fd, n)
	})
}

// SetKeepAliveInterval sets the time between keepalive probes.
func SetKeepAliveInterval(c *net.TCPConn, d time.Duration) error {
	return control(c, func(fd uintptr) error {
		return setInterval(fd, d)
	})
}

func control(c *net.TCPConn, f func(fd uintptr) error) error {
	rc, err := c.SyscallConn()
	if err != nil {
		return err
	}
	var operr error
	err = rc.Control(func(fd uintptr) {
		operr = f(fd)
	})
	if err != nil {
		return err
	}
	return operr
}
