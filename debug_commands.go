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
	"os"
	"slices"
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

// EvalAddress evaluates a simple expression: <term> [+|- <term>]*
// Each term is either a register name or a numeric address.
func EvalAddress(expr string, cpu DebuggableCPU) (uint64, bool) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return 0, false
	}

	// Tokenize: split on + and - while preserving operators
	type token struct {
		text string
		op   byte // 0 for first term, '+' or '-'
	}

	var tokens []token
	current := strings.Builder{}
	currentOp := byte(0)

	for i := 0; i < len(expr); i++ {
		ch := expr[i]
		if (ch == '+' || ch == '-') && i > 0 {
			t := strings.TrimSpace(current.String())
			if t != "" {
				tokens = append(tokens, token{text: t, op: currentOp})
			}
			currentOp = ch
			current.Reset()
		} else {
			current.WriteByte(ch)
		}
	}
	t := strings.TrimSpace(current.String())
	if t != "" {
		tokens = append(tokens, token{text: t, op: currentOp})
	}

	if len(tokens) == 0 {
		return 0, false
	}

	var result uint64
	for _, tok := range tokens {
		var val uint64
		var ok bool

		// Try register name first (if CPU available)
		if cpu != nil {
			val, ok = cpu.GetRegister(strings.ToUpper(tok.text))
		}
		if !ok {
			val, ok = ParseAddress(tok.text)
		}
		if !ok {
			return 0, false
		}

		switch tok.op {
		case 0, '+':
			result += val
		case '-':
			result -= val
		}
	}

	return result, true
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
	case "save":
		return m.cmdSaveMemory(cmd)
	case "load":
		return m.cmdLoadMemory(cmd)
	case "u":
		return m.cmdRunUntil(cmd)
	case "ww":
		return m.cmdWatchpointSet(cmd)
	case "wc":
		return m.cmdWatchpointClear(cmd)
	case "wl":
		return m.cmdWatchpointList(cmd)
	case "bt":
		return m.cmdBacktrace(cmd)
	case "ss":
		return m.cmdSaveState(cmd)
	case "sl":
		return m.cmdLoadState(cmd)
	case "trace":
		return m.cmdTrace(cmd)
	case "bs":
		return m.cmdBackstep(cmd)
	case "io":
		return m.cmdIOView(cmd)
	case "e":
		return m.cmdHexEdit(cmd)
	case "script":
		return m.cmdScript(cmd)
	case "macro":
		return m.cmdMacro(cmd)
	case "?", "help":
		return m.cmdHelp(cmd)
	default:
		// Check for macro invocation
		if cmds, ok := m.macros[cmd.Name]; ok {
			return m.executeMacro(cmds)
		}
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
		if v, ok := EvalAddress(cmd.Args[0], entry.CPU); ok {
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

	// Build set of addresses in the visible window for branch target markers
	addrSet := make(map[uint64]bool, len(lines))
	for _, line := range lines {
		addrSet[line.Address] = true
	}
	// Mark which addresses are targeted by branches in the window
	targetSet := make(map[uint64]bool)
	for _, line := range lines {
		if line.IsBranch && line.BranchTarget != 0 && addrSet[line.BranchTarget] {
			targetSet[line.BranchTarget] = true
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
		if targetSet[line.Address] {
			prefix = "T "
		}

		// Branch annotation suffix
		suffix := ""
		if line.IsBranch && line.BranchTarget != 0 {
			if line.BranchTarget < line.Address {
				suffix = " <- LOOP"
				if color == colorWhite {
					color = colorMagenta
				}
			}
		}

		text := fmt.Sprintf("%s"+addrFmt+": %-24s %s%s", prefix, line.Address, line.HexBytes, line.Mnemonic, suffix)
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
		if v, ok := EvalAddress(cmd.Args[0], entry.CPU); ok {
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
		for j := range 16 {
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

	// Snapshot before stepping (for backstep) — per-CPU history
	snap := TakeSnapshot(entry.CPU)
	cpuID := m.focusedID
	m.stepHistory[cpuID] = append(m.stepHistory[cpuID], snap)
	if len(m.stepHistory[cpuID]) > m.maxBackstep {
		m.stepHistory[cpuID] = m.stepHistory[cpuID][len(m.stepHistory[cpuID])-m.maxBackstep:]
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
			if v, ok := EvalAddress(cmd.Args[0], entry.CPU); ok {
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
		m.appendOutput("Usage: b <addr> [condition]", colorRed)
		return false
	}

	addr, ok := EvalAddress(cmd.Args[0], entry.CPU)
	if !ok {
		m.appendOutput(fmt.Sprintf("Invalid address: %s", cmd.Args[0]), colorRed)
		return false
	}

	// Optional condition
	if len(cmd.Args) >= 2 {
		condStr := strings.Join(cmd.Args[1:], " ")
		cond, err := ParseCondition(condStr)
		if err != nil {
			m.appendOutput(fmt.Sprintf("Invalid condition: %s", err), colorRed)
			return false
		}
		entry.CPU.SetConditionalBreakpoint(addr, cond)
		m.appendOutput(fmt.Sprintf("Breakpoint set at $%X if %s", addr, FormatCondition(cond)), colorCyan)
	} else {
		entry.CPU.SetBreakpoint(addr)
		m.appendOutput(fmt.Sprintf("Breakpoint set at $%X", addr), colorCyan)
	}
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
		// Discard any saved conditions for this CPU since the breakpoints are gone
		delete(m.savedConditions, m.focusedID)
		m.appendOutput("All breakpoints cleared", colorCyan)
		return false
	}

	addr, ok := ParseAddress(cmd.Args[0])
	if !ok {
		m.appendOutput(fmt.Sprintf("Invalid address: %s", cmd.Args[0]), colorRed)
		return false
	}

	if entry.CPU.ClearBreakpoint(addr) {
		// Discard any saved condition for this address
		if saved, ok := m.savedConditions[m.focusedID]; ok {
			delete(saved, addr)
			if len(saved) == 0 {
				delete(m.savedConditions, m.focusedID)
			}
		}
		m.appendOutput(fmt.Sprintf("Breakpoint cleared at $%X", addr), colorCyan)
	} else {
		m.appendOutput(fmt.Sprintf("No breakpoint at $%X", addr), colorRed)
	}
	return false
}

func (m *MachineMonitor) cmdBreakpointList(_ MonitorCommand) bool {
	for _, entry := range m.cpus {
		cbps := entry.CPU.ListConditionalBreakpoints()
		if len(cbps) == 0 {
			continue
		}
		for _, bp := range cbps {
			condStr := ""
			if bp.Condition != nil {
				condStr = " if " + FormatCondition(bp.Condition)
			}
			hitStr := ""
			if bp.HitCount > 0 {
				hitStr = fmt.Sprintf(" (hits:%d)", bp.HitCount)
			}
			m.appendOutput(fmt.Sprintf("$%X%s%s (id:%d %s)", bp.Address, condStr, hitStr, entry.ID, entry.Label), colorCyan)
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
	if m.soundChip == nil {
		m.appendOutput("No sound chip available", colorRed)
		return false
	}
	m.soundChip.audioFrozen.Store(true)
	m.appendOutput("Audio frozen", colorCyan)
	return false
}

func (m *MachineMonitor) cmdThawAudio(_ MonitorCommand) bool {
	if m.soundChip == nil {
		m.appendOutput("No sound chip available", colorRed)
		return false
	}
	m.soundChip.audioFrozen.Store(false)
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
		"  bs                 Backstep (undo last step, CPU+memory)",
		"  g [addr]           Go/continue (exit monitor)",
		"  u <addr>           Run until address",
		"  x                  Exit monitor",
		"  b <addr> [cond]    Set breakpoint (optional condition)",
		"  bc <addr|*>        Clear breakpoint(s)",
		"  bl                 List breakpoints",
		"  ww <addr>          Set write watchpoint",
		"  wc <addr|*>        Clear watchpoint(s)",
		"  wl                 List watchpoints",
		"  bt [depth]         Stack backtrace",
		"  f <start> <end> <byte>   Fill memory",
		"  w <addr> <bytes..>       Write bytes",
		"  h <start> <end> <bytes..> Hunt/search",
		"  c <start> <end> <dest>   Compare memory",
		"  t <start> <end> <dest>   Transfer/copy memory",
		"  save <s> <e> <file>  Save memory to file",
		"  load <file> <addr>   Load file into memory",
		"  ss [file]          Save machine state",
		"  sl [file]          Load machine state",
		"  trace <count>      Trace N instructions",
		"  trace file <path|off>  Set/stop trace file output",
		"  trace watch add/del/list <addr>  Write tracking",
		"  trace history show/clear <addr>  Write history",
		"  io [device]        I/O register viewer",
		"  e <addr>           Hex editor mode",
		"  script <file>      Run command script",
		"  macro <name> <cmds..> Define macro (;-separated)",
		"  cpu                List CPUs",
		"  cpu <id|label>     Switch focused CPU",
		"  freeze <id|*>      Freeze CPU(s)",
		"  thaw <id|*>        Thaw CPU(s)",
		"  fa / ta            Freeze/thaw audio",
		"",
		"Addresses: $hex, 0xhex, bare hex, #decimal, expr+expr",
		"Conditions: reg==val, [$addr]==val, hitcount>val",
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

// --- Feature 1: Export/Import Memory ---

func (m *MachineMonitor) cmdSaveMemory(cmd MonitorCommand) bool {
	entry := m.cpus[m.focusedID]
	if entry == nil {
		m.appendOutput("No CPU focused", colorRed)
		return false
	}

	if len(cmd.Args) < 3 {
		m.appendOutput("Usage: save <start> <end> <filename>", colorRed)
		return false
	}

	start, ok1 := EvalAddress(cmd.Args[0], entry.CPU)
	end, ok2 := EvalAddress(cmd.Args[1], entry.CPU)
	if !ok1 || !ok2 {
		m.appendOutput("Invalid address", colorRed)
		return false
	}
	if end < start {
		m.appendOutput("End must be >= start", colorRed)
		return false
	}

	size := int(end - start + 1)
	if size > 32*1024*1024 {
		m.appendOutput("Range too large (max 32MB)", colorRed)
		return false
	}

	data := entry.CPU.ReadMemory(start, size)
	if err := os.WriteFile(cmd.Args[2], data, 0644); err != nil {
		m.appendOutput(fmt.Sprintf("Error: %s", err), colorRed)
		return false
	}

	m.appendOutput(fmt.Sprintf("Saved %d bytes ($%X-$%X) to %s", size, start, end, cmd.Args[2]), colorCyan)
	return false
}

func (m *MachineMonitor) cmdLoadMemory(cmd MonitorCommand) bool {
	entry := m.cpus[m.focusedID]
	if entry == nil {
		m.appendOutput("No CPU focused", colorRed)
		return false
	}

	if len(cmd.Args) < 2 {
		m.appendOutput("Usage: load <filename> <addr>", colorRed)
		return false
	}

	data, err := os.ReadFile(cmd.Args[0])
	if err != nil {
		m.appendOutput(fmt.Sprintf("Error: %s", err), colorRed)
		return false
	}

	if len(data) > 32*1024*1024 {
		m.appendOutput("File too large (max 32MB)", colorRed)
		return false
	}

	addr, ok := EvalAddress(cmd.Args[1], entry.CPU)
	if !ok {
		m.appendOutput(fmt.Sprintf("Invalid address: %s", cmd.Args[1]), colorRed)
		return false
	}

	entry.CPU.WriteMemory(addr, data)
	m.appendOutput(fmt.Sprintf("Loaded %d bytes from %s to $%X", len(data), cmd.Args[0], addr), colorCyan)
	return false
}

// --- Feature 2: Run-Until ---

func (m *MachineMonitor) cmdRunUntil(cmd MonitorCommand) bool {
	entry := m.cpus[m.focusedID]
	if entry == nil {
		m.appendOutput("No CPU focused", colorRed)
		return false
	}

	if len(cmd.Args) < 1 {
		m.appendOutput("Usage: u <addr>", colorRed)
		return false
	}

	addr, ok := EvalAddress(cmd.Args[0], entry.CPU)
	if !ok {
		m.appendOutput(fmt.Sprintf("Invalid address: %s", cmd.Args[0]), colorRed)
		return false
	}

	existingBP := entry.CPU.GetConditionalBreakpoint(addr)
	if existingBP == nil {
		// No breakpoint exists — create a temp unconditional one
		entry.CPU.SetBreakpoint(addr)
		if m.tempBreakpoints[m.focusedID] == nil {
			m.tempBreakpoints[m.focusedID] = make(map[uint64]bool)
		}
		m.tempBreakpoints[m.focusedID][addr] = true
	} else if existingBP.Condition != nil {
		// A conditional breakpoint exists — temporarily make it unconditional
		// so run-until always stops. Save the original condition for restore.
		if m.savedConditions[m.focusedID] == nil {
			m.savedConditions[m.focusedID] = make(map[uint64]*BreakpointCondition)
		}
		m.savedConditions[m.focusedID][addr] = existingBP.Condition
		existingBP.Condition = nil
	}
	// else: unconditional breakpoint already exists — it will fire on its own

	m.appendOutput(fmt.Sprintf("Run until $%X", addr), colorCyan)
	return true // exit monitor to resume execution
}

// --- Feature 5: Watchpoints ---

func (m *MachineMonitor) cmdWatchpointSet(cmd MonitorCommand) bool {
	entry := m.cpus[m.focusedID]
	if entry == nil {
		m.appendOutput("No CPU focused", colorRed)
		return false
	}

	if len(cmd.Args) < 1 {
		m.appendOutput("Usage: ww <addr>", colorRed)
		return false
	}

	addr, ok := EvalAddress(cmd.Args[0], entry.CPU)
	if !ok {
		m.appendOutput(fmt.Sprintf("Invalid address: %s", cmd.Args[0]), colorRed)
		return false
	}

	entry.CPU.SetWatchpoint(addr)
	m.appendOutput(fmt.Sprintf("Write watchpoint set at $%X", addr), colorCyan)
	return false
}

func (m *MachineMonitor) cmdWatchpointClear(cmd MonitorCommand) bool {
	entry := m.cpus[m.focusedID]
	if entry == nil {
		m.appendOutput("No CPU focused", colorRed)
		return false
	}

	if len(cmd.Args) < 1 {
		m.appendOutput("Usage: wc <addr|*>", colorRed)
		return false
	}

	if cmd.Args[0] == "*" {
		entry.CPU.ClearAllWatchpoints()
		m.appendOutput("All watchpoints cleared", colorCyan)
		return false
	}

	addr, ok := EvalAddress(cmd.Args[0], entry.CPU)
	if !ok {
		m.appendOutput(fmt.Sprintf("Invalid address: %s", cmd.Args[0]), colorRed)
		return false
	}

	if entry.CPU.ClearWatchpoint(addr) {
		m.appendOutput(fmt.Sprintf("Watchpoint cleared at $%X", addr), colorCyan)
	} else {
		m.appendOutput(fmt.Sprintf("No watchpoint at $%X", addr), colorRed)
	}
	return false
}

func (m *MachineMonitor) cmdWatchpointList(_ MonitorCommand) bool {
	for _, entry := range m.cpus {
		wps := entry.CPU.ListWatchpoints()
		if len(wps) == 0 {
			continue
		}
		for _, addr := range wps {
			m.appendOutput(fmt.Sprintf("W $%X (id:%d %s)", addr, entry.ID, entry.Label), colorCyan)
		}
	}
	return false
}

// --- Feature 6: Stack Trace / Backtrace ---

func (m *MachineMonitor) cmdBacktrace(cmd MonitorCommand) bool {
	entry := m.cpus[m.focusedID]
	if entry == nil {
		m.appendOutput("No CPU focused", colorRed)
		return false
	}

	depth := 16
	if len(cmd.Args) >= 1 {
		if v, ok := ParseAddress(cmd.Args[0]); ok {
			depth = int(v)
		}
	}

	addrs := backtrace(entry.CPU, depth)
	if len(addrs) == 0 {
		m.appendOutput("No stack frames found", colorDim)
		return false
	}

	for i, addr := range addrs {
		m.appendOutput(fmt.Sprintf("#%-3d $%06X", i, addr), colorCyan)
	}
	return false
}

// --- Feature 7: Save/Load State ---

func (m *MachineMonitor) cmdSaveState(cmd MonitorCommand) bool {
	entry := m.cpus[m.focusedID]
	if entry == nil {
		m.appendOutput("No CPU focused", colorRed)
		return false
	}

	filename := "snapshot.iem"
	if len(cmd.Args) >= 1 {
		filename = cmd.Args[0]
	}

	snap := TakeSnapshot(entry.CPU)
	if err := SaveSnapshotToFile(snap, filename); err != nil {
		m.appendOutput(fmt.Sprintf("Error: %s", err), colorRed)
		return false
	}

	m.appendOutput(fmt.Sprintf("State saved to %s (CPU+memory)", filename), colorCyan)
	return false
}

func (m *MachineMonitor) cmdLoadState(cmd MonitorCommand) bool {
	entry := m.cpus[m.focusedID]
	if entry == nil {
		m.appendOutput("No CPU focused", colorRed)
		return false
	}

	filename := "snapshot.iem"
	if len(cmd.Args) >= 1 {
		filename = cmd.Args[0]
	}

	snap, err := LoadSnapshotFromFile(filename)
	if err != nil {
		m.appendOutput(fmt.Sprintf("Error: %s", err), colorRed)
		return false
	}

	RestoreSnapshot(entry.CPU, snap)
	m.saveCurrentRegs()
	m.appendOutput(fmt.Sprintf("State loaded from %s (CPU+memory)", filename), colorCyan)
	m.showRegisters()
	m.showDisassembly(0, 8)
	return false
}

// --- Feature 8: Trace/Logging + Write History ---

func (m *MachineMonitor) cmdTrace(cmd MonitorCommand) bool {
	if len(cmd.Args) < 1 {
		m.appendOutput("Usage: trace <count> | trace file <path|off>", colorRed)
		m.appendOutput("       trace watch add/del/list <addr>", colorRed)
		m.appendOutput("       trace history show/clear <addr|*>", colorRed)
		return false
	}

	sub := strings.ToLower(cmd.Args[0])

	switch sub {
	case "file":
		return m.cmdTraceFile(cmd)
	case "watch":
		return m.cmdTraceWatch(cmd)
	case "history":
		return m.cmdTraceHistory(cmd)
	default:
		return m.cmdTraceRun(cmd)
	}
}

func (m *MachineMonitor) cmdTraceFile(cmd MonitorCommand) bool {
	if len(cmd.Args) < 2 {
		m.appendOutput("Usage: trace file <path|off>", colorRed)
		return false
	}

	if strings.ToLower(cmd.Args[1]) == "off" {
		if m.traceFile != nil {
			m.traceFile.Close()
			m.traceFile = nil
		}
		m.appendOutput("Trace file output stopped", colorCyan)
		return false
	}

	if m.traceFile != nil {
		m.traceFile.Close()
	}

	f, err := os.Create(cmd.Args[1])
	if err != nil {
		m.appendOutput(fmt.Sprintf("Error: %s", err), colorRed)
		return false
	}
	m.traceFile = f
	m.appendOutput(fmt.Sprintf("Trace output to %s", cmd.Args[1]), colorCyan)
	return false
}

func (m *MachineMonitor) cmdTraceWatch(cmd MonitorCommand) bool {
	if len(cmd.Args) < 2 {
		m.appendOutput("Usage: trace watch add/del/list [addr]", colorRed)
		return false
	}

	entry := m.cpus[m.focusedID]

	switch strings.ToLower(cmd.Args[1]) {
	case "add":
		if len(cmd.Args) < 3 || entry == nil {
			m.appendOutput("Usage: trace watch add <addr>", colorRed)
			return false
		}
		addr, ok := EvalAddress(cmd.Args[2], entry.CPU)
		if !ok {
			m.appendOutput(fmt.Sprintf("Invalid address: %s", cmd.Args[2]), colorRed)
			return false
		}
		m.traceWatches[addr] = true
		// Snapshot current value
		data := entry.CPU.ReadMemory(addr, 1)
		if len(data) > 0 {
			m.traceSnapshots[addr] = data[0]
		}
		m.appendOutput(fmt.Sprintf("Trace watch added at $%X", addr), colorCyan)

	case "del":
		if len(cmd.Args) < 3 || entry == nil {
			m.appendOutput("Usage: trace watch del <addr>", colorRed)
			return false
		}
		addr, ok := EvalAddress(cmd.Args[2], entry.CPU)
		if !ok {
			m.appendOutput(fmt.Sprintf("Invalid address: %s", cmd.Args[2]), colorRed)
			return false
		}
		delete(m.traceWatches, addr)
		delete(m.traceSnapshots, addr)
		m.appendOutput(fmt.Sprintf("Trace watch removed at $%X", addr), colorCyan)

	case "list":
		if len(m.traceWatches) == 0 {
			m.appendOutput("No trace watches", colorDim)
			return false
		}
		for addr := range m.traceWatches {
			m.appendOutput(fmt.Sprintf("  $%X", addr), colorCyan)
		}

	default:
		m.appendOutput("Usage: trace watch add/del/list [addr]", colorRed)
	}
	return false
}

func (m *MachineMonitor) cmdTraceHistory(cmd MonitorCommand) bool {
	if len(cmd.Args) < 2 {
		m.appendOutput("Usage: trace history show/clear <addr|*>", colorRed)
		return false
	}

	entry := m.cpus[m.focusedID]

	switch strings.ToLower(cmd.Args[1]) {
	case "show":
		if len(cmd.Args) < 3 || entry == nil {
			m.appendOutput("Usage: trace history show <addr>", colorRed)
			return false
		}
		addr, ok := EvalAddress(cmd.Args[2], entry.CPU)
		if !ok {
			m.appendOutput(fmt.Sprintf("Invalid address: %s", cmd.Args[2]), colorRed)
			return false
		}
		records := m.writeHistory[addr]
		if len(records) == 0 {
			m.appendOutput(fmt.Sprintf("$%X: no writes recorded", addr), colorDim)
			return false
		}
		m.appendOutput(fmt.Sprintf("$%X: %d writes recorded", addr, len(records)), colorCyan)
		for _, rec := range records {
			m.appendOutput(fmt.Sprintf("  Step #%-5d PC=$%06X  $%02X -> $%02X",
				rec.StepNum, rec.PC, rec.OldValue, rec.NewValue), colorWhite)
		}

	case "clear":
		if len(cmd.Args) < 3 {
			m.appendOutput("Usage: trace history clear <addr|*>", colorRed)
			return false
		}
		if cmd.Args[2] == "*" {
			m.writeHistory = make(map[uint64][]WriteRecord)
			m.appendOutput("All write history cleared", colorCyan)
		} else {
			if entry == nil {
				return false
			}
			addr, ok := EvalAddress(cmd.Args[2], entry.CPU)
			if !ok {
				m.appendOutput(fmt.Sprintf("Invalid address: %s", cmd.Args[2]), colorRed)
				return false
			}
			delete(m.writeHistory, addr)
			m.appendOutput(fmt.Sprintf("Write history cleared for $%X", addr), colorCyan)
		}

	default:
		m.appendOutput("Usage: trace history show/clear <addr|*>", colorRed)
	}
	return false
}

func (m *MachineMonitor) cmdTraceRun(cmd MonitorCommand) bool {
	entry := m.cpus[m.focusedID]
	if entry == nil {
		m.appendOutput("No CPU focused", colorRed)
		return false
	}

	count, ok := ParseAddress(cmd.Args[0])
	if !ok || count == 0 {
		m.appendOutput("Usage: trace <count>", colorRed)
		return false
	}

	n := int(count)
	addrWidth := entry.CPU.AddressWidth()
	addrFmt := "%06X"
	if addrWidth <= 16 {
		addrFmt = "%04X"
	}

	for i := range n {
		// Get pre-step state
		pc := entry.CPU.GetPC()
		lines := entry.CPU.Disassemble(pc, 1)
		mnemonic := ""
		if len(lines) > 0 {
			mnemonic = lines[0].Mnemonic
		}

		// Save pre-step registers
		preRegs := entry.CPU.GetRegisters()

		// Step
		entry.CPU.Step()

		// Build trace line showing changed registers
		postRegs := entry.CPU.GetRegisters()
		var changes []string
		for j, r := range postRegs {
			if j < len(preRegs) && preRegs[j].Value != r.Value {
				changes = append(changes, fmt.Sprintf("%s=$%X", r.Name, r.Value))
			}
		}

		traceLine := fmt.Sprintf(addrFmt+": %-30s %s", pc, mnemonic, strings.Join(changes, " "))

		if m.traceFile != nil {
			fmt.Fprintln(m.traceFile, traceLine)
		} else {
			m.appendOutput(traceLine, colorWhite)
		}

		// Check trace watches for writes
		for addr := range m.traceWatches {
			data := entry.CPU.ReadMemory(addr, 1)
			if len(data) > 0 {
				oldVal := m.traceSnapshots[addr]
				if data[0] != oldVal {
					rec := WriteRecord{
						PC:       pc,
						OldValue: oldVal,
						NewValue: data[0],
						StepNum:  i + 1,
					}
					m.writeHistory[addr] = append(m.writeHistory[addr], rec)
					if len(m.writeHistory[addr]) > 256 {
						m.writeHistory[addr] = m.writeHistory[addr][len(m.writeHistory[addr])-256:]
					}
					m.traceSnapshots[addr] = data[0]
				}
			}
		}

		// Check for breakpoint at new PC — only stop if condition is satisfied
		newPC := entry.CPU.GetPC()
		if bp := entry.CPU.GetConditionalBreakpoint(newPC); bp != nil {
			bp.HitCount++
			if evaluateConditionWithHitCount(bp.Condition, entry.CPU, bp.HitCount) {
				m.appendOutput(fmt.Sprintf("Trace stopped at breakpoint $%X", newPC), colorRed)
				break
			}
		}

		// Yield lock periodically so UI can render
		if m.traceFile != nil {
			if i%1000 == 999 {
				m.yieldLock()
			}
		} else {
			m.yieldLock()
		}
	}

	m.appendOutput(fmt.Sprintf("Trace complete: %d instructions", count), colorCyan)
	m.saveCurrentRegs()
	m.showDisassembly(0, 1)
	return false
}

// --- Feature 9: Backstep ---

func (m *MachineMonitor) cmdBackstep(_ MonitorCommand) bool {
	entry := m.cpus[m.focusedID]
	if entry == nil {
		m.appendOutput("No CPU focused", colorRed)
		return false
	}

	cpuID := m.focusedID
	hist := m.stepHistory[cpuID]
	if len(hist) == 0 {
		m.appendOutput("No step history available", colorRed)
		return false
	}

	snap := hist[len(hist)-1]
	m.stepHistory[cpuID] = hist[:len(hist)-1]

	RestoreSnapshot(entry.CPU, snap)

	m.appendOutput(fmt.Sprintf("Backstep: restored to PC=$%X (CPU+memory)", entry.CPU.GetPC()), colorCyan)

	// Show changed registers
	regs := entry.CPU.GetRegisters()
	for _, r := range regs {
		if prev, ok := m.prevRegs[r.Name]; ok && prev != r.Value {
			m.appendOutput(fmt.Sprintf("  %s: $%X -> $%X", r.Name, prev, r.Value), colorGreen)
		}
	}
	m.saveCurrentRegs()
	m.showDisassembly(0, 1)
	return false
}

// --- Feature 11: I/O Register Viewer ---

func (m *MachineMonitor) cmdIOView(cmd MonitorCommand) bool {
	entry := m.cpus[m.focusedID]
	if entry == nil {
		m.appendOutput("No CPU focused", colorRed)
		return false
	}

	if len(cmd.Args) == 0 {
		m.appendOutput("Available I/O devices:", colorCyan)
		for _, name := range listIODevices() {
			m.appendOutput(fmt.Sprintf("  %s", name), colorWhite)
		}
		return false
	}

	arg := strings.ToLower(cmd.Args[0])
	if arg == "all" {
		for _, name := range listIODevices() {
			lines := formatIOView(entry.CPU, name)
			for _, line := range lines {
				m.appendOutput(line, colorCyan)
			}
		}
		return false
	}

	lines := formatIOView(entry.CPU, arg)
	for _, line := range lines {
		m.appendOutput(line, colorCyan)
	}
	return false
}

// --- Feature 12: Hex Editor ---

func (m *MachineMonitor) cmdHexEdit(cmd MonitorCommand) bool {
	entry := m.cpus[m.focusedID]
	if entry == nil {
		m.appendOutput("No CPU focused", colorRed)
		return false
	}

	if len(cmd.Args) < 1 {
		m.appendOutput("Usage: e <addr>", colorRed)
		return false
	}

	addr, ok := EvalAddress(cmd.Args[0], entry.CPU)
	if !ok {
		m.appendOutput(fmt.Sprintf("Invalid address: %s", cmd.Args[0]), colorRed)
		return false
	}

	m.state = MonitorHexEdit
	m.hexEditAddr = addr
	m.hexEditCursor = 0
	m.hexEditNibble = 0
	m.hexEditDirty = make(map[uint64]byte)
	return false
}

// HexEditCommit writes all dirty bytes to memory and returns to command mode.
func (m *MachineMonitor) HexEditCommit() {
	entry := m.cpus[m.focusedID]
	if entry == nil {
		return
	}
	for addr, val := range m.hexEditDirty {
		entry.CPU.WriteMemory(addr, []byte{val})
	}
	count := len(m.hexEditDirty)
	m.hexEditDirty = make(map[uint64]byte)
	m.state = MonitorActive
	m.appendOutput(fmt.Sprintf("Committed %d byte(s)", count), colorGreen)
}

// HexEditDiscard returns to command mode without writing.
func (m *MachineMonitor) HexEditDiscard() {
	m.hexEditDirty = make(map[uint64]byte)
	m.state = MonitorActive
	m.appendOutput("Hex edit discarded", colorDim)
}

// HexEditSetNibble sets one nibble at the cursor position.
func (m *MachineMonitor) HexEditSetNibble(nibbleVal byte) {
	entry := m.cpus[m.focusedID]
	if entry == nil {
		return
	}

	addr := m.hexEditAddr + uint64(m.hexEditCursor)

	// Get current value (from dirty map or memory)
	var current byte
	if v, ok := m.hexEditDirty[addr]; ok {
		current = v
	} else {
		data := entry.CPU.ReadMemory(addr, 1)
		if len(data) > 0 {
			current = data[0]
		}
	}

	if m.hexEditNibble == 0 {
		current = (nibbleVal << 4) | (current & 0x0F)
	} else {
		current = (current & 0xF0) | (nibbleVal & 0x0F)
	}
	m.hexEditDirty[addr] = current

	// Advance
	m.hexEditNibble++
	if m.hexEditNibble > 1 {
		m.hexEditNibble = 0
		m.hexEditCursor++
	}
}

// --- Feature 13: Scripting / Command Batching ---

func (m *MachineMonitor) cmdScript(cmd MonitorCommand) bool {
	if len(cmd.Args) < 1 {
		m.appendOutput("Usage: script <filename>", colorRed)
		return false
	}

	data, err := os.ReadFile(cmd.Args[0])
	if err != nil {
		m.appendOutput(fmt.Sprintf("Error: %s", err), colorRed)
		return false
	}

	m.scriptDepth++
	if m.scriptDepth > 8 {
		m.scriptDepth--
		m.appendOutput("Script recursion limit reached", colorRed)
		return false
	}

	lines := strings.SplitSeq(string(data), "\n")
	for line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if m.ExecuteCommand(line) {
			m.scriptDepth--
			return true
		}
	}

	m.scriptDepth--
	return false
}

func (m *MachineMonitor) cmdMacro(cmd MonitorCommand) bool {
	if len(cmd.Args) < 2 {
		m.appendOutput("Usage: macro <name> <cmd1> ; <cmd2> ; ...", colorRed)
		return false
	}

	name := strings.ToLower(cmd.Args[0])
	body := strings.Join(cmd.Args[1:], " ")
	cmds := strings.Split(body, ";")
	var cleaned []string
	for _, c := range cmds {
		c = strings.TrimSpace(c)
		if c != "" {
			cleaned = append(cleaned, c)
		}
	}

	m.macros[name] = cleaned
	m.appendOutput(fmt.Sprintf("Macro '%s' defined (%d commands)", name, len(cleaned)), colorCyan)
	return false
}

func (m *MachineMonitor) executeMacro(cmds []string) bool {
	m.scriptDepth++
	if m.scriptDepth > 8 {
		m.scriptDepth--
		m.appendOutput("Macro recursion limit reached", colorRed)
		return false
	}

	if slices.ContainsFunc(cmds, m.ExecuteCommand) {
		m.scriptDepth--
		return true
	}

	m.scriptDepth--
	return false
}
