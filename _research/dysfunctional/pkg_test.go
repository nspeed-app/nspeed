// https://gist.github.com/rednafi/08fe371ed31072ab0bd96bf51611660a

package src

import (
	"testing"
)

// Apply functional options pattern
type config struct {
	// Required
	foo, bar string

	// Optional
	fizz, buzz int
}

type option func(*config)

func WithFizz(fizz int) option {
	return func(c *config) {
		c.fizz = fizz
	}
}

func WithBuzz(buzz int) option {
	return func(c *config) {
		c.buzz = buzz
	}
}

func NewConfig(foo, bar string, opts ...option) *config {
	c := &config{foo: foo, bar: bar}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Benchmark the functional options pattern
func BenchmarkNewConfig(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		NewConfig("foo", "bar", WithFizz(1), WithBuzz(2))
	}
}

// Dysfunctional options pattern
type configDys struct {
	// Required
	foo, bar string

	// Optional
	fizz, buzz int
}

func NewConfigDys(foo, bar string) *configDys {
	return &configDys{foo: foo, bar: bar}
}

func (c *configDys) WithFizz(fizz int) *configDys {
	c.fizz = fizz
	return c
}

func (c *configDys) WithBuzz(buzz int) *configDys {
	c.buzz = buzz
	return c
}

// Benchmark the dysfunctional options pattern
func BenchmarkNewConfigDys(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		NewConfigDys("foo", "bar").WithFizz(1).WithBuzz(2)
	}
}
