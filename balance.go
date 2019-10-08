package main

import (
	"sync"
)

// Balancer maintains the load balancer state
type Balancer struct {
	state int
	m     sync.Mutex
}

// Next will get the next round robin folder
func (b *Balancer) Next(folders []string) string {
	// lock so we have exclusive access to state
	b.m.Lock()
	f := folders[b.state]
	b.state++

	if b.state >= len(folders) {
		b.state = 0
	}

	// we are done with state unlock
	b.m.Unlock()

	return f
}
