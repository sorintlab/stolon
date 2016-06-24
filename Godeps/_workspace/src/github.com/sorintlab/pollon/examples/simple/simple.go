package main

import (
	"io/ioutil"
	"log"
	"net"
	"strings"
	"time"

	"github.com/sorintlab/pollon"
)

func Check(c chan pollon.ConfData) {
	conf, err := ioutil.ReadFile("./conf")
	if err != nil {
		log.Printf("err: %v", err)
		c <- pollon.ConfData{DestAddr: nil}
		return
	}
	addrStr := strings.TrimSpace(string(conf))
	_, _, err = net.SplitHostPort(addrStr)
	if err != nil {
		log.Printf("err: %v", err)
		c <- pollon.ConfData{DestAddr: nil}
		return
	}
	addr, err := net.ResolveTCPAddr("tcp", addrStr)
	if err != nil {
		log.Printf("error resolving address: %v", err)
		c <- pollon.ConfData{DestAddr: nil}
		return
	}
	log.Printf("address: %s", addr)
	c <- pollon.ConfData{DestAddr: addr}
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

	proxy, err := pollon.NewProxy(listener)
	if err != nil {
		log.Fatalf("error: %v", err)
	}

	go func() {
		for {
			Check(proxy.C)
			time.Sleep(2 * time.Second)
		}
	}()

	err = proxy.Start()
	if err != nil {
		log.Fatalf("error: %v", err)
	}

}
