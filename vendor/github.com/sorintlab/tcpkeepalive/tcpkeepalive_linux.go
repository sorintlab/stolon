package tcpkeepalive

import (
	"os"
	"syscall"
	"time"
)

func setIdle(fd uintptr, d time.Duration) error {
	// The kernel expects seconds so round to next highest second.
	d += (time.Second - time.Nanosecond)
	secs := int(d.Seconds())
	return os.NewSyscallError("setsockopt", syscall.SetsockoptInt(int(fd), syscall.IPPROTO_TCP, syscall.TCP_KEEPIDLE, secs))
}

func setCount(fd uintptr, n int) error {
	return os.NewSyscallError("setsockopt", syscall.SetsockoptInt(int(fd), syscall.IPPROTO_TCP, syscall.TCP_KEEPCNT, n))
}

func setInterval(fd uintptr, d time.Duration) error {
	// The kernel expects seconds so round to next highest second.
	d += (time.Second - time.Nanosecond)
	secs := int(d.Seconds())
	return os.NewSyscallError("setsockopt", syscall.SetsockoptInt(int(fd), syscall.IPPROTO_TCP, syscall.TCP_KEEPINTVL, secs))
}
