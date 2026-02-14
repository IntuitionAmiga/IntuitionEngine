// debug_commands.go - Command parser and handlers for Machine Monitor

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
	"strconv"
	"strings"
)

// MonitorCommand is a parsed command with name and arguments.
type MonitorCommand struct {
	Name string
	Args []string
}

// ParseCommand splits a raw input line into a command name and arguments.
func ParseCommand(input string) MonitorCommand {
	input = strings.TrimSpace(input)
	if input == "" {
		return MonitorCommand{}
	}
	parts := strings.Fields(input)
	return MonitorCommand{
		Name: strings.ToLower(parts[0]),
		Args: parts[1:],
	}
}

// ParseAddress parses a monitor address in various formats:
// $hex, 0xhex, bare hex, #decimal
func ParseAddress(s string) (uint64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}

	// #decimal
	if strings.HasPrefix(s, "#") {
		v, err := strconv.ParseUint(s[1:], 10, 64)
		return v, err == nil
	}

	// $hex
	if strings.HasPrefix(s, "$") {
		v, err := strconv.ParseUint(s[1:], 16, 64)
		return v, err == nil
	}

	// 0x or 0X hex
	if strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X") {
		v, err := strconv.ParseUint(s[2:], 16, 64)
		return v, err == nil
	}

	// bare hex (try hex first)
	v, err := strconv.ParseUint(s, 16, 64)
	return v, err == nil
}

// ExecuteCommand dispatches a parsed command to the appropriate handler.
// Returns true if the monitor should exit.
func (m *MachineMonitor) ExecuteCommand(input string) bool {
	cmd := ParseCommand(input)
	if cmd.Name == "" {
		return false
	}

	// Add to history
	if len(m.history) == 0 || m.history[len(m.history)-1] != input {
		m.history = append(m.history, input)
	}
	m.historyIdx = len(m.history)

	switch cmd.Name {
	case "r":
		return m.cmdRegisters(cmd)
	case "d":
		return m.cmdDisassemble(cmd)
	case "m":
		return m.cmdMemoryDump(cmd)
	case "s":
		return m.cmdStep(cmd)
	case "g":
		return m.cmdGo(cmd)
	case "x":
		return m.cmdExit(cmd)
	case "b":
		return m.cmdBreakpointSet(cmd)
	case "bc":
		return m.cmdBreakpointClear(cmd)
	case "bl":
		return m.cmdBreakpointList(cmd)
	case "f":
		return m.cmdFill(cmd)
	case "h":
		return m.cmdHunt(cmd)
	case "c":
		return m.cmdCompare(cmd)
	case "t":
		return m.cmdTransfer(cmd)
	case "w":
		return m.cmdWrite(cmd)
	case "cpu":
		return m.cmdCPU(cmd)
	case "freeze":
		return m.cmdFreeze(cmd)
	case "thaw":
		return m.cmdThaw(cmd)
	case "fa":
		return m.cmdFreezeAudio(cmd)
	case "ta":
		return m.cmdThawAudio(cmd)
	case "?", "help":
		return m.cmdHelp(cmd)
	default:
		m.appendOutput(fmt.Sprintf("Unknown command: %s", cmd.Name), colorRed)
		return false
	}
}

func (m *MachineMonitor) cmdRegisters(cmd MonitorCommand) bool {
	entry := m.cpus[m.focusedID]
	if entry == nil {
		m.appendOutput("No CPU focused", colorRed)
		return false
	}

	if len(cmd.Args) >= 2 {
		// Set register: r <name> <value>
		name := cmd.Args[0]
		val, ok := ParseAddress(cmd.Args[1])
		if !ok {
			m.appendOutput(fmt.Sprintf("Invalid value: %s", cmd.Args[1]), colorRed)
			return false
		}
		if entry.CPU.SetRegister(name, val) {
			m.appendOutput(fmt.Sprintf("%s = $%X", strings.ToUpper(name), val), colorGreen)
		} else {
			m.appendOutput(fmt.Sprintf("Unknown register: %s", name), colorRed)
		}
		return false
	}

	m.showRegisters()
	return false
}

