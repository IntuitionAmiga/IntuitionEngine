//go:build darwin && !headless

package main

import "runtime"

// mainThreadQ carries callbacks that must run on the process main OS thread.
// Buffered so videoChip.Start() can post (via mainThreadCall) before
// driveMainThread begins pumping. In practice there is only one producer
// (the Ebiten video backend) and a single ebiten.RunGame callback; the
// buffer is sized small to absorb that gap without an unbounded queue.
var mainThreadQ = make(chan func(), 4)

// lockMainThread pins the Go main goroutine to the OS main thread. macOS
// Cocoa/AppKit refuses NSApplication/NSWindow operations from any other
// thread, so Ebiten's purego darwin backend must call into AppKit from
// here. Must be the first statement of main().
func lockMainThread() { runtime.LockOSThread() }

// mainThreadCall posts fn to the main-thread queue. fn runs when
// driveMainThread pumps the queue (typically: ebiten.RunGame).
func mainThreadCall(fn func()) { mainThreadQ <- fn }

// driveMainThread runs block in a goroutine while the calling (main)
// goroutine pumps mainThreadQ. Returns after block returns AND any
// late-posted callbacks have been drained.
func driveMainThread(block func()) {
	done := make(chan struct{})
	go func() {
		defer close(done)
		block()
	}()
	for {
		select {
		case fn := <-mainThreadQ:
			fn()
		case <-done:
			for {
				select {
				case fn := <-mainThreadQ:
					fn()
				default:
					return
				}
			}
		}
	}
}
