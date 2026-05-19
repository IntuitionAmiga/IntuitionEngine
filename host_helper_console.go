package main

import (
	"context"
	"sync"
)

type HostUpdateConfirmationKey int

const (
	HostUpdateConfirmationKeyNone HostUpdateConfirmationKey = iota
	HostUpdateConfirmationKeyY
	HostUpdateConfirmationKeyN
	HostUpdateConfirmationKeyEscape
)

type HostUpdateConfirmationAction struct {
	Done  bool
	Allow bool
}

type HostUpdateConfirmation struct {
	done  bool
	allow bool
}

func NewHostUpdateConfirmation() *HostUpdateConfirmation {
	return &HostUpdateConfirmation{}
}

func (c *HostUpdateConfirmation) HandleInput(key HostUpdateConfirmationKey) HostUpdateConfirmationAction {
	if c.done {
		return HostUpdateConfirmationAction{Done: true, Allow: c.allow}
	}

	switch key {
	case HostUpdateConfirmationKeyY:
		c.done = true
		c.allow = true
	case HostUpdateConfirmationKeyN, HostUpdateConfirmationKeyEscape:
		c.done = true
		c.allow = false
	}

	return HostUpdateConfirmationAction{Done: c.done, Allow: c.allow}
}

func (c *HostUpdateConfirmation) Done() bool {
	return c.done
}

func (c *HostUpdateConfirmation) Allow() bool {
	return c.done && c.allow
}

func (c *HostUpdateConfirmation) ConfirmHostUpdate(ctx context.Context) bool {
	if c == nil || !c.done {
		return false
	}
	return c.allow
}

type TerminalHostUpdateConfirmer struct {
	term *TerminalMMIO

	mu     sync.Mutex
	active bool
	input  chan byte
}

func NewTerminalHostUpdateConfirmer(term *TerminalMMIO) *TerminalHostUpdateConfirmer {
	return &TerminalHostUpdateConfirmer{term: term}
}

func (c *TerminalHostUpdateConfirmer) HandleInput(b byte) bool {
	c.mu.Lock()
	active := c.active
	input := c.input
	c.mu.Unlock()
	if !active || input == nil {
		return false
	}
	select {
	case input <- b:
	default:
	}
	return true
}

func (c *TerminalHostUpdateConfirmer) ConfirmHostUpdate(ctx context.Context) bool {
	if c == nil || c.term == nil {
		return false
	}

	input := make(chan byte, 8)
	c.mu.Lock()
	if c.active {
		c.mu.Unlock()
		return false
	}
	c.active = true
	c.input = input
	c.mu.Unlock()

	c.term.SetHostKeyInterceptor(c.HandleInput)
	defer func() {
		c.term.SetHostKeyInterceptor(nil)
		c.mu.Lock()
		c.active = false
		c.input = nil
		c.mu.Unlock()
	}()

	c.writeString("\r\n[Intuition Engine System]\r\n")
	c.writeString("This will run apt update && apt upgrade -y. It may take several minutes.\r\n")
	c.writeString("Proceed? [Y]es / [N]o ")

	for {
		select {
		case <-ctx.Done():
			c.writeString("\r\nCancelled.\r\n")
			return false
		case b := <-input:
			switch b {
			case 'y', 'Y':
				c.writeString("Yes\r\n")
				return true
			case 'n', 'N', 0x1b, 0x03:
				c.writeString("No\r\n")
				return false
			}
		}
	}
}

func (c *TerminalHostUpdateConfirmer) writeString(s string) {
	for i := 0; i < len(s); i++ {
		c.term.HandleWrite(TERM_OUT, uint32(s[i]))
	}
}