func (m *MachineMonitor) showRegisters() {
	entry := m.cpus[m.focusedID]
	if entry == nil {
		return
	}

	regs := entry.CPU.GetRegisters()
	addrWidth := entry.CPU.AddressWidth()

	for _, r := range regs {
		color := uint32(colorWhite)
		if prev, ok := m.prevRegs[r.Name]; ok && prev != r.Value {
			color = colorGreen
		}

		var line string
		switch {
		case addrWidth <= 16:
			line = fmt.Sprintf("%-4s $%04X", r.Name, r.Value)
		case addrWidth <= 32 && r.BitWidth <= 32:
			line = fmt.Sprintf("%-4s $%08X", r.Name, r.Value)
		default:
			line = fmt.Sprintf("%-4s $%016X", r.Name, r.Value)
		}
		m.appendOutput(line, color)
	}
}

func (m *MachineMonitor) cmdDisassemble(cmd MonitorCommand) bool {
	entry := m.cpus[m.focusedID]
	if entry == nil {
		m.appendOutput("No CPU focused", colorRed)
		return false
	}

	addr := entry.CPU.GetPC()
	count := 16

	if len(cmd.Args) >= 1 {
		if v, ok := ParseAddress(cmd.Args[0]); ok {
			addr = v
		}
	}
	if len(cmd.Args) >= 2 {
		if v, ok := ParseAddress(cmd.Args[1]); ok {
			count = int(v)
		}
	}

	m.showDisassemblyAt(addr, count)
	return false
}

func (m *MachineMonitor) showDisassembly(addr uint64, count int) {
	entry := m.cpus[m.focusedID]
	if entry == nil {
		return
	}
	if addr == 0 {
		addr = entry.CPU.GetPC()
	}
	m.showDisassemblyAt(addr, count)
}

func (m *MachineMonitor) showDisassemblyAt(addr uint64, count int) {
	entry := m.cpus[m.focusedID]
	if entry == nil {
		return
	}

	lines := entry.CPU.Disassemble(addr, count)
	for _, line := range lines {
		color := uint32(colorWhite)
		prefix := "  "
		if line.IsPC {
			color = colorYellow
			prefix = "> "
		}
		if entry.CPU.HasBreakpoint(line.Address) {
			prefix = "* "
			if !line.IsPC {
				color = colorRed
			}
		}
		addrWidth := entry.CPU.AddressWidth()
		var addrFmt string
		switch {
		case addrWidth <= 16:
			addrFmt = "%04X"
		case addrWidth <= 32:
			addrFmt = "%06X"
		default:
			addrFmt = "%08X"
		}
		text := fmt.Sprintf("%s"+addrFmt+": %-24s %s", prefix, line.Address, line.HexBytes, line.Mnemonic)
		m.appendOutput(text, color)
	}
}

func (m *MachineMonitor) cmdMemoryDump(cmd MonitorCommand) bool {
	entry := m.cpus[m.focusedID]
	if entry == nil {
		m.appendOutput("No CPU focused", colorRed)
		return false
	}

	addr := entry.CPU.GetPC()
	lines := 8

	if len(cmd.Args) >= 1 {
		if v, ok := ParseAddress(cmd.Args[0]); ok {
			addr = v
		}
	}
	if len(cmd.Args) >= 2 {
		if v, ok := ParseAddress(cmd.Args[1]); ok {
			lines = int(v)
		}
	}

	for i := 0; i < lines; i++ {
		data := entry.CPU.ReadMemory(addr, 16)
		if len(data) == 0 {
			break
		}

		var hexParts []string
		var asciiParts []byte
		for j := 0; j < 16; j++ {
			if j < len(data) {
				hexParts = append(hexParts, fmt.Sprintf("%02X", data[j]))
				if data[j] >= 0x20 && data[j] < 0x7F {
					asciiParts = append(asciiParts, data[j])
				} else {
					asciiParts = append(asciiParts, '.')
				}
			} else {
				hexParts = append(hexParts, "  ")
				asciiParts = append(asciiParts, ' ')
			}
		}

		hexStr := strings.Join(hexParts[:8], " ") + "  " + strings.Join(hexParts[8:], " ")
		text := fmt.Sprintf("%06X: %s  %s", addr, hexStr, string(asciiParts))
		m.appendOutput(text, colorWhite)
		addr += 16
	}
	return false
}

