// Copyright (c) Jean-Francois Giorgi & AUTHORS
// part of nspeed.app
// SPDX-License-Identifier: BSD-3-Clause

// Package ttlmap provides a thread-safe, in-memory map with time-to-live (TTL) expiration.
//
// It allows storing key-value pairs where each entry has an associated expiration timer.
// Once the timer expires, the entry is automatically removed from the map.
//
// The map is designed to be used in concurrent environments, providing safe access from multiple goroutines.
//
// Example Usage:
//
//	m := ttlmap.New[string, int](10, 100) // Create a map with initial capacity 10 and max capacity 100
//	err := m.Set("myKey", 123, time.Minute) // Set a value with a 1-minute TTL
//	if err != nil {
//		// handle error
//	}
//	value, ok := m.Get("myKey") // Get the value (if it exists and hasn't expired)
//	if ok {
//		fmt.Println("Value:", value)
//	}
//	m.Delete("myKey") // Manually delete an entry
//
// Features:
//   - Thread-safe: Safe for concurrent access.
//   - TTL expiration: Entries automatically expire after a specified duration.
//   - Configurable capacity: Set initial and maximum capacity (optional)
//   - Manual deletion: Entries can be manually deleted.
//   - Generic: Support any type for key and value.
//
// Limitations:
//   - In-memory: Data is not persisted across restarts.
//   - No eviction policy: When the maximum capacity is reached, `Set` will return an error.
package ttlmap

import (
	"fmt"
	"sync"
	"time"
)

// TTLMap implementes a map with an expiration timer for each entries
type TTLMap[K comparable, V any] struct {
	mu sync.RWMutex
	// explore: we could embed entries? declare the struct ?
	entries map[K]entries[V]
	// values      map[K]V
	// timers      map[K]*time.Timer
	close       *chan any
	maxCapacity int
}
type entries[V any] struct {
	value V
	timer *time.Timer
}

// New creates a new TTL map with given initial and maximum capacity (use 0 or a negative number
// for no capacity limit)
func New[K comparable, V any](initialCapacity, maxCapacity int) *TTLMap[K, V] {
	c := make(chan any)
	return &TTLMap[K, V]{
		entries:     make(map[K]entries[V], initialCapacity),
		close:       &c,
		maxCapacity: maxCapacity,
	}
}

// Len returns the current number of entries in the map
func (m *TTLMap[K, V]) Len() int {
	defer m.mu.Unlock()
	m.mu.Lock()

	return len(m.entries)
}

// Get returns an entry
func (m *TTLMap[K, V]) Get(key K) (V, bool) {
	defer m.mu.Unlock()
	m.mu.Lock()
	v, ok := m.entries[key]
	return v.value, ok
}

// Set creates a new entry in the map
func (m *TTLMap[K, V]) Set(key K, value V, ttl time.Duration) error {
	defer m.mu.Unlock()
	m.mu.Lock()
	_, ok := m.entries[key]
	if ok {
		return fmt.Errorf("TTL map key exists")
	}
	if m.maxCapacity > 0 && len(m.entries) >= m.maxCapacity {
		return fmt.Errorf("no more TTL map capacity")
	}

	timer := time.NewTimer(ttl)
	m.entries[key] = entries[V]{
		value: value,
		timer: timer,
	}
	// todo: investigate possible leak (if "m" is deleted when the select is still waiting)
	go func(c chan any) {
		select {
		case <-timer.C:
			_ = m.Delete(key)
		case <-c:
			timer.Stop()
		}
	}(*m.close)
	return nil
}

// Delete removes the entry matching the key
func (m *TTLMap[K, V]) Delete(key K) error {
	defer m.mu.Unlock()
	m.mu.Lock()
	e, ok := m.entries[key]
	if !ok {
		return fmt.Errorf("TTLMap key not found")
	}
	e.timer.Stop()
	delete(m.entries, key)
	return nil
}

// Clear deletes all entries and free their timers
func (m *TTLMap[K, V]) Clear() {
	defer m.mu.Unlock()
	m.mu.Lock()
	close(*m.close)
	c := make(chan any)
	m.close = &c
	clear(m.entries)
}
