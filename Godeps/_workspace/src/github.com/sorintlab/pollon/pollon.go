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

	"github.com/sorintlab/stolon/Godeps/_workspace/src/github.com/coreos/pkg/capnslog"
)

var log = capnslog.NewPackageLogger("github.com/sorintlab/pollon", "pollon")

const (
	// min check interval
	minCheckInterval = 100 * time.Millisecond
)

type ConfChecker interface {
	Check() (destAddr *net.TCPAddr, err error)
}

type Proxy struct {
	listener     *net.TCPListener
	config       *Config
	confMutex    sync.Mutex
	destAddr     *net.TCPAddr
	closeConns   chan struct{}
	connMutex    sync.Mutex
	checkerError error
}

type Config struct {
	ConfChecker   ConfChecker
	CheckInterval time.Duration
	// whether to continue or exit if the confChecker returns an error (defaults to false)
	ExitOnCheckerError bool
}

func NewProxy(listener *net.TCPListener, config *Config) (*Proxy, error) {
	if config == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}
	if config.ConfChecker == nil {
		return nil, fmt.Errorf("confChecker cannot be nil")
	}
	if config.CheckInterval < minCheckInterval {
		config.CheckInterval = minCheckInterval
	}
	return &Proxy{
		listener:   listener,
		config:     config,
		closeConns: make(chan struct{}),
		connMutex:  sync.Mutex{},
	}, nil
}

func (p *Proxy) proxyConn(conn *net.TCPConn) {
	p.connMutex.Lock()
	closeConns := p.closeConns
	destAddr := p.destAddr
	p.connMutex.Unlock()
	defer func() {
		log.Debugf("closing source connection: %v", conn)
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
		log.Debugf("closing destination connection: %v", destConn)
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
		log.Debugf("ending. copied %d bytes from source to dest", n)
	}()
	wg.Add(1)
	go func() {
		defer wg.Done()
		n, err := io.Copy(conn, destConn)
		if err != nil {
		}
		destConn.Close()
		conn.CloseRead()
		log.Debugf("ending. copied %d bytes from dest to source", n)
	}()

	go func() {
		wg.Wait()
		end <- true
	}()

	select {
	case <-end:
		log.Debugf("all io copy goroutines done")
		return
	case <-closeConns:
		log.Debugf("closing all connections")
		return
	}
}

func (p *Proxy) confCheck(errCh chan error, stop chan struct{}) {
	endCh := make(chan error)
	c := time.NewTimer(0).C
	for {
		select {
		case <-stop:
			return
		case <-c:
			go func(endCh chan error) {
				endCh <- func() error {
					destAddr, err := p.config.ConfChecker.Check()
					if err != nil {
						p.connMutex.Lock()
						close(p.closeConns)
						p.closeConns = make(chan struct{})
						p.destAddr = nil
						p.connMutex.Unlock()

						if p.config.ExitOnCheckerError {
							return err
						}
						return nil
					}
					if destAddr.String() != p.destAddr.String() {
						p.connMutex.Lock()
						close(p.closeConns)
						p.closeConns = make(chan struct{})
						p.destAddr = destAddr
						p.connMutex.Unlock()
					}
					return nil
				}()
			}(endCh)
		case err := <-endCh:
			if err != nil {
				errCh <- err
				return
			}
			p.confMutex.Lock()
			c = time.NewTimer(p.config.CheckInterval).C
			p.confMutex.Unlock()
		}
	}
}

func (p *Proxy) setCheckInterval(checkInterval time.Duration) {
	p.confMutex.Lock()
	p.config.CheckInterval = checkInterval
	p.confMutex.Unlock()
}

func (p *Proxy) setConfig(config *Config) {
	p.confMutex.Lock()
	p.config = config
	p.confMutex.Unlock()
}

func (p *Proxy) accepter(errCh chan error) {
	for {
		conn, err := p.listener.AcceptTCP()
		if err != nil {
			errCh <- fmt.Errorf("accept error: %v", err)
			return
		}
		go p.proxyConn(conn)
	}
}

func (p *Proxy) Start() error {
	stop := make(chan struct{})
	errCh := make(chan error)
	go p.confCheck(errCh, stop)
	go p.accepter(errCh)
	err := <-errCh
	close(stop)
	return fmt.Errorf("proxy error: %v", err)
}
