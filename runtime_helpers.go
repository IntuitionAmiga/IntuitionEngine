// runtime_helpers.go - CPU mode detection, factory, and reload closures

/*
 ██▓ ███▄    █ ▄▄▄█████▓ █    ██  ██▓▄▄▄█████▓ ██▓ ▒█████   ███▄    █    ▓█████  ███▄    █   ▄████  ██▓ ███▄    █ ▓█████
▓██▒ ██ ▀█   █ ▓  ██▒ ▓▒ ██  ▓██▒▓██▒▓  ██▒ ▓▒▓██▒▒██▒  ██▒ ██ ▀█   █    ▓█   ▀  ██ ▀█   █  ██▒ ▀█▒▓██▒ ██ ▀█   █ ▓█   ▀
▒██▒▓██  ▀█ ██▒▒ ▓██░ ▒░▓██  ▒██░▒██▒▒ ▓██░ ▒░▒██▒▒██░  ██▒▓██  ▀█ ██▒   ▒███   ▓██  ▀█ ██▒▒██░▄▄▄░▒██▒▓██  ▀█ ██▒▒███
░██░▓██▒  ▐▌██▒░ ▓██▓ ░ ▓▓█  ░██░░██░░ ▓██▓ ░ ░██░▒██   ██░▓██▒  ▐▌██▒   ▒▓█  ▄ ▓██▒  ▐▌██▒░▓█  ██▓░██░▓██▒  ▐▌██▒▒▓█  ▄
░██░▒██░   ▓██░  ▒██▒ ░ ▒▒█████▓ ░██░  ▒██▒ ░ ░██░░ ████▓▒░▒██░   ▓██░   ░▒████▒▒██░   ▓██░░▒▓███▀▒░██░▒██░   ▓██░░▒████▒
░▓  ░ ▒░   ▒ ▒   ▒ ░░   ░▒▓▒ ▒ ▒ ░▓    ▒ ░░   ░▓  ░ ▒░▒░▒░ ░ ▒░   ▒ ▒    ░░ ▒░ ░░ ▒░   ▒ ▒  ░▒   ▒ ░▓  ░ ▒░   ▒ ▒ ░░ ▒░ ░
 ▒ ░░ ░░   ░ ▒░    ░    ░░▒░ ░ ░  ▒ ░    ░     ▒ ░  ░ ▒ ▒░ ░ ░░   ░ ▒░    ░ ░  ░░ ░░   ░ ▒░  ░   ░  ▒ ░░ ░░   ░ ▒░ ░ ░  ░
 ▒ ░   ░   ░ ░   ░       ░░░ ░ ░  ▒ ░  ░       ▒ ░░ ░ ░ ▒     ░   ░ ░       ░      ░   ░ ░ ░ ░   ░  ▒ ░   ░   ░ ░    ░
 ░           ░             ░      ░            ░      ░ ░           ░       ░  ░         ░       ░  ░           ░    ░  ░

(c) 2024 - 2026 Zayn Otley
https://github.com/IntuitionAmiga/IntuitionEngine
License: GPLv3 or later
*/

package main

import (
	"fmt"
	"path/filepath"
	"strings"
)

func modeFromExtension(path string) (string, error) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".ie32", ".iex":
		return "ie32", nil
	case ".ie64":
		return "ie64", nil
	case ".ie65":
		return "6502", nil
	case ".ie68":
		return "m68k", nil
	case ".ie80":
		return "z80", nil
	case ".ie86":
		return "x86", nil
	default:
		return "", fmt.Errorf("unsupported extension: %s", filepath.Ext(path))
	}
}

func createCPURunner(mode string, sysBus *MachineBus, videoChip *VideoChip,
	vgaEngine *VGAEngine, voodooEngine *VoodooEngine) (EmulatorCPU, error) {
	switch mode {
	case "ie32":
		videoChip.SetBigEndianMode(false)
		cpu := NewCPU(sysBus)
		return cpu, nil
	case "ie64":
		videoChip.SetBigEndianMode(false)
		cpu := NewCPU64(sysBus)
		return cpu, nil
	case "m68k":
		videoChip.SetBigEndianMode(true)
		m68k := NewM68KCPU(sysBus)
		return NewM68KRunner(m68k), nil
	case "z80":
		videoChip.SetBigEndianMode(false)
		return NewCPUZ80Runner(sysBus, CPUZ80Config{
			VGAEngine:    vgaEngine,
			VoodooEngine: voodooEngine,
		}), nil
	case "x86":
		videoChip.SetBigEndianMode(false)
		return NewCPUX86Runner(sysBus, &CPUX86Config{
			VGAEngine:    vgaEngine,
			VoodooEngine: voodooEngine,
		}), nil
	case "6502":
		videoChip.SetBigEndianMode(false)
		return NewCPU6502Runner(sysBus, CPU6502Config{}), nil
	default:
		return nil, fmt.Errorf("unsupported CPU mode: %s", mode)
	}
}

func buildReloadClosure(mode string, runner EmulatorCPU, bytes []byte, bus *MachineBus) func() {
	switch mode {
	case "ie32":
		return func() {
			cpu := runner.(*CPU)
			cpu.Reset()
			copy(cpu.memory[PROG_START:], bytes)
		}
	case "ie64":
		return func() {
			cpu := runner.(*CPU64)
			cpu.Reset()
			cpu.LoadProgramBytes(bytes)
		}
	case "m68k":
		return func() {
			r := runner.(*M68KRunner)
			r.cpu.LoadProgramBytes(bytes)
		}
	case "z80":
		return func() {
			r := runner.(*CPUZ80Runner)
			r.cpu.Reset()
			for i, b := range bytes {
				bus.Write8(uint32(r.loadAddr)+uint32(i), b)
			}
			entry := r.entry
			if entry == 0 {
				entry = r.loadAddr
			}
			r.cpu.PC = entry
		}
	case "6502":
		return func() {
			r := runner.(*CPU6502Runner)
			for i, b := range bytes {
				bus.Write8(uint32(r.loadAddr)+uint32(i), b)
			}
			entry := r.entry
			if entry == 0 {
				entry = r.loadAddr
			}
			bus.Write8(RESET_VECTOR, uint8(entry&0x00FF))
			bus.Write8(RESET_VECTOR+1, uint8(entry>>8))
			bus.Write8(NMI_VECTOR, uint8(entry&0x00FF))
			bus.Write8(NMI_VECTOR+1, uint8(entry>>8))
			bus.Write8(IRQ_VECTOR, uint8(entry&0x00FF))
			bus.Write8(IRQ_VECTOR+1, uint8(entry>>8))
			r.cpu.Reset()
			r.cpu.SetRDYLine(true)
		}
	case "x86":
		return func() {
			r := runner.(*CPUX86Runner)
			r.cpu.Reset()
			r.cpu.EIP = r.entry
			// Copy program into bus memory via the adapter
			for i, b := range bytes {
				r.bus.Write(uint32(i), b)
			}
		}
	default:
		return func() {}
	}
}
