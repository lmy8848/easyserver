package infra

import (
	"log"
	"runtime/debug"
)

// Go runs fn in a goroutine with panic recovery.
// Use this instead of bare `go func()` to prevent a single panic from crashing the entire server.
func Go(fn func()) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("safego: recovered panic: %v\n%s", r, debug.Stack())
			}
		}()
		fn()
	}()
}