func (m *MachineMonitor) cmdStep(cmd MonitorCommand) bool {
	entry := m.cpus[m.focusedID]
	if entry == nil {
		m.appendOutput("No CPU focused", colorRed)
		return false
	}

	count := 1
	if len(cmd.Args) >= 1 {
		if v, ok := ParseAddress(cmd.Args[0]); ok {
			count = int(v)
		}
	}

	totalCycles := 0
	for i := 0; i < count; i++ {
		cycles := entry.CPU.Step()
		totalCycles += cycles
	}

	m.appendOutput(fmt.Sprintf("Step: %d instruction(s), %d cycle(s)", count, totalCycles), colorCyan)

	// Show changed registers
	regs := entry.CPU.GetRegisters()
	for _, r := range regs {
		if prev, ok := m.prevRegs[r.Name]; ok && prev != r.Value {
			m.appendOutput(fmt.Sprintf("  %s: $%X -> $%X", r.Name, prev, r.Value), colorGreen)
		}
	}
	m.saveCurrentRegs()

	// Show next instruction
	m.showDisassembly(0, 1)
	return false
}

func (m *MachineMonitor) cmdGo(cmd MonitorCommand) bool {
	if len(cmd.Args) >= 1 {
		entry := m.cpus[m.focusedID]
		if entry != nil {
			if v, ok := ParseAddress(cmd.Args[0]); ok {
				entry.CPU.SetPC(v)
			}
		}
	}
	return true // exit monitor
}

func (m *MachineMonitor) cmdExit(_ MonitorCommand) bool {
	return true // exit monitor
}

func (m *MachineMonitor) cmdBreakpointSet(cmd MonitorCommand) bool {
	entry := m.cpus[m.focusedID]
	if entry == nil {
		m.appendOutput("No CPU focused", colorRed)
		return false
	}

	if len(cmd.Args) < 1 {
		m.appendOutput("Usage: b <addr>", colorRed)
		return false
	}

	addr, ok := ParseAddress(cmd.Args[0])
	if !ok {
		m.appendOutput(fmt.Sprintf("Invalid address: %s", cmd.Args[0]), colorRed)
		return false
	}

	entry.CPU.SetBreakpoint(addr)
	m.appendOutput(fmt.Sprintf("Breakpoint set at $%X", addr), colorCyan)
	return false
}

func (m *MachineMonitor) cmdBreakpointClear(cmd MonitorCommand) bool {
	entry := m.cpus[m.focusedID]
	if entry == nil {
		m.appendOutput("No CPU focused", colorRed)
		return false
	}

	if len(cmd.Args) < 1 {
		m.appendOutput("Usage: bc <addr> | bc *", colorRed)
		return false
	}

	if cmd.Args[0] == "*" {
		entry.CPU.ClearAllBreakpoints()
		m.appendOutput("All breakpoints cleared", colorCyan)
		return false
	}

	addr, ok := ParseAddress(cmd.Args[0])
	if !ok {
		m.appendOutput(fmt.Sprintf("Invalid address: %s", cmd.Args[0]), colorRed)
		return false
	}

	if entry.CPU.ClearBreakpoint(addr) {
		m.appendOutput(fmt.Sprintf("Breakpoint cleared at $%X", addr), colorCyan)
	} else {
		m.appendOutput(fmt.Sprintf("No breakpoint at $%X", addr), colorRed)
	}
	return false
}

func (m *MachineMonitor) cmdBreakpointList(_ MonitorCommand) bool {
	for _, entry := range m.cpus {
		bps := entry.CPU.ListBreakpoints()
		if len(bps) == 0 {
			continue
		}
		for _, addr := range bps {
			m.appendOutput(fmt.Sprintf("$%X (id:%d %s)", addr, entry.ID, entry.Label), colorCyan)
		}
	}
	return false
}

