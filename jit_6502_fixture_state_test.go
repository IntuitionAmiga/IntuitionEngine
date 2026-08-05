package main

import "fmt"

func p65FixtureCPUState(cpu *CPU_6502) string {
	return fmt.Sprintf("PC=$%04X SP=$%02X A=$%02X X=$%02X Y=$%02X SR=$%02X cycles=%d",
		cpu.PC, cpu.SP, cpu.A, cpu.X, cpu.Y, cpu.SR, cpu.Cycles)
}
