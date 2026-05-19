package main

import "context"

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
