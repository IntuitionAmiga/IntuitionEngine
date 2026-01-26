// terminal_output_debug.go - Debug version with more verbose output

package main

import (
	"fmt"
	"sync"
)

// Constants for terminal output
// Note: TERMINAL_OUT is defined in registers.go (master I/O address map)
const (
	MAX_LINE = 1024 // Maximum line length
)

// TerminalOutput implements a simple terminal output device
type TerminalOutput struct {
	mutex      sync.Mutex
	enabled    bool
	buffer     []byte
	maxLineLen int
}

// NewTerminalOutput creates a new terminal output device
func NewTerminalOutput() *TerminalOutput {
	return &TerminalOutput{
		enabled:    true,
		maxLineLen: MAX_LINE,
		buffer:     make([]byte, 0, MAX_LINE),
	}
}

// // HandleWrite processes writes to the terminal output register
//
//	func (t *TerminalOutput) HandleWrite(addr uint32, value uint32) {
//		fmt.Printf("DEBUG: Terminal write caught: addr=0x%X, value=0x%X (%c)\n", addr, value, byte(value&0xFF))
//		t.mutex.Lock()
//		defer t.mutex.Unlock()
//
//		if !t.enabled {
//			fmt.Println("Terminal output disabled, ignoring write")
//			return
//		}
//
//		// Use the least significant byte as the character
//		char := byte(value & 0xFF)
//
//		// Process special characters
//		if char == 0 {
//			// Null terminator - flush buffer
//			t.flush()
//			return
//		}
//
//		// Add to buffer
//		t.buffer = append(t.buffer, char)
//
//		// Flush on newline or if buffer full
//		if char == 10 || len(t.buffer) >= t.maxLineLen {
//			t.flush()
//		}
//	}
//
// // flush prints the current buffer to the console
//
//	func (t *TerminalOutput) flush() {
//		if len(t.buffer) > 0 {
//			fmt.Printf("Terminal output: %s", string(t.buffer))
//			t.buffer = t.buffer[:0]
//		} else {
//			fmt.Println("Terminal flush: buffer empty")
//		}
//	}
//
// HandleWrite processes writes to the terminal output register
func (t *TerminalOutput) HandleWrite(addr uint32, value uint32) {
	// Normalize address to handle both direct and sign-extended forms
	if addr == TERM_OUT_16BIT || addr == TERM_OUT_SIGNEXT {
		t.mutex.Lock()
		defer t.mutex.Unlock()

		if !t.enabled {
			return
		}

		// Use the least significant byte as the character
		char := byte(value & 0xFF)

		// For debugging
		fmt.Printf("Terminal output: %c (0x%02X)\n", char, char)

		// Immediately print the character
		fmt.Printf("%c", char)

		// Don't buffer - just print directly
		return
	}

	fmt.Printf("Warning: Write to unrecognized terminal address: 0x%08X\n", addr)
}

// flush prints the current buffer to the console
func (t *TerminalOutput) flush() {
	if len(t.buffer) > 0 {
		fmt.Print(string(t.buffer))
		t.buffer = t.buffer[:0]
	}
}

// Enable enables the terminal output
func (t *TerminalOutput) Enable() {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	fmt.Println("Terminal output enabled")
	t.enabled = true
}

// Disable disables the terminal output
func (t *TerminalOutput) Disable() {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	fmt.Println("Terminal output disabled")
	t.enabled = false
}
