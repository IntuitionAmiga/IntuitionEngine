package main

import (
	"fmt"
	"strconv"
	"strings"

	ie64asm "github.com/intuitionamiga/IntuitionEngine/internal/asm/ie64"
)

const monitorAssembleScriptError = "monitor assemble mode is interactive only and cannot be used from scripts or macros"

func (m *MachineMonitor) currentPrompt() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.currentPromptLocked()
}

func (m *MachineMonitor) currentPromptLocked() string {
	if m.assembleMode {
		return fmt.Sprintf("asm $%016X> ", m.assembleAddr)
	}
	return "> "
}

func (m *MachineMonitor) clearAssembleModeLocked() {
	m.assembleMode = false
	m.assembleAddr = 0
}

func (m *MachineMonitor) cmdAssemble(cmd MonitorCommand) bool {
	if m.scriptDepth > 0 {
		m.appendOutput(monitorAssembleScriptError, colorRed)
		return false
	}
	entry := m.cpus[m.focusedID]
	if entry == nil || !strings.EqualFold(entry.CPU.CPUName(), "IE64") {
		m.appendOutput("monitor assembly is IE64-only", colorRed)
		return false
	}
	if len(cmd.Args) != 1 {
		m.appendOutput("Usage: A <addr>", colorRed)
		return false
	}
	addr, err := parseMonitorAssembleAddress(cmd.Args[0])
	if err != nil {
		m.appendOutput(fmt.Sprintf("Invalid assemble address %q: %s", cmd.Args[0], err), colorRed)
		return false
	}
	m.assembleMode = true
	m.assembleAddr = addr
	m.appendOutput(fmt.Sprintf("IE64 assemble at $%016X; empty line exits", addr), colorCyan)
	return false
}

func parseMonitorAssembleAddress(text string) (uint64, error) {
	s := strings.TrimSpace(text)
	if s == "" {
		return 0, fmt.Errorf("empty address")
	}
	base := 16
	digits := s
	switch {
	case strings.HasPrefix(s, "#"):
		base = 10
		digits = s[1:]
	case strings.HasPrefix(s, "$"):
		base = 16
		digits = s[1:]
	case strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X"):
		base = 16
		digits = s[2:]
	case strings.HasPrefix(s, "-"):
		return 0, fmt.Errorf("negative addresses are not supported")
	}
	if digits == "" {
		return 0, fmt.Errorf("missing digits")
	}
	value, err := strconv.ParseUint(strings.ReplaceAll(digits, "_", ""), base, 64)
	if err != nil {
		if numErr, ok := err.(*strconv.NumError); ok && numErr.Err == strconv.ErrRange {
			return 0, fmt.Errorf("overflow parsing unsigned 64-bit address")
		}
		return 0, err
	}
	return value, nil
}

func (m *MachineMonitor) executeAssembleModeInput(input string) bool {
	if strings.TrimSpace(input) == "" {
		m.clearAssembleModeLocked()
		m.appendOutput("Exited IE64 assemble mode", colorDim)
		return false
	}
	if m.scriptDepth > 0 {
		m.appendOutput(monitorAssembleScriptError, colorRed)
		return false
	}
	if cmd := ParseCommand(input); cmd.Name == "cpu" {
		m.clearAssembleModeLocked()
		return m.executeCommand(input)
	}
	entry := m.cpus[m.focusedID]
	if entry == nil || !strings.EqualFold(entry.CPU.CPUName(), "IE64") {
		m.clearAssembleModeLocked()
		m.appendOutput("monitor assembly is IE64-only", colorRed)
		return false
	}

	addr := m.assembleAddr
	result := ie64asm.AssembleInstruction(addr, input)
	if len(result.Diagnostics) > 0 {
		for _, diag := range result.Diagnostics {
			if diag.Column > 0 {
				m.appendOutput(fmt.Sprintf("asm: col %d: %s", diag.Column, diag.Message), colorRed)
			} else {
				m.appendOutput("asm: "+diag.Message, colorRed)
			}
		}
		return false
	}
	if len(result.Bytes) != 8 {
		m.appendOutput("asm: no instruction assembled", colorRed)
		return false
	}

	debugCPU, ok := entry.CPU.(*DebugIE64)
	if !ok || debugCPU.cpu == nil {
		m.appendOutput("IE64 debug adapter unavailable for monitor assembly", colorRed)
		return false
	}
	if err := m.writeAssembledIE64RAMLocked(debugCPU.cpu, addr, result.Bytes); err != nil {
		m.appendOutput(fmt.Sprintf("asm write failed at $%016X: %s", addr, err), colorRed)
		return false
	}

	disasm := disassembleIE64(func(readAddr uint64, size int) []byte {
		if readAddr != addr || size != 8 {
			return nil
		}
		return append([]byte(nil), result.Bytes...)
	}, addr, 1)
	mnemonic := strings.TrimSpace(input)
	if len(disasm) == 1 {
		mnemonic = disasm[0].Mnemonic
	}
	hexBytes := formatBytes8(result.Bytes)
	m.appendOutput(fmt.Sprintf("$%016X: %s  %s", addr, hexBytes, mnemonic), colorGreen)
	m.assembleAddr += uint64(len(result.Bytes))
	return false
}

func (m *MachineMonitor) writeAssembledIE64RAMLocked(focused *CPU64, addr uint64, data []byte) error {
	if focused == nil {
		return fmt.Errorf("IE64 CPU unavailable")
	}
	if focused.IsRunning() {
		return fmt.Errorf("IE64 CPU must be stopped before monitor code writes")
	}
	if len(data) == 0 {
		return fmt.Errorf("no bytes to write")
	}
	if focused.bus == nil {
		return fmt.Errorf("machine bus unavailable")
	}
	if err := focused.bus.WritePhysRAMOnly(addr, data); err != nil {
		return err
	}
	m.flushAllIE64JITFullLocked()
	return nil
}

func (m *MachineMonitor) flushAllIE64JITFullLocked() {
	seen := make(map[*CPU64]bool)
	for _, entry := range m.cpus {
		debugCPU, ok := entry.CPU.(*DebugIE64)
		if !ok || debugCPU.cpu == nil || seen[debugCPU.cpu] {
			continue
		}
		seen[debugCPU.cpu] = true
		debugCPU.cpu.FlushIE64JITFull()
	}
}

func formatBytes8(data []byte) string {
	parts := make([]string, 0, len(data))
	for _, b := range data {
		parts = append(parts, fmt.Sprintf("%02X", b))
	}
	return strings.Join(parts, " ")
}
