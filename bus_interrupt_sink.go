package main

type InterruptMask uint8

const (
	IntMaskVBI InterruptMask = 1 << 0
	IntMaskDLI InterruptMask = 1 << 1
)

type InterruptSink interface {
	Pulse(mask InterruptMask)
}
