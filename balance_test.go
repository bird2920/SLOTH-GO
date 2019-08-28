package main

import (
	"sync"
	"testing"
)

func TestBalancer_Next(t *testing.T) {
	type fields struct {
		state int
		m     sync.Mutex
	}
	type args struct {
		folders []string
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   string
	}{
		// TODO: Add test cases.
		{"Default Case",[]string{""}} //https://medium.com/@sebdah/go-best-practices-testing-3448165a0e18
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := &Balancer{
				state: tt.fields.state,
				m:     tt.fields.m,
			}
			if got := b.Next(tt.args.folders); got != tt.want {
				t.Errorf("Balancer.Next() = %v, want %v", got, tt.want)
			}
		})
	}
}
