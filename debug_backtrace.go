// debug_backtrace.go - CPU-specific stack trace / backtrace for Machine Monitor

package main

import "encoding/binary"

// backtrace walks the stack of the focused CPU and returns up to depth return addresses.
func backtrace(cpu DebuggableCPU, depth int) []uint64 {
	switch cpu.CPUName() {
	case "IE64":
		return backtraceIE64(cpu, depth)
	case "IE32":
		return backtraceIE32(cpu, depth)
	case "M68K":
		return backtraceM68K(cpu, depth)
	case "Z80":
		return backtraceZ80(cpu, depth)
	case "6502":
		return backtrace6502(cpu, depth)
	case "X86":
		return backtraceX86(cpu, depth)
	default:
		return nil
	}
}

// backtraceIE64 walks 8-byte stack slots, masking to IE64_ADDR_MASK.
func backtraceIE64(cpu DebuggableCPU, depth int) []uint64 {
	sp, _ := cpu.GetRegister("SP")
	var result []uint64
	for range depth {
		data := cpu.ReadMemory(sp, 8)
		if len(data) < 8 {
			break
		}
		addr := binary.LittleEndian.Uint64(data) & IE64_ADDR_MASK
		result = append(result, addr)
		sp += 8
	}
	return result
}

// backtraceIE32 walks 4-byte stack slots.
func backtraceIE32(cpu DebuggableCPU, depth int) []uint64 {
	sp, _ := cpu.GetRegister("SP")
	var result []uint64
	for range depth {
		data := cpu.ReadMemory(sp, 4)
		if len(data) < 4 {
			break
		}
		addr := uint64(binary.LittleEndian.Uint32(data))
		result = append(result, addr)
		sp += 4
	}
	return result
}

// backtraceM68K walks 4-byte stack slots (A7/SP).
func backtraceM68K(cpu DebuggableCPU, depth int) []uint64 {
	sp, _ := cpu.GetRegister("A7")
	var result []uint64
	for range depth {
		data := cpu.ReadMemory(sp, 4)
		if len(data) < 4 {
			break
		}
		// M68K is big-endian
		addr := uint64(binary.BigEndian.Uint32(data))
		result = append(result, addr)
		sp += 4
	}
	return result
}

// backtraceZ80 walks 2-byte stack slots (little-endian).
func backtraceZ80(cpu DebuggableCPU, depth int) []uint64 {
	sp, _ := cpu.GetRegister("SP")
	var result []uint64
	for range depth {
		data := cpu.ReadMemory(sp, 2)
		if len(data) < 2 {
			break
		}
		addr := uint64(binary.LittleEndian.Uint16(data))
		result = append(result, addr)
		sp += 2
	}
	return result
}

// backtrace6502 walks 2-byte stack slots on page 1. 6502 JSR pushes return-1,
// so we add 1 to each address.
func backtrace6502(cpu DebuggableCPU, depth int) []uint64 {
	sp, _ := cpu.GetRegister("SP")
	// 6502 SP is 8-bit, stack is at 0x0100-0x01FF, grows downward
	sp = 0x0100 + ((sp + 1) & 0xFF) // point to first stacked byte
	var result []uint64
	for range depth {
		if sp > 0x01FF {
			break
		}
		data := cpu.ReadMemory(sp, 2)
		if len(data) < 2 {
			break
		}
		// Low byte first (little-endian), then add 1 because JSR pushes return-1
		addr := uint64(binary.LittleEndian.Uint16(data)) + 1
		result = append(result, addr)
		sp += 2
	}
	return result
}

// backtraceX86 walks 4-byte stack slots.
func backtraceX86(cpu DebuggableCPU, depth int) []uint64 {
	sp, _ := cpu.GetRegister("ESP")
	var result []uint64
	for range depth {
		data := cpu.ReadMemory(sp, 4)
		if len(data) < 4 {
			break
		}
		addr := uint64(binary.LittleEndian.Uint32(data))
		result = append(result, addr)
		sp += 4
	}
	return result
}
