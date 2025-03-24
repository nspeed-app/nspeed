package main

import (
	"fmt"
	"runtime"
	"time"

	"nspeed.app/helper/ttlmap"
)

var cache *ttlmap.TTLMap[string, int]

func init() {
	cache = ttlmap.New[string, int](20, 100)
}

func main2(loop int) {
	err := cache.Set("one", 1, time.Second)
	if err != nil {
		panic(err)
	}
	err = cache.Set("two", 1, time.Second/10)
	if err != nil {
		panic(err)
	}
	time.Sleep(time.Second / 4)
	fmt.Println("exiting 2", loop)
}

func main() {
	fmt.Println("launching, rc:", runtime.NumGoroutine())
	main2(1)
	cache.Clear()
	main2(1)
	runtime.GC()
	fmt.Println("exiting, rc:", runtime.NumGoroutine())
	ng := runtime.NumGoroutine()
	for {
		newng := runtime.NumGoroutine()
		if newng != ng {
			fmt.Printf("waiting for %d remaining goroutines\n", ng)
		}
		ng = newng
		if ng == 1 {
			break
		}
		time.Sleep(time.Second / 10)
	}
	fmt.Println("finally exiting")
}
