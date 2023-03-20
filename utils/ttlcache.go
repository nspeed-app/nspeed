package utils

import (
	"fmt"
	"sync"
	"time"
)

// TTLCache implementes a map which entries expires after a given time
type TTLCache[K comparable, V any] struct {
	mu     sync.RWMutex
	values map[K]V
	timers map[K]*time.Timer
	close  chan any
}

func NewTTLCache[K comparable, V any](capacity int) *TTLCache[K, V] {
	return &TTLCache[K, V]{
		values: make(map[K]V, capacity),
		timers: make(map[K]*time.Timer, capacity),
		close:  make(chan any),
	}
}

// Get returns the same as the inner map[k]
func (cache *TTLCache[K, V]) Get(key K) (V, bool) {
	defer cache.mu.Unlock()
	cache.mu.Lock()
	v, ok := cache.values[key]
	return v, ok
}

func (cache *TTLCache[K, V]) Set(key K, value V, ttl time.Duration) error {
	defer cache.mu.Unlock()
	cache.mu.Lock()
	v, ok := cache.values[key]
	if ok {
		return fmt.Errorf("key exists")
	}
	cache.values[key] = v
	timer := time.NewTimer(ttl)
	cache.timers[key] = timer

	go func() {
		for {
			select {
			case <-timer.C:
				cache.Delete(key)
				break
			case <-cache.close:
				timer.Stop()
				break
			}
		}
	}()
	return nil
}

func (cache *TTLCache[K, V]) Delete(key K) error {
	defer cache.mu.Unlock()
	cache.mu.Lock()
	fmt.Println("deleting key", key)
	_, ok := cache.values[key]
	if !ok {
		return fmt.Errorf("key not found")
	}
	timer := cache.timers[key]
	if timer == nil {
		return fmt.Errorf("timer for key not found")
	}
	timer.Stop()
	delete(cache.values, key)
	delete(cache.timers, key)
	return nil
}
