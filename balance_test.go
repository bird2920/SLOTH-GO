package main

import (
	"sync"
	"testing"
)

func TestBalancer_Next(t *testing.T) {
	tests := []struct {
		name    string
		folders []string
		calls   int
		want    []string
	}{
		{
			name:    "Empty slice",
			folders: []string{},
			calls:   1,
			want:    []string{""},
		},
		{
			name:    "Single folder",
			folders: []string{"/path1"},
			calls:   3,
			want:    []string{"/path1", "/path1", "/path1"},
		},
		{
			name:    "Multiple folders round robin",
			folders: []string{"/path1", "/path2", "/path3"},
			calls:   5,
			want:    []string{"/path1", "/path2", "/path3", "/path1", "/path2"},
		},
		{
			name:    "Two folders",
			folders: []string{"/downloads", "/archive"},
			calls:   4,
			want:    []string{"/downloads", "/archive", "/downloads", "/archive"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := &Balancer{}
			results := make([]string, tt.calls)

			for i := 0; i < tt.calls; i++ {
				val, err := b.Next(tt.folders)
				if len(tt.folders) == 0 && err == nil {
					t.Errorf("expected error for empty folders slice")
				}
				if len(tt.folders) > 0 && err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				results[i] = val
			}

			for i, expected := range tt.want {
				if results[i] != expected {
					t.Errorf("Call %d: got %v, want %v", i, results[i], expected)
				}
			}
		})
	}
}

// Test concurrent access to balancer
func TestBalancer_Concurrent(t *testing.T) {
	b := &Balancer{}
	folders := []string{"/path1", "/path2", "/path3"}

	const numGoroutines = 10
	const callsPerGoroutine = 100

	results := make(chan string, numGoroutines*callsPerGoroutine)
	var wg sync.WaitGroup

	// Start multiple goroutines
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < callsPerGoroutine; j++ {
				val, err := b.Next(folders)
				if err != nil {
					// Should not happen for non-empty slice
					continue
				}
				results <- val
			}
		}()
	}

	wg.Wait()
	close(results)

	// Verify all results are valid folder paths
	count := 0
	folderCounts := make(map[string]int)

	for result := range results {
		count++
		folderCounts[result]++

		// Verify result is one of the expected folders
		found := false
		for _, folder := range folders {
			if result == folder {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Unexpected folder returned: %s", result)
		}
	}

	if count != numGoroutines*callsPerGoroutine {
		t.Errorf("Expected %d results, got %d", numGoroutines*callsPerGoroutine, count)
	}
}
