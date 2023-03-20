package utils

import (
	"testing"
	"time"
)

func TestCache(t *testing.T) {
	cache := NewTTLCache[string, int](2, 2)

	err := cache.Set("key1", 1, time.Second*1)
	if err != nil {
		t.Error("error calling Set", err)
	}
	err = cache.Set("key2", 2, time.Second*4)
	if err != nil {
		t.Error("error calling Set", err)
	}
	err = cache.Set("key3", 3, time.Second*4)
	if err == nil {
		t.Error("max capacity not working")
	}

	if cache.Len() != 2 {
		t.Error("incorrect Len() value")
	}
	if i, ok := cache.Get("key1"); !ok {
		t.Error("basic Set not working :", i)
	}
	// this is a time sensitive test, may be not a good approach ?
	time.Sleep(2 * time.Second)
	// key1 should be gone
	if i, ok := cache.Get("key1"); ok {
		t.Error("timer not working", i)
	}
	if cache.Len() != 1 {
		t.Error("incorrect Len() value")
	}
	if err := cache.Delete("key2"); err != nil {
		t.Error("delete not working:", err)
	}
	if cache.Len() != 0 {
		t.Error("incorrect Len() value")
	}
}