func (m *MachineMonitor) cmdFill(cmd MonitorCommand) bool {
	entry := m.cpus[m.focusedID]
	if entry == nil {
		m.appendOutput("No CPU focused", colorRed)
		return false
	}

	if len(cmd.Args) < 3 {
		m.appendOutput("Usage: f <start> <end> <byte>", colorRed)
		return false
	}

	start, ok1 := ParseAddress(cmd.Args[0])
	end, ok2 := ParseAddress(cmd.Args[1])
	val, ok3 := ParseAddress(cmd.Args[2])
	if !ok1 || !ok2 || !ok3 {
		m.appendOutput("Invalid argument", colorRed)
		return false
	}

	size := int(end - start + 1)
	if size <= 0 || size > 0x100000 {
		m.appendOutput("Invalid range", colorRed)
		return false
	}

	data := make([]byte, size)
	for i := range data {
		data[i] = byte(val)
	}
	entry.CPU.WriteMemory(start, data)
	m.appendOutput(fmt.Sprintf("Filled $%X-$%X with $%02X", start, end, byte(val)), colorCyan)
	return false
}

func (m *MachineMonitor) cmdHunt(cmd MonitorCommand) bool {
	entry := m.cpus[m.focusedID]
	if entry == nil {
		m.appendOutput("No CPU focused", colorRed)
		return false
	}

	if len(cmd.Args) < 3 {
		m.appendOutput("Usage: h <start> <end> <bytes..>", colorRed)
		return false
	}

	start, ok1 := ParseAddress(cmd.Args[0])
	end, ok2 := ParseAddress(cmd.Args[1])
	if !ok1 || !ok2 {
		m.appendOutput("Invalid argument", colorRed)
		return false
	}

	var pattern []byte
	for _, arg := range cmd.Args[2:] {
		v, ok := ParseAddress(arg)
		if !ok {
			m.appendOutput(fmt.Sprintf("Invalid byte: %s", arg), colorRed)
			return false
		}
		pattern = append(pattern, byte(v))
	}

	found := 0
	for addr := start; addr <= end-uint64(len(pattern))+1; addr++ {
		data := entry.CPU.ReadMemory(addr, len(pattern))
		if len(data) < len(pattern) {
			break
		}
		match := true
		for i := range pattern {
			if data[i] != pattern[i] {
				match = false
				break
			}
		}
		if match {
			m.appendOutput(fmt.Sprintf("Found at $%X", addr), colorCyan)
			found++
			if found >= 256 {
				m.appendOutput("... (truncated)", colorDim)
				break
			}
		}
	}
	if found == 0 {
		m.appendOutput("Not found", colorDim)
	}
	return false
}

func (m *MachineMonitor) cmdCompare(cmd MonitorCommand) bool {
	entry := m.cpus[m.focusedID]
	if entry == nil {
		m.appendOutput("No CPU focused", colorRed)
		return false
	}

	if len(cmd.Args) < 3 {
		m.appendOutput("Usage: c <start> <end> <dest>", colorRed)
		return false
	}

	start, ok1 := ParseAddress(cmd.Args[0])
	end, ok2 := ParseAddress(cmd.Args[1])
	dest, ok3 := ParseAddress(cmd.Args[2])
	if !ok1 || !ok2 || !ok3 {
		m.appendOutput("Invalid argument", colorRed)
		return false
	}

	size := int(end - start + 1)
	if size <= 0 {
		return false
	}

	data1 := entry.CPU.ReadMemory(start, size)
	data2 := entry.CPU.ReadMemory(dest, size)
	diffs := 0
	for i := 0; i < len(data1) && i < len(data2); i++ {
		if data1[i] != data2[i] {
			m.appendOutput(fmt.Sprintf("$%X: %02X != %02X (at $%X)", start+uint64(i), data1[i], data2[i], dest+uint64(i)), colorYellow)
			diffs++
			if diffs >= 256 {
				m.appendOutput("... (truncated)", colorDim)
				break
			}
		}
	}
	if diffs == 0 {
		m.appendOutput("Identical", colorGreen)
	}
	return false
}

func (m *MachineMonitor) cmdTransfer(cmd MonitorCommand) bool {
	entry := m.cpus[m.focusedID]
	if entry == nil {
		m.appendOutput("No CPU focused", colorRed)
		return false
	}

	if len(cmd.Args) < 3 {
		m.appendOutput("Usage: t <start> <end> <dest>", colorRed)
		return false
	}

	start, ok1 := ParseAddress(cmd.Args[0])
	end, ok2 := ParseAddress(cmd.Args[1])
	dest, ok3 := ParseAddress(cmd.Args[2])
	if !ok1 || !ok2 || !ok3 {
		m.appendOutput("Invalid argument", colorRed)
		return false
	}

	size := int(end - start + 1)
	if size <= 0 {
		return false
	}

	data := entry.CPU.ReadMemory(start, size)
	entry.CPU.WriteMemory(dest, data)
	m.appendOutput(fmt.Sprintf("Transferred %d bytes from $%X to $%X", size, start, dest), colorCyan)
	return false
}

