//go:build windows || (!linux && !darwin)

package main

func NewUnixArosHostSocketBackend() arosHostSocketBackend { return &disabledArosHostSocketBackend{} }
