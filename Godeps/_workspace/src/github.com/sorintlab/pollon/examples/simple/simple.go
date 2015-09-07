package main

import (
	"io/ioutil"
	"log"
	"net"
	"strings"
	"time"

	"github.com/sorintlab/stolon/Godeps/_workspace/src/github.com/sorintlab/pollon"
)

type ConfChecker struct{}

func (c *ConfChecker) Check() (*net.TCPAddr, error) {
	conf, err := ioutil.ReadFile("./conf")
	if err != nil {
		log.Printf("err: %v", err)
		return nil, err
	}
	addrStr := strings.TrimSpace(string(conf))
	_, _, err = net.SplitHostPort(addrStr)
	if err != nil {
		log.Printf("err: %v", err)
		return nil, err
	}
	addr, err := net.ResolveTCPAddr("tcp", addrStr)
	if err != nil {
		log.Printf("error resolving address: %v", err)
		return nil, err
	}
	log.Printf("address: %s", addr)
	return addr, nil
}

func main() {
	addr, err := net.ResolveTCPAddr("tcp", "127.0.0.1:2222")
	if err != nil {
		log.Fatalf("error: %v", err)
	}
	listener, err := net.ListenTCP("tcp", addr)
	if err != nil {
		log.Fatalf("error: %v", err)
	}

	proxyConfig := &pollon.Config{
		ConfChecker:        &ConfChecker{},
		CheckInterval:      1 * time.Second,
		ExitOnCheckerError: false,
	}
	proxy, err := pollon.NewProxy(listener, proxyConfig)
	if err != nil {
		log.Fatalf("error: %v", err)
	}
	err = proxy.Start()
	if err != nil {
		log.Fatalf("error: %v", err)
	}

}
