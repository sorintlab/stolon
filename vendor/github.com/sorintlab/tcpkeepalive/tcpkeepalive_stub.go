// +build !linux

package tcpkeepalive

import (
	"time"
)

func setIdle(fd uintptr, d time.Duration) error {
	return nil
}

func setCount(fd uintptr, n int) error {
	return nil
}

func setInterval(fd uintptr, d time.Duration) error {
	return nil
}
