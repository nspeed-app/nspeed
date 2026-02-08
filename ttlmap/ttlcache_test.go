// Copyright (c) Jean-Francois Giorgi & AUTHORS
// part of nspeed.app
// SPDX-License-Identifier: BSD-3-Clause

package ttlmap

import (
	"testing"
	"time"
)

func TestTTLMap(t *testing.T) {
	m := New[string, int](2, 2)

	err := m.Set("key1", 1, time.Second*1)
	if err != nil {
		t.Error("error calling Set", err)
	}
	err = m.Set("key2", 2, time.Second*4)
	if err != nil {
		t.Error("error calling Set", err)
	}
	err = m.Set("key3", 3, time.Second*4)
	if err == nil {
		t.Error("max capacity not working")
	}

	if m.Len() != 2 {
		t.Error("incorrect Len() value")
	}
	if i, ok := m.Get("key1"); !ok {
		t.Error("basic Set not working :", i)
	}
	// this is a time sensitive test, may be not a good approach ?
	time.Sleep(2 * time.Second)
	// key1 should be gone
	if i, ok := m.Get("key1"); ok {
		t.Error("timer not working", i)
	}
	if m.Len() != 1 {
		t.Error("incorrect Len() value")
	}
	if err := m.Delete("key2"); err != nil {
		t.Error("delete not working:", err)
	}
	if m.Len() != 0 {
		t.Error("incorrect Len() value")
	}
}
