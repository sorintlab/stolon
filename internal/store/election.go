// Copyright 2026 PgVillage
// Copyright 2016 Sorint.lab
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

package store

import (
	"context"
	"sync"
	"time"

	"github.com/kvtools/valkeyrie/store"
)

const (
	defaultLockTTL = 20 * time.Second
)

// Candidate runs the leader election algorithm asynchronously
type Candidate struct {
	client store.Store
	key    string
	node   string

	electedCh chan bool
	lock      sync.Mutex
	lockTTL   time.Duration
	leader    bool
	stopCh    chan struct{}
	stopRenew chan struct{}
	resignCh  chan bool
	errCh     chan error
}

// NewCandidate creates a new Candidate
func NewCandidate(client store.Store, key, node string, ttl time.Duration) *Candidate {
	return &Candidate{
		client: client,
		key:    key,
		node:   node,

		leader:   false,
		lockTTL:  ttl,
		resignCh: make(chan bool),
		stopCh:   make(chan struct{}),
	}
}

// IsLeader returns true if the candidate is currently a leader.
func (c *Candidate) IsLeader() bool {
	c.lock.Lock()
	defer c.lock.Unlock()
	return c.leader
}

// RunForElection starts the leader election algorithm. Updates in status are
// pushed through the ElectedCh channel.
//
// ElectedCh is used to get a channel which delivers signals on
// acquiring or losing leadership. It sends true if we become
// the leader, and false if we lose it.
func (c *Candidate) RunForElection(ctx context.Context) (<-chan bool, <-chan error) {
	c.electedCh = make(chan bool)
	c.errCh = make(chan error)

	go c.campaign(ctx)

	return c.electedCh, c.errCh
}

// Stop running for election.
func (c *Candidate) Stop() {
	close(c.stopCh)
}

// Resign forces the candidate to step-down and try again.
// If the candidate is not a leader, it doesn't have any effect.
// Candidate will retry immediately to acquire the leadership. If no-one else
// took it, then the Candidate will end up being a leader again.
func (c *Candidate) Resign() {
	c.lock.Lock()
	leader := c.leader
	c.lock.Unlock()
	if !leader {
		return
	}
	select {
	case <-c.stopCh:
		return
	case c.resignCh <- true:
	default:
	}
}

func (c *Candidate) update(status bool) {
	c.lock.Lock()

	c.leader = status
	c.lock.Unlock()
	c.electedCh <- status
}

func (c *Candidate) initLock(ctx context.Context) (store.Locker, error) {
	// Give up on the lock session if
	// we recovered from a store failure
	if c.stopRenew != nil {
		close(c.stopRenew)
	}

	lockOpts := &store.LockOptions{
		Value: []byte(c.node),
	}

	if c.lockTTL > 0 {
		lockOpts.TTL = c.lockTTL
	} else {
		lockOpts.TTL = defaultLockTTL
	}

	lockOpts.RenewLock = make(chan struct{})
	c.stopRenew = lockOpts.RenewLock

	lock, err := c.client.NewLock(ctx, c.key, lockOpts)
	return lock, err
}

func (c *Candidate) campaign(ctx context.Context) {
	defer close(c.electedCh)
	defer close(c.errCh)

	for {
		// Start as a follower.
		c.update(false)

		lock, err := c.initLock(ctx)
		if err != nil {
			c.errCh <- err
			return
		}

		lostCh, err := lock.Lock(ctx)
		if err != nil {
			c.errCh <- err
			return
		}

		// Hooray! We acquired the lock therefore we are the new leader.
		c.update(true)

		select {
		case <-c.resignCh:
			// We were asked to resign, give up the lock and go back
			// campaigning.
			// TODO: implement zerolog
			_ = lock.Unlock(ctx)
		case <-c.stopCh:
			// Give up the leadership and quit.
			if c.leader {
				// TODO: implement zerolog
				_ = lock.Unlock(ctx)
			}
			return
		case <-ctx.Done():
			c.lock.Lock()
			wasLeader := c.leader
			c.leader = false
			c.lock.Unlock()
			if wasLeader {
				_ = lock.Unlock(ctx)
			}
			return
		case <-lostCh:
			// We lost the lock. Someone else is the leader, try again.
		}
	}
}
