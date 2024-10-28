// Copyright (c) Jean-Francois Giorgi & AUTHORS
// part of nspeed.app
// SPDX-License-Identifier: BSD-3-Clause
package utils

import (
	"fmt"
	"sync"
	"time"
)

// TTLCache implementes a map which entries expires after a given time
type TTLCache[K comparable, V any] struct {
	mu          sync.RWMutex
	values      map[K]V
	timers      map[K]*time.Timer
	close       chan any
	maxCapacity int
}

// NewTTLCache creates a new TTL Cache with given initial and maximum capacity (use 0 or a negative number
// for no capacity limit)
func NewTTLCache[K comparable, V any](initialCapacity, maxCapacity int) *TTLCache[K, V] {
	return &TTLCache[K, V]{
		values:      make(map[K]V, initialCapacity),
		timers:      make(map[K]*time.Timer, initialCapacity),
		close:       make(chan any),
		maxCapacity: maxCapacity,
	}
}

func (cache *TTLCache[K, V]) Len() int {
	defer cache.mu.Unlock()
	cache.mu.Lock()

	return len(cache.values)
}

// Get returns and optionnaly delete an entry
func (cache *TTLCache[K, V]) Get(key K) (V, bool) {
	cache.mu.Lock()
	v, ok := cache.values[key]
	cache.mu.Unlock()
	return v, ok
}

func (cache *TTLCache[K, V]) Set(key K, value V, ttl time.Duration) error {
	defer cache.mu.Unlock()
	cache.mu.Lock()
	_, ok := cache.values[key]
	if ok {
		return fmt.Errorf("TTL cache key exists")
	}
	if cache.maxCapacity > 0 && len(cache.values) >= cache.maxCapacity {
		return fmt.Errorf("no more TTL cache capacity")
	}

	cache.values[key] = value
	timer := time.NewTimer(ttl)
	cache.timers[key] = timer

	// todo: investigate possible leak (if "cache" is deleted when the select is still waiting)
	go func() {
		select {
		case <-timer.C:
			_ = cache.Delete(key)
		case <-cache.close:
			timer.Stop()
		}
	}()
	return nil
}

func (cache *TTLCache[K, V]) Delete(key K) error {
	defer cache.mu.Unlock()
	cache.mu.Lock()
	_, ok := cache.values[key]
	if !ok {
		return fmt.Errorf("TTL cache key not found")
	}
	timer := cache.timers[key]
	if timer == nil {
		return fmt.Errorf("TTL cache timer for key not found")
	}
	timer.Stop()
	delete(cache.values, key)
	delete(cache.timers, key)
	return nil
}