func (m *MachineMonitor) cmdWrite(cmd MonitorCommand) bool {
	entry := m.cpus[m.focusedID]
	if entry == nil {
		m.appendOutput("No CPU focused", colorRed)
		return false
	}

	if len(cmd.Args) < 2 {
		m.appendOutput("Usage: w <addr> <bytes..>", colorRed)
		return false
	}

	addr, ok := ParseAddress(cmd.Args[0])
	if !ok {
		m.appendOutput(fmt.Sprintf("Invalid address: %s", cmd.Args[0]), colorRed)
		return false
	}

	var data []byte
	for _, arg := range cmd.Args[1:] {
		v, ok := ParseAddress(arg)
		if !ok {
			m.appendOutput(fmt.Sprintf("Invalid byte: %s", arg), colorRed)
			return false
		}
		data = append(data, byte(v))
	}

	entry.CPU.WriteMemory(addr, data)
	m.appendOutput(fmt.Sprintf("Wrote %d byte(s) at $%X", len(data), addr), colorCyan)
	return false
}

func (m *MachineMonitor) cmdCPU(cmd MonitorCommand) bool {
	if len(cmd.Args) == 0 {
		// List all registered CPUs
		for _, entry := range m.cpus {
			status := "FROZEN"
			if entry.CPU.IsRunning() {
				status = "RUNNING"
			}
			focus := " "
			if entry.ID == m.focusedID {
				focus = "*"
			}
			m.appendOutput(fmt.Sprintf("%sid:%-3d %-12s [%-7s]  PC=$%X",
				focus, entry.ID, entry.Label, status, entry.CPU.GetPC()), colorWhite)
		}
		// List active coprocessor workers
		if m.coprocMgr != nil {
			workers := m.coprocMgr.GetActiveWorkers()
			for _, w := range workers {
				m.appendOutput(fmt.Sprintf(" %-16s [WORKER ]  PC=$%X",
					w.Label, w.CPU.GetPC()), colorDim)
			}
		}
		return false
	}

	// Switch focus by ID or label
	target := cmd.Args[0]

	// Try numeric ID first
	if id, err := strconv.Atoi(target); err == nil {
		if _, ok := m.cpus[id]; ok {
			m.focusedID = id
			m.saveCurrentRegs()
			m.appendOutput(fmt.Sprintf("Focused on id:%d %s", id, m.cpus[id].Label), colorCyan)
			m.showRegisters()
			m.showDisassembly(0, 8)
			return false
		}
		m.appendOutput(fmt.Sprintf("No CPU with id:%d", id), colorRed)
		return false
	}

	// Try label match
	var matches []*CPUEntry
	for _, entry := range m.cpus {
		if strings.EqualFold(entry.Label, target) {
			matches = append(matches, entry)
		}
	}

	if len(matches) == 1 {
		m.focusedID = matches[0].ID
		m.saveCurrentRegs()
		m.appendOutput(fmt.Sprintf("Focused on id:%d %s", matches[0].ID, matches[0].Label), colorCyan)
		m.showRegisters()
		m.showDisassembly(0, 8)
		return false
	}

	if len(matches) > 1 {
		m.appendOutput("Ambiguous label, use ID:", colorRed)
		for _, e := range matches {
			m.appendOutput(fmt.Sprintf("  id:%d %s", e.ID, e.Label), colorWhite)
		}
		return false
	}

	m.appendOutput(fmt.Sprintf("No CPU matching '%s'", target), colorRed)
	return false
}

