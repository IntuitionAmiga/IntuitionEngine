// debug_backtrace.go - CPU-specific stack trace / backtrace for Machine Monitor

package main

import "encoding/binary"

type BacktraceFrame struct {
	Address       uint64
	Symbol        SymbolResolution
	HasSymbol     bool
	LowConfidence bool
}

// backtrace walks the stack of the focussed CPU and returns up to depth return addresses.
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

// backtraceIE64 walks 8-byte stack slots and reports each frame's full
// 64-bit return address. The legacy IE64_ADDR_MASK 25-bit/32 MB mask was
// retired by PLAN_MAX_RAM.md slice 3; debug formatting now preserves the
// full virtual/physical address so traces above 4 GiB are readable.
func backtraceIE64(cpu DebuggableCPU, depth int) []uint64 {
	sp, _ := cpu.GetRegister("SP")
	var result []uint64
	for range depth {
		data := cpu.ReadMemory(sp, 8)
		if len(data) < 8 {
			break
		}
		addr := binary.LittleEndian.Uint64(data)
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

// backtraceM68K follows the conventional A6 frame-link chain.
func backtraceM68K(cpu DebuggableCPU, depth int) []uint64 {
	a6, ok := cpu.GetRegister("A6")
	if !ok || a6 == 0 {
		return backtraceM68KStackScan(cpu, depth)
	}
	var result []uint64
	for range depth {
		frame := cpu.ReadMemory(a6, 8)
		if len(frame) < 8 {
			break
		}
		prevA6 := uint64(binary.BigEndian.Uint32(frame[0:4]))
		ret := uint64(binary.BigEndian.Uint32(frame[4:8]))
		if ret != 0 {
			result = append(result, ret)
		}
		if prevA6 == 0 || prevA6 == a6 {
			break
		}
		a6 = prevA6
	}
	if len(result) == 0 {
		return backtraceM68KStackScan(cpu, depth)
	}
	return result
}

func backtraceM68KStackScan(cpu DebuggableCPU, depth int) []uint64 {
	sp, ok := cpu.GetRegister("A7")
	if !ok {
		if ssp, sspOK := cpu.GetRegister("SSP"); sspOK {
			sp = ssp
		} else {
			return nil
		}
	}
	var result []uint64
	for range depth {
		data := cpu.ReadMemory(sp, 4)
		if len(data) < 4 {
			break
		}
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
	if framed := backtraceX86EBP(cpu, depth); len(framed) > 0 {
		return framed
	}
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

func backtraceX86EBP(cpu DebuggableCPU, depth int) []uint64 {
	ebp, ok := cpu.GetRegister("EBP")
	if !ok || ebp == 0 {
		return nil
	}
	var result []uint64
	seen := make(map[uint64]bool)
	for range depth {
		if seen[ebp] {
			break
		}
		seen[ebp] = true
		frame := cpu.ReadMemory(ebp, 8)
		if len(frame) < 8 {
			break
		}
		next := uint64(binary.LittleEndian.Uint32(frame[0:4]))
		ret := uint64(binary.LittleEndian.Uint32(frame[4:8]))
		if ret != 0 {
			result = append(result, ret)
		}
		if next == 0 || next <= ebp {
			break
		}
		ebp = next
	}
	return result
}

func symbolAwareBacktrace(cpu DebuggableCPU, depth int, symbols *SymbolTable, regions *RegionRegistry) []BacktraceFrame {
	addrs := backtrace(cpu, depth)
	if len(addrs) == 0 {
		return nil
	}
	cpuName := cpu.CPUName()
	hasSymbols := symbols != nil && len(symbols.List(cpuName)) > 0
	frames := make([]BacktraceFrame, 0, len(addrs))
	for _, addr := range addrs {
		if addr == 0 {
			continue
		}
		frame := BacktraceFrame{Address: addr, LowConfidence: cpuName == "6502"}
		if hasSymbols && !backtraceAddressAligned(cpuName, addr) {
			continue
		}
		if symbols != nil {
			if res, ok := symbols.Resolve(cpuName, addr); ok {
				frame.Symbol = res
				frame.HasSymbol = true
			} else if hasSymbols {
				continue
			}
		}
		if hasSymbols && !backtraceRegionLooksExecutable(cpuName, addr, regions) {
			continue
		}
		frames = append(frames, frame)
	}
	return frames
}

func backtraceAddressAligned(cpu string, addr uint64) bool {
	switch cpu {
	case "IE64":
		return addr%8 == 0
	case "IE32", "M68K", "X86":
		return addr%2 == 0
	default:
		return true
	}
}

func backtraceRegionLooksExecutable(cpu string, addr uint64, regions *RegionRegistry) bool {
	if regions == nil {
		return true
	}
	region := regions.Lookup(cpu, addr)
	if region == nil {
		return true
	}
	switch region.Kind {
	case RegionMMIO, RegionStack, RegionVRAM:
		return false
	default:
		return true
	}
}
