package main

import "unsafe"

// These are the only CPU fields addressed directly by generated IE32 code.
// Keep them as compile-time constants: changing CPU layout then requires the
// backend ABI checks below to compile before any emitter can use the offset.
const (
	ie32JITABIPC               = unsafe.Offsetof(CPU{}.PC)
	ie32JITABISP               = unsafe.Offsetof(CPU{}.SP)
	ie32JITABIA                = unsafe.Offsetof(CPU{}.A)
	ie32JITABIInterruptEnabled = unsafe.Offsetof(CPU{}.interruptEnabled)
	ie32JITABIInInterrupt      = unsafe.Offsetof(CPU{}.inInterrupt)
	ie32JITABIMemory           = unsafe.Offsetof(CPU{}.memory)
	ie32JITABIMemBase          = unsafe.Offsetof(CPU{}.memBase)
)

// Native emitters use 32-bit word accesses and signed 32-bit displacements.
// Negative array sizes are compile-time errors, making ABI drift fail builds.
var (
	_ [0 - int(ie32JITABIPC%4)]byte
	_ [0 - int(ie32JITABISP%4)]byte
	_ [0 - int(ie32JITABIA%4)]byte
	_ [0 - int(ie32JITABIInterruptEnabled%4)]byte
	_ [0 - int(ie32JITABIInInterrupt%4)]byte
	_ [0 - int(ie32JITABIMemory%4)]byte
	_ [0 - int(ie32JITABIMemBase%unsafe.Alignof(uintptr(0)))]byte
	_ [0 - int(ie32JITABIPC>>31)]byte
	_ [0 - int(ie32JITABISP>>31)]byte
	_ [0 - int(ie32JITABIA>>31)]byte
	_ [0 - int(ie32JITABIInterruptEnabled>>31)]byte
	_ [0 - int(ie32JITABIInInterrupt>>31)]byte
	_ [0 - int(ie32JITABIMemory>>31)]byte
	_ [0 - int(ie32JITABIMemBase>>31)]byte
)
