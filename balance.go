package main

import (
	"errors"
	"sync"
)

// Balancer maintains the load balancer state
type Balancer struct {
	state int
	m     sync.Mutex
}

// Next will get the next round robin folder
func (b *Balancer) Next(folders []string) (string, error) {
	if len(folders) == 0 {
		return "", errors.New("balancer: empty folders slice")
	}

	// lock so we have exclusive access to state
	b.m.Lock()
	f := folders[b.state]
	b.state++

	if b.state >= len(folders) {
		b.state = 0
	}
	b.m.Unlock()
	return f, nil
}
