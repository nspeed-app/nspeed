package utils

import (
	"testing"
)

func TestGetRoute(t *testing.T) {
	// we can't test much here. So we test localhost for IPv4 and IPv6
	_, _, _, err := GetRoute("127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	_, _, _, err = GetRoute("::1")
	if err != nil {
		t.Fatal(err)
	}
}
