// Copyright 2015 Sorint.lab
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied
// See the License for the specific language governing permissions and
// limitations under the License.

package pollon

import (
	"fmt"
	"io"
	"net"
	"sync"
	"time"
)

type ConfData struct {
	DestAddr *net.TCPAddr
}

type Proxy struct {
	C          chan ConfData
	listener   *net.TCPListener
	confMutex  sync.Mutex
	destAddr   *net.TCPAddr
	closeConns chan struct{}
	stop       chan struct{}
	endCh      chan error
	connMutex  sync.Mutex

	keepAlive         bool
	keepAliveIdle     time.Duration
	keepAliveCount    int
	keepAliveInterval time.Duration
}

func NewProxy(listener *net.TCPListener) (*Proxy, error) {
	return &Proxy{
		C:          make(chan ConfData),
		listener:   listener,
		closeConns: make(chan struct{}),
		stop:       make(chan struct{}),
		endCh:      make(chan error),
		connMutex:  sync.Mutex{},
	}, nil
}

func (p *Proxy) proxyConn(conn *net.TCPConn) {
	p.connMutex.Lock()
	closeConns := p.closeConns
	destAddr := p.destAddr
	p.connMutex.Unlock()
	defer func() {
		log.Printf("closing source connection: %v", conn)
		conn.Close()
	}()
	defer conn.Close()

	if destAddr == nil {
		return
	}

	destConn, err := net.DialTCP("tcp", nil, p.destAddr)
	if err != nil {
		conn.Close()
		return
	}
	defer func() {
		log.Printf("closing destination connection: %v", destConn)
		destConn.Close()
	}()

	var wg sync.WaitGroup
	end := make(chan bool)
	wg.Add(1)
	go func() {
		defer wg.Done()
		n, err := io.Copy(destConn, conn)
		if err != nil {
		}
		conn.Close()
		destConn.CloseRead()
		log.Printf("ending. copied %d bytes from source to dest", n)
	}()
	wg.Add(1)
	go func() {
		defer wg.Done()
		n, err := io.Copy(conn, destConn)
		if err != nil {
		}
		destConn.Close()
		conn.CloseRead()
		log.Printf("ending. copied %d bytes from dest to source", n)
	}()

	go func() {
		wg.Wait()
		end <- true
	}()

	select {
	case <-end:
		log.Printf("all io copy goroutines done")
		return
	case <-closeConns:
		log.Printf("closing all connections")
		return
	}
}

func (p *Proxy) confCheck() {
	for {
		select {
		case <-p.stop:
			return
		case confData := <-p.C:
			if confData.DestAddr.String() != p.destAddr.String() {
				p.connMutex.Lock()
				close(p.closeConns)
				p.closeConns = make(chan struct{})
				p.destAddr = confData.DestAddr
				p.connMutex.Unlock()
			}
		}
	}
}

func (p *Proxy) accepter() {
	for {
		conn, err := p.listener.AcceptTCP()
		if err != nil {
			p.endCh <- fmt.Errorf("accept error: %v", err)
			return
		}
		if p.keepAlive {
			if err := p.SetupKeepAlive(conn); err != nil {
				p.endCh <- fmt.Errorf("setKeepAlive error: %v", err)
				return
			}
		}
		go p.proxyConn(conn)
	}
}

func (p *Proxy) Stop() {
	p.endCh <- nil
}

func (p *Proxy) Start() error {
	go p.confCheck()
	go p.accepter()
	err := <-p.endCh
	close(p.stop)
	if err != nil {
		return fmt.Errorf("proxy error: %v", err)
	}
	return nil
}

func (p *Proxy) SetKeepAlive(keepalive bool) {
	p.keepAlive = keepalive
}

func (p *Proxy) SetKeepAliveIdle(d time.Duration) {
	p.keepAliveIdle = d
}

func (p *Proxy) SetKeepAliveCount(n int) {
	p.keepAliveCount = n
}

func (p *Proxy) SetKeepAliveInterval(d time.Duration) {
	p.keepAliveInterval = d
}
