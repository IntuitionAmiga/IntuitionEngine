package main

import "sync"

type NullAudioOutput struct {
	mu      sync.Mutex
	started bool
	closed  bool
}

func NewNullAudioOutput() *NullAudioOutput {
	return &NullAudioOutput{}
}

func (n *NullAudioOutput) Start() {
	n.mu.Lock()
	defer n.mu.Unlock()

	if !n.closed {
		n.started = true
	}
}

func (n *NullAudioOutput) Stop() {
	n.mu.Lock()
	defer n.mu.Unlock()

	n.started = false
}

func (n *NullAudioOutput) Close() {
	n.mu.Lock()
	defer n.mu.Unlock()

	n.started = false
	n.closed = true
}

func (n *NullAudioOutput) IsStarted() bool {
	n.mu.Lock()
	defer n.mu.Unlock()

	return n.started
}
