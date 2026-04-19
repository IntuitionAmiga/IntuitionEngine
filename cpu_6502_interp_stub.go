//go:build !amd64 || !linux

package main

func (cpu_6502 *CPU_6502) executeOptimizedInterpreter() {
	cpu_6502.ExecuteFast()
}