func (m *MachineMonitor) cmdFreeze(cmd MonitorCommand) bool {
	if len(cmd.Args) < 1 {
		m.appendOutput("Usage: freeze <id|label|*>", colorRed)
		return false
	}

	if cmd.Args[0] == "*" {
		for _, entry := range m.cpus {
			if entry.CPU.IsRunning() {
				entry.CPU.Freeze()
			}
		}
		m.appendOutput("All CPUs frozen", colorCyan)
		return false
	}

	entry := m.findCPUByArg(cmd.Args[0])
	if entry == nil {
		return false
	}

	if entry.CPU.IsRunning() {
		entry.CPU.Freeze()
		m.appendOutput(fmt.Sprintf("Frozen id:%d %s", entry.ID, entry.Label), colorCyan)
	} else {
		m.appendOutput(fmt.Sprintf("id:%d %s already frozen", entry.ID, entry.Label), colorDim)
	}
	return false
}

func (m *MachineMonitor) cmdThaw(cmd MonitorCommand) bool {
	if len(cmd.Args) < 1 {
		m.appendOutput("Usage: thaw <id|label|*>", colorRed)
		return false
	}

	if cmd.Args[0] == "*" {
		for _, entry := range m.cpus {
			if !entry.CPU.IsRunning() {
				entry.CPU.Resume()
			}
		}
		m.appendOutput("All CPUs thawed", colorCyan)
		return false
	}

	entry := m.findCPUByArg(cmd.Args[0])
	if entry == nil {
		return false
	}

	if !entry.CPU.IsRunning() {
		entry.CPU.Resume()
		m.appendOutput(fmt.Sprintf("Thawed id:%d %s", entry.ID, entry.Label), colorCyan)
	} else {
		m.appendOutput(fmt.Sprintf("id:%d %s already running", entry.ID, entry.Label), colorDim)
	}
	return false
}

func (m *MachineMonitor) cmdFreezeAudio(_ MonitorCommand) bool {
	m.audioFrozen = true
	m.appendOutput("Audio frozen", colorCyan)
	return false
}

func (m *MachineMonitor) cmdThawAudio(_ MonitorCommand) bool {
	m.audioFrozen = false
	m.appendOutput("Audio thawed", colorCyan)
	return false
}

func (m *MachineMonitor) cmdHelp(_ MonitorCommand) bool {
	helpLines := []string{
		"Machine Monitor Commands:",
		"  r                  Show registers",
		"  r <name> <value>   Set register",
		"  d [addr] [count]   Disassemble",
		"  m [addr] [count]   Memory dump (hex+ASCII)",
		"  s [count]          Single-step",
		"  g [addr]           Go/continue (exit monitor)",
		"  x                  Exit monitor",
		"  b <addr>           Set breakpoint",
		"  bc <addr|*>        Clear breakpoint(s)",
		"  bl                 List breakpoints",
		"  f <start> <end> <byte>   Fill memory",
		"  w <addr> <bytes..>       Write bytes",
		"  h <start> <end> <bytes..> Hunt/search",
		"  c <start> <end> <dest>   Compare memory",
		"  t <start> <end> <dest>   Transfer/copy memory",
		"  cpu                List CPUs",
		"  cpu <id|label>     Switch focused CPU",
		"  freeze <id|*>      Freeze CPU(s)",
		"  thaw <id|*>        Thaw CPU(s)",
		"  fa / ta            Freeze/thaw audio",
		"",
		"Addresses: $hex, 0xhex, bare hex, #decimal",
	}
	for _, line := range helpLines {
		m.appendOutput(line, colorCyan)
	}
	return false
}

// findCPUByArg resolves an ID or label argument to a CPUEntry.
func (m *MachineMonitor) findCPUByArg(arg string) *CPUEntry {
	if id, err := strconv.Atoi(arg); err == nil {
		if entry, ok := m.cpus[id]; ok {
			return entry
		}
		m.appendOutput(fmt.Sprintf("No CPU with id:%d", id), colorRed)
		return nil
	}

	var matches []*CPUEntry
	for _, entry := range m.cpus {
		if strings.EqualFold(entry.Label, arg) {
			matches = append(matches, entry)
		}
	}

	if len(matches) == 1 {
		return matches[0]
	}
	if len(matches) > 1 {
		m.appendOutput("Ambiguous label, use ID:", colorRed)
		for _, e := range matches {
			m.appendOutput(fmt.Sprintf("  id:%d %s", e.ID, e.Label), colorWhite)
		}
		return nil
	}

	m.appendOutput(fmt.Sprintf("No CPU matching '%s'", arg), colorRed)
	return nil
}
