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
	"math"
	"os"
	"path/filepath"
	"slices"
	"sort"
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
	parts, err := splitCommandLine(input)
	if err != nil || len(parts) == 0 {
		return MonitorCommand{}
	}
	return MonitorCommand{
		Name: strings.ToLower(parts[0]),
		Args: parts[1:],
	}
}

func splitCommandLine(input string) ([]string, error) {
	var parts []string
	var cur strings.Builder
	inQuote := false
	escaped := false
	for _, r := range input {
		switch {
		case escaped:
			cur.WriteRune(r)
			escaped = false
		case r == '\\' && inQuote:
			escaped = true
		case r == '"':
			inQuote = !inQuote
		case (r == ' ' || r == '\t') && !inQuote:
			if cur.Len() > 0 {
				parts = append(parts, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteRune(r)
		}
	}
	if escaped || inQuote {
		return nil, fmt.Errorf("unterminated quote")
	}
	if cur.Len() > 0 {
		parts = append(parts, cur.String())
	}
	return parts, nil
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

func parseCount(s string) (uint64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty count")
	}
	base := 10
	if strings.HasPrefix(s, "$") {
		s = s[1:]
		base = 16
	} else if strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X") {
		s = s[2:]
		base = 16
	}
	v, err := strconv.ParseUint(s, base, 64)
	if err != nil {
		return 0, err
	}
	return v, nil
}

// EvalAddress evaluates a simple expression: <term> [+|- <term>]*
// Each term is either a register name or a numeric address.
func EvalAddress(expr string, cpu DebuggableCPU) (uint64, bool) {
	return EvalAddressWithSymbols(expr, cpu, nil, "")
}

func EvalAddressWithSymbols(expr string, cpu DebuggableCPU, symbols *SymbolTable, cpuName string) (uint64, bool) {
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

	var result int64
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
		if !ok && symbols != nil {
			val, ok = symbols.Lookup(cpuName, tok.text)
		}
		if !ok {
			return 0, false
		}

		switch tok.op {
		case 0, '+':
			if val > math.MaxInt64 || result > math.MaxInt64-int64(val) {
				return 0, false
			}
			result += int64(val)
		case '-':
			if val > math.MaxInt64 || result < int64(val) {
				return 0, false
			}
			result -= int64(val)
		}
	}

	if result < 0 {
		return 0, false
	}
	return uint64(result), true
}

func (m *MachineMonitor) evalAddress(expr string, entry *CPUEntry) (uint64, bool) {
	if entry == nil {
		return 0, false
	}
	return EvalAddressWithSymbols(expr, entry.CPU, m.symbols, entry.CPU.CPUName())
}

// ExecuteCommand dispatches a parsed command to the appropriate handler.
// Acquires m.mu for thread safety. Returns true if the monitor should exit.
func (m *MachineMonitor) ExecuteCommand(input string) bool {
	exit, _ := m.ExecuteCommandResult(input)
	return exit
}

// ExecuteCommandResult dispatches a command and returns the exit hint plus the
// monitor output lines appended by this command.
func (m *MachineMonitor) ExecuteCommandResult(input string) (bool, []OutputLine) {
	m.mu.Lock()
	before := len(m.outputLines)
	exit := m.executeCommand(input)
	if before < 0 || before > len(m.outputLines) {
		before = 0
	}
	out := append([]OutputLine(nil), m.outputLines[before:]...)
	m.mu.Unlock()
	return exit, out
}

// executeCommand is the lock-free implementation of ExecuteCommand.
// Caller must hold m.mu.
func (m *MachineMonitor) executeCommand(input string) bool {
	rawInput := strings.TrimSpace(input)
	cmd := ParseCommand(input)
	if cmd.Name == "" {
		if m.lastRepeat != "" {
			cmd = ParseCommand(m.lastRepeat)
			rawInput = m.lastRepeat
		}
	}
	if cmd.Name == "" {
		return false
	}
	if expanded, ok := m.aliases[cmd.Name]; ok {
		pieces := []string{expanded}
		pieces = append(pieces, cmd.Args...)
		rawInput = strings.Join(pieces, " ")
		cmd = ParseCommand(rawInput)
		if cmd.Name == "" {
			return false
		}
	}

	// Add to history
	if rawInput != "" && (len(m.history) == 0 || m.history[len(m.history)-1] != rawInput) {
		m.history = append(m.history, rawInput)
		m.savePersistentHistory()
	}
	m.historyIdx = len(m.history)
	m.rememberRepeatCommand(rawInput, cmd.Name)

	switch cmd.Name {
	case "r":
		return m.cmdRegisters(cmd)
	case "d":
		return m.cmdDisassemble(cmd)
	case "list":
		return m.cmdListSource(cmd)
	case "m":
		return m.cmdMemoryDump(cmd)
	case "s":
		return m.cmdStep(cmd)
	case "rg":
		return m.cmdReverseContinue(cmd)
	case "rt":
		return m.cmdReverseRunUntil(cmd)
	case "tl":
		return m.cmdTimeline(cmd)
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
	case "wr", "wrw":
		return m.cmdWatchpointSetMode(cmd, WatchReadWrite, 1)
	case "bpmbr", "bpmrb":
		return m.cmdWatchpointSetMode(cmd, WatchRead, 1)
	case "bpmbw", "bpmwb":
		return m.cmdWatchpointSetMode(cmd, WatchWrite, 1)
	case "bpmb", "bpmba", "bpmab":
		return m.cmdWatchpointSetMode(cmd, WatchReadWrite, 1)
	case "bpmwr", "bpmrw":
		return m.cmdWatchpointSetMode(cmd, WatchRead, 2)
	case "bpmww":
		return m.cmdWatchpointSetMode(cmd, WatchWrite, 2)
	case "bpmw", "bpmwa", "bpmaw":
		return m.cmdWatchpointSetMode(cmd, WatchReadWrite, 2)
	case "bpmdr", "bpmrd":
		return m.cmdWatchpointSetMode(cmd, WatchRead, 4)
	case "bpmdw", "bpmwd":
		return m.cmdWatchpointSetMode(cmd, WatchWrite, 4)
	case "bpmd", "bpmda", "bpmad":
		return m.cmdWatchpointSetMode(cmd, WatchReadWrite, 4)
	case "bpmqr", "bpmrq":
		return m.cmdWatchpointSetMode(cmd, WatchRead, 8)
	case "bpmqw", "bpmwq":
		return m.cmdWatchpointSetMode(cmd, WatchWrite, 8)
	case "bpmq", "bpmqa", "bpmaq":
		return m.cmdWatchpointSetMode(cmd, WatchReadWrite, 8)
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
	case "tracering":
		return m.cmdTraceRing(cmd)
	case "show":
		return m.cmdShowTraceRing(cmd)
	case "history":
		return m.cmdHistory(cmd)
	case "bs":
		return m.cmdBackstep(cmd)
	case "rs":
		return m.cmdBackstep(cmd)
	case "io":
		return m.cmdIOView(cmd)
	case "sym":
		return m.cmdSymbols(cmd)
	case "map":
		return m.cmdMap(cmd)
	case "addr":
		return m.cmdAddr(cmd)
	case "pg":
		return m.cmdPageGuard(cmd)
	case "accesslog":
		return m.cmdAccessLog(cmd)
	case "who":
		return m.cmdWhoAccess(cmd)
	case "bfirst":
		return m.cmdBreakFirst(cmd)
	case "fault":
		return m.cmdFault(cmd)
	case "e":
		return m.cmdHexEdit(cmd)
	case "script":
		return m.cmdScript(cmd)
	case "macro":
		return m.cmdMacro(cmd)
	case "alias":
		return m.cmdAlias(cmd)
	case "rc":
		return m.cmdRC(cmd)
	case "layout":
		return m.cmdLayout(cmd)
	case "bug":
		return m.cmdBugReport(cmd)
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

func (m *MachineMonitor) cmdSymbols(cmd MonitorCommand) bool {
	entry := m.cpus[m.focusedID]
	if entry == nil {
		m.appendOutput("No CPU focussed", colorRed)
		return false
	}
	cpuName := entry.CPU.CPUName()
	if len(cmd.Args) < 1 {
		m.appendOutput("Usage: sym add|lookup|resolve|list|loadlbl|loadelf ...", colorRed)
		return false
	}
	switch strings.ToLower(cmd.Args[0]) {
	case "add":
		if len(cmd.Args) < 3 {
			m.appendOutput("Usage: sym add <name> <addr> [func|object|label]", colorRed)
			return false
		}
		addr, ok := m.evalAddress(cmd.Args[2], entry)
		if !ok {
			m.appendOutput(fmt.Sprintf("Invalid address: %s", cmd.Args[2]), colorRed)
			return false
		}
		kind := SymbolLabel
		if len(cmd.Args) >= 4 {
			kind = SymbolKind(strings.ToLower(cmd.Args[3]))
		}
		m.symbols.Add(cpuName, addr, cmd.Args[1], 0, kind)
		m.appendOutput(fmt.Sprintf("Symbol %s = $%X", cmd.Args[1], addr), colorCyan)
	case "lookup":
		if len(cmd.Args) < 2 {
			m.appendOutput("Usage: sym lookup <name>", colorRed)
			return false
		}
		addr, ok := m.symbols.Lookup(cpuName, cmd.Args[1])
		if !ok {
			m.appendOutput(fmt.Sprintf("No symbol: %s", cmd.Args[1]), colorRed)
			return false
		}
		m.appendOutput(fmt.Sprintf("%s = $%X", cmd.Args[1], addr), colorCyan)
	case "resolve":
		if len(cmd.Args) < 2 {
			m.appendOutput("Usage: sym resolve <addr>", colorRed)
			return false
		}
		addr, ok := m.evalAddress(cmd.Args[1], entry)
		if !ok {
			m.appendOutput(fmt.Sprintf("Invalid address: %s", cmd.Args[1]), colorRed)
			return false
		}
		res, ok := m.symbols.Resolve(cpuName, addr)
		if !ok {
			m.appendOutput(fmt.Sprintf("$%X: no symbol", addr), colorDim)
			return false
		}
		m.appendOutput(fmt.Sprintf("$%X = %s", addr, formatSymbolResolution(res)), colorCyan)
	case "list":
		syms := m.symbols.List(cpuName)
		if len(syms) == 0 {
			m.appendOutput("No symbols", colorDim)
			return false
		}
		for _, sym := range syms {
			m.appendOutput(fmt.Sprintf("$%X %-8s %s", sym.Addr, sym.Kind, sym.Name), colorCyan)
		}
	case "loadlbl":
		if len(cmd.Args) < 2 {
			m.appendOutput("Usage: sym loadlbl <file> [base]", colorRed)
			return false
		}
		base := uint64(0)
		if len(cmd.Args) >= 3 {
			var ok bool
			base, ok = m.evalAddress(cmd.Args[2], entry)
			if !ok {
				m.appendOutput(fmt.Sprintf("Invalid base: %s", cmd.Args[2]), colorRed)
				return false
			}
		}
		f, err := os.Open(cmd.Args[1])
		if err != nil {
			m.appendOutput(fmt.Sprintf("Error: %s", err), colorRed)
			return false
		}
		err = m.symbols.LoadVICELabels(cpuName, f, base)
		closeErr := f.Close()
		if err == nil {
			err = closeErr
		}
		if err != nil {
			m.appendOutput(fmt.Sprintf("Error: %s", err), colorRed)
			return false
		}
		m.appendOutput(fmt.Sprintf("Loaded VICE labels from %s", cmd.Args[1]), colorCyan)
	case "loadelf":
		if len(cmd.Args) < 2 {
			m.appendOutput("Usage: sym loadelf <file>", colorRed)
			return false
		}
		if err := m.symbols.LoadELF(cpuName, cmd.Args[1]); err != nil {
			m.appendOutput(fmt.Sprintf("Error: %s", err), colorRed)
			return false
		}
		_ = m.sources.LoadELF(cpuName, cmd.Args[1])
		m.appendOutput(fmt.Sprintf("Loaded ELF symbols from %s", cmd.Args[1]), colorCyan)
	default:
		m.appendOutput("Usage: sym add|lookup|resolve|list|loadlbl|loadelf ...", colorRed)
	}
	return false
}

func (m *MachineMonitor) cmdMap(cmd MonitorCommand) bool {
	entry := m.cpus[m.focusedID]
	if entry == nil {
		m.appendOutput("No CPU focussed", colorRed)
		return false
	}
	regions := m.regions.List(entry.CPU.CPUName())
	if len(regions) == 0 {
		m.appendOutput("No regions", colorDim)
		return false
	}
	for _, region := range regions {
		m.appendOutput(fmt.Sprintf("$%X-$%X %-6s %s", region.Start, region.End, region.Kind, region.Name), colorCyan)
	}
	return false
}

func (m *MachineMonitor) cmdAddr(cmd MonitorCommand) bool {
	entry := m.cpus[m.focusedID]
	if entry == nil {
		m.appendOutput("No CPU focussed", colorRed)
		return false
	}
	if len(cmd.Args) < 1 {
		m.appendOutput("Usage: addr <addr>", colorRed)
		return false
	}
	addr, ok := m.evalAddress(cmd.Args[0], entry)
	if !ok {
		m.appendOutput(fmt.Sprintf("Invalid address: %s", cmd.Args[0]), colorRed)
		return false
	}
	region := m.regions.Lookup(entry.CPU.CPUName(), addr)
	if region == nil {
		m.appendOutput(fmt.Sprintf("$%X: unmapped", addr), colorDim)
		return false
	}
	m.appendOutput(fmt.Sprintf("$%X: %s %s ($%X-$%X)", addr, region.Kind, region.Name, region.Start, region.End), colorCyan)
	return false
}

func (m *MachineMonitor) cmdPageGuard(cmd MonitorCommand) bool {
	if m.access == nil {
		m.appendOutput("Debug access service unavailable", colorRed)
		return false
	}
	if len(cmd.Args) < 1 {
		m.appendOutput("Usage: pg add <start> <end> <rwx> [cpu=all|current] | pg list | pg clear", colorRed)
		return false
	}
	switch strings.ToLower(cmd.Args[0]) {
	case "add":
		if !m.access.Instrumented() {
			m.appendOutput("Page guards require CPU/bus access instrumentation; this build has not enabled it yet", colorRed)
			return false
		}
		entry := m.cpus[m.focusedID]
		if entry == nil || len(cmd.Args) < 4 {
			m.appendOutput("Usage: pg add <start> <end> <rwx> [cpu=all|current]", colorRed)
			return false
		}
		start, ok1 := m.evalAddress(cmd.Args[1], entry)
		end, ok2 := m.evalAddress(cmd.Args[2], entry)
		perm, ok3 := parseAccessPerm(cmd.Args[3])
		if !ok1 || !ok2 || !ok3 || end < start {
			m.appendOutput("Invalid page guard arguments", colorRed)
			return false
		}
		scope := GuardScope{AllCPUs: true}
		if len(cmd.Args) >= 5 && strings.EqualFold(cmd.Args[4], "cpu=current") {
			scope = GuardScope{CPUID: m.focusedID}
		}
		m.access.Guard(start, end, perm, scope)
		m.appendOutput(fmt.Sprintf("Guard added $%X-$%X %s", start, end, strings.ToLower(cmd.Args[3])), colorCyan)
	case "list":
		guards := m.access.ListGuards()
		if len(guards) == 0 {
			m.appendOutput("No page guards", colorDim)
			return false
		}
		for _, guard := range guards {
			scope := "all"
			if !guard.Scope.AllCPUs {
				scope = fmt.Sprintf("cpu=%d", guard.Scope.CPUID)
			}
			m.appendOutput(fmt.Sprintf("$%X-$%X %s %s", guard.Start, guard.End, formatAccessPerm(guard.Perm), scope), colorCyan)
		}
	case "clear":
		m.access.ClearGuards()
		m.appendOutput("Page guards cleared", colorCyan)
	default:
		m.appendOutput("Usage: pg add <start> <end> <rwx> [cpu=all|current] | pg list | pg clear", colorRed)
	}
	return false
}

func (m *MachineMonitor) cmdAccessLog(cmd MonitorCommand) bool {
	if m.access == nil {
		m.appendOutput("Debug access service unavailable", colorRed)
		return false
	}
	if len(cmd.Args) == 0 {
		m.appendOutput("Usage: accesslog on [size] | accesslog off | accesslog show [count]", colorRed)
		return false
	}
	switch strings.ToLower(cmd.Args[0]) {
	case "on":
		if !m.access.Instrumented() {
			m.appendOutput("Access log requires CPU/bus access instrumentation; this build has not enabled it yet", colorRed)
			return false
		}
		size := 256
		if len(cmd.Args) >= 2 {
			count, err := parseCount(cmd.Args[1])
			if err != nil || count == 0 {
				m.appendOutput("Usage: accesslog on [size]", colorRed)
				return false
			}
			size = int(count)
		}
		m.access.EnableHistory(size)
		m.appendOutput(fmt.Sprintf("Access log enabled (%d events)", size), colorCyan)
	case "off":
		m.access.DisableHistory()
		m.appendOutput("Access log disabled", colorCyan)
	case "show":
		count := 16
		if len(cmd.Args) >= 2 {
			parsed, err := parseCount(cmd.Args[1])
			if err != nil {
				m.appendOutput("Usage: accesslog show [count]", colorRed)
				return false
			}
			count = int(parsed)
		}
		events := m.access.HistoryTail(count)
		if len(events) == 0 {
			m.appendOutput("Access log is empty", colorCyan)
			return false
		}
		for _, ev := range events {
			m.appendOutput(formatAccessEvent(ev), colorWhite)
		}
	default:
		m.appendOutput("Usage: accesslog on [size] | accesslog off | accesslog show [count]", colorRed)
	}
	return false
}

func (m *MachineMonitor) cmdWhoAccess(cmd MonitorCommand) bool {
	if m.access == nil {
		m.appendOutput("Debug access service unavailable", colorRed)
		return false
	}
	if len(cmd.Args) < 2 {
		m.appendOutput("Usage: who read|wrote|fetched <addr>", colorRed)
		return false
	}
	kind, ok := parseWhoAccessKind(cmd.Args[0])
	if !ok {
		m.appendOutput("Usage: who read|wrote|fetched <addr>", colorRed)
		return false
	}
	entry := m.cpus[m.focusedID]
	addr, ok := m.evalAddress(cmd.Args[1], entry)
	if !ok {
		m.appendOutput(fmt.Sprintf("Invalid address: %s", cmd.Args[1]), colorRed)
		return false
	}
	ev, ok := m.access.LastAccess(kind, addr)
	if !ok {
		m.appendOutput(fmt.Sprintf("No %s recorded for $%X", accessKindString(kind), addr), colorCyan)
		return false
	}
	m.appendOutput(formatAccessEvent(ev), colorWhite)
	return false
}

func (m *MachineMonitor) cmdBreakFirst(cmd MonitorCommand) bool {
	if m.access == nil {
		m.appendOutput("Debug access service unavailable", colorRed)
		return false
	}
	entry := m.cpus[m.focusedID]
	if entry == nil {
		m.appendOutput("No CPU focussed", colorRed)
		return false
	}
	if len(cmd.Args) < 2 {
		m.appendOutput("Usage: bfirst read|write|fetch <region-name>", colorRed)
		return false
	}
	if !m.access.Instrumented() {
		m.appendOutput("bfirst requires CPU/bus access instrumentation; this build has not enabled it yet", colorRed)
		return false
	}
	kind, ok := parseWhoAccessKind(cmd.Args[0])
	if !ok {
		m.appendOutput("Usage: bfirst read|write|fetch <region-name>", colorRed)
		return false
	}
	region := m.regions.LookupName(entry.CPU.CPUName(), cmd.Args[1])
	if region == nil {
		m.appendOutput(fmt.Sprintf("Unknown region for %s: %s", entry.CPU.CPUName(), cmd.Args[1]), colorRed)
		return false
	}
	m.access.GuardOnce(region.Start, region.End, accessPermForKind(kind), GuardScope{AllCPUs: true}, region.Name)
	m.appendOutput(fmt.Sprintf("Break-on-first %s armed for %s ($%X-$%X)", accessKindString(kind), region.Name, region.Start, region.End), colorCyan)
	return false
}

func (m *MachineMonitor) cmdFault(cmd MonitorCommand) bool {
	if m.faults == nil {
		m.appendOutput("Fault interception unavailable", colorRed)
		return false
	}
	if len(cmd.Args) == 0 || strings.EqualFold(cmd.Args[0], "list") {
		all, kinds := m.faults.List()
		if all {
			m.appendOutput("Fault interception: all", colorCyan)
			return false
		}
		if len(kinds) == 0 {
			m.appendOutput("Fault interception: off", colorDim)
			return false
		}
		m.appendOutput("Fault break kinds:", colorCyan)
		for _, kind := range kinds {
			m.appendOutput("  "+kind, colorCyan)
		}
		return false
	}
	switch strings.ToLower(cmd.Args[0]) {
	case "on":
		m.faults.EnableAll()
		m.appendOutput("Fault interception enabled for all supported faults", colorCyan)
	case "off":
		m.faults.DisableAll()
		m.appendOutput("Fault interception disabled", colorCyan)
	case "break":
		if len(cmd.Args) < 2 || !m.faults.EnableKind(cmd.Args[1]) {
			m.appendOutput("Usage: fault break <kind>", colorRed)
			return false
		}
		m.appendOutput("Fault break enabled: "+normalizeFaultKind(cmd.Args[1]), colorCyan)
	case "clear":
		if len(cmd.Args) < 2 || !m.faults.DisableKind(cmd.Args[1]) {
			m.appendOutput("Usage: fault clear <kind>", colorRed)
			return false
		}
		m.appendOutput("Fault break cleared: "+normalizeFaultKind(cmd.Args[1]), colorCyan)
	default:
		m.appendOutput("Usage: fault on|off|list|break <kind>|clear <kind>", colorRed)
	}
	return false
}

func parseWhoAccessKind(s string) (AccessKind, bool) {
	switch strings.ToLower(s) {
	case "read", "r":
		return AccessRead, true
	case "wrote", "write", "written", "w":
		return AccessWrite, true
	case "fetched", "fetch", "execute", "exec", "x":
		return AccessExecute, true
	default:
		return AccessRead, false
	}
}

func accessPermForKind(kind AccessKind) AccessPerm {
	switch kind {
	case AccessRead:
		return PermRead
	case AccessWrite:
		return PermWrite
	case AccessExecute:
		return PermExecute
	default:
		return 0
	}
}

func formatAccessEvent(ev AccessEvent) string {
	width := ev.Width
	if width <= 0 {
		width = 1
	}
	line := fmt.Sprintf("#%d cpu=%d pc=$%X %s $%X", ev.Seq, ev.CPUID, ev.PC, accessKindString(ev.Kind), ev.Address)
	if width > 1 {
		line += fmt.Sprintf("..$%X", ev.Address+uint64(width-1))
	}
	if ev.Kind == AccessWrite {
		if ev.OldValueKnown {
			line += fmt.Sprintf(" old=$%X", ev.OldValue)
		} else {
			line += " old=?"
		}
		line += fmt.Sprintf(" new=$%X", ev.NewValue)
	}
	return line
}

func parseAccessPerm(text string) (AccessPerm, bool) {
	var perm AccessPerm
	for _, r := range strings.ToLower(text) {
		switch r {
		case 'r':
			perm |= PermRead
		case 'w':
			perm |= PermWrite
		case 'x':
			perm |= PermExecute
		default:
			return 0, false
		}
	}
	return perm, perm != 0
}

func formatAccessPerm(perm AccessPerm) string {
	var b strings.Builder
	if perm&PermRead != 0 {
		b.WriteByte('r')
	}
	if perm&PermWrite != 0 {
		b.WriteByte('w')
	}
	if perm&PermExecute != 0 {
		b.WriteByte('x')
	}
	return b.String()
}

func (m *MachineMonitor) cmdRegisters(cmd MonitorCommand) bool {
	entry := m.cpus[m.focusedID]
	if entry == nil {
		m.appendOutput("No CPU focussed", colorRed)
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

		m.appendOutput(formatMonitorRegisterLine(r, addrWidth), color)
	}
}

func (m *MachineMonitor) cmdDisassemble(cmd MonitorCommand) bool {
	entry := m.cpus[m.focusedID]
	if entry == nil {
		m.appendOutput("No CPU focussed", colorRed)
		return false
	}

	addr := entry.CPU.GetPC()
	count := 16
	sourceMode := false

	args := cmd.Args
	if len(args) > 0 && args[0] == "/s" {
		sourceMode = true
		args = args[1:]
	}
	if len(args) >= 1 {
		if v, ok := m.evalAddress(args[0], entry); ok {
			addr = v
		}
	}
	if len(args) >= 2 {
		if v, ok := ParseAddress(args[1]); ok {
			count = int(v)
		}
	}

	m.showDisassemblyAt(addr, count, sourceMode)
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
	m.showDisassemblyAt(addr, count, false)
}

func (m *MachineMonitor) showDisassemblyAt(addr uint64, count int, sourceMode ...bool) {
	entry := m.cpus[m.focusedID]
	if entry == nil {
		return
	}
	withSource := len(sourceMode) > 0 && sourceMode[0]

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

	lastSource := ""
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
			if res, ok := m.symbols.Resolve(entry.CPU.CPUName(), line.BranchTarget); ok {
				suffix += " ; " + formatSymbolResolution(res)
			}
		}

		if withSource {
			if src, ok := m.sources.Resolve(entry.CPU.CPUName(), line.Address); ok {
				srcText := fmt.Sprintf("%s:%d", src.File, src.Line)
				if srcText != lastSource {
					m.appendOutput("  "+srcText, colorDim)
					lastSource = srcText
				}
			} else if lastSource == "" {
				m.appendOutput(fmt.Sprintf("  no source info for %s", entry.CPU.CPUName()), colorDim)
				lastSource = "-"
			}
		}
		text := fmt.Sprintf("%s"+addrFmt+": %-24s %s%s", prefix, line.Address, line.HexBytes, line.Mnemonic, suffix)
		m.appendOutput(text, color)
	}
}

func (m *MachineMonitor) cmdListSource(cmd MonitorCommand) bool {
	entry := m.cpus[m.focusedID]
	if entry == nil {
		m.appendOutput("No CPU focussed", colorRed)
		return false
	}
	addr := entry.CPU.GetPC()
	if len(cmd.Args) >= 1 {
		if v, ok := m.evalAddress(cmd.Args[0], entry); ok {
			addr = v
		}
	}
	src, ok := m.sources.Resolve(entry.CPU.CPUName(), addr)
	if !ok {
		m.appendOutput(fmt.Sprintf("no source info for %s", entry.CPU.CPUName()), colorDim)
		return false
	}
	m.appendOutput(fmt.Sprintf("%s:%d", src.File, src.Line), colorCyan)
	if lines, ok := sourceContextLines(src.File, src.Line, 2); ok {
		for _, line := range lines {
			m.appendOutput(line, colorWhite)
		}
	}
	return false
}

func sourceContextLines(file string, line, radius int) ([]string, bool) {
	path, ok := resolveSourcePath(file)
	if !ok {
		return nil, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	srcLines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	if line <= 0 || line > len(srcLines) {
		return nil, false
	}
	start := line - radius
	if start < 1 {
		start = 1
	}
	end := line + radius
	if end > len(srcLines) {
		end = len(srcLines)
	}
	out := make([]string, 0, end-start+1)
	width := len(strconv.Itoa(end))
	for n := start; n <= end; n++ {
		marker := " "
		if n == line {
			marker = ">"
		}
		out = append(out, fmt.Sprintf("%s %*d  %s", marker, width, n, srcLines[n-1]))
	}
	return out, true
}

func resolveSourcePath(file string) (string, bool) {
	if file == "" {
		return "", false
	}
	if filepath.IsAbs(file) {
		if _, err := os.Stat(file); err == nil {
			return file, true
		}
		return "", false
	}
	candidates := []string{file}
	if paths := os.Getenv("IEMON_SRC_PATH"); paths != "" {
		for _, root := range filepath.SplitList(paths) {
			if root != "" {
				candidates = append(candidates, filepath.Join(root, file))
			}
		}
	}
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate, true
		}
	}
	return "", false
}

func (m *MachineMonitor) cmdMemoryDump(cmd MonitorCommand) bool {
	entry := m.cpus[m.focusedID]
	if entry == nil {
		m.appendOutput("No CPU focussed", colorRed)
		return false
	}

	addr := entry.CPU.GetPC()
	lines := 8

	if len(cmd.Args) >= 1 {
		if v, ok := m.evalAddress(cmd.Args[0], entry); ok {
			addr = v
		}
	}
	if len(cmd.Args) >= 2 {
		if v, err := parseCount(cmd.Args[1]); err == nil {
			lines = int(v)
		}
	}
	if err := entry.CPU.ValidateAddress(addr); err != nil {
		m.appendOutput(err.Error(), colorRed)
		return false
	}
	addrDigits := (entry.CPU.AddressWidth() + 3) / 4

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
		text := fmt.Sprintf("%0*X: %s  %s", addrDigits, addr, hexStr, string(asciiParts))
		m.appendOutput(text, colorWhite)
		addr += 16
	}
	return false
}

func (m *MachineMonitor) cmdStep(cmd MonitorCommand) bool {
	entry := m.cpus[m.focusedID]
	if entry == nil {
		m.appendOutput("No CPU focussed", colorRed)
		return false
	}

	count := 1
	if len(cmd.Args) >= 1 {
		if v, err := parseCount(cmd.Args[0]); err == nil {
			count = int(v)
		}
	}

	cpuID := m.focusedID

	totalCycles := 0
	for i := 0; i < count; i++ {
		m.recordWholeMachineHistory()
		snap := TakeSnapshot(entry.CPU)
		m.stepHistory[cpuID] = append(m.stepHistory[cpuID], snap)
		if len(m.stepHistory[cpuID]) > m.maxBackstep {
			m.stepHistory[cpuID] = m.stepHistory[cpuID][len(m.stepHistory[cpuID])-m.maxBackstep:]
		}
		m.recordTraceRing(cpuID, entry, entry.CPU.GetPC())
		cycles, err := safeDebugStep(entry.CPU)
		if err != nil {
			m.appendOutput(fmt.Sprintf("Step failed: %s", err), colorRed)
			break
		}
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

func (m *MachineMonitor) recordWholeMachineHistory() uint64 {
	snap, err := m.takeWholeMachineSnapshotLocked()
	if err != nil {
		m.appendOutput(fmt.Sprintf("Whole-machine snapshot skipped: %s", err), colorRed)
		return 0
	}
	if len(m.wholeHistory) > 0 {
		if prev, err := m.materializeWholeMachineSnapshotLocked(m.wholeHistory[len(m.wholeHistory)-1]); err == nil && wholeSnapshotsEquivalent(prev, snap) {
			return prev.ID
		}
	}
	m.nextWholeID++
	snap.ID = m.nextWholeID
	snap.Full = true
	snap.DeltaBytes = snapshotDeltaBytes(snap)
	if len(m.wholeHistory) > 0 {
		prev, err := m.materializeWholeMachineSnapshotLocked(m.wholeHistory[len(m.wholeHistory)-1])
		if err == nil {
			interval := m.wholeCheckpointInterval
			if interval <= 0 {
				interval = 32
			}
			bytesLimit := m.wholeCheckpointBytes
			if bytesLimit == 0 {
				bytesLimit = 64 << 20
			}
			if m.wholeDeltaCount < interval && m.wholeDeltaBytes < bytesLimit {
				snap = makeWholeMachineDelta(snap, prev)
				m.wholeDeltaCount++
				m.wholeDeltaBytes += snap.DeltaBytes
			} else {
				m.wholeDeltaCount = 0
				m.wholeDeltaBytes = 0
			}
		}
	}
	m.wholeHistory = append(m.wholeHistory, snap)
	m.pruneWholeHistoryLocked()
	return snap.ID
}

func (m *MachineMonitor) pruneWholeHistoryLocked() {
	limit := m.maxWholeHistory
	if limit <= 0 {
		limit = 32
	}
	if len(m.wholeHistory) > limit {
		m.wholeHistory = m.wholeHistory[len(m.wholeHistory)-limit:]
	}
	checkpointLimit := m.maxWholeCheckpoints
	if checkpointLimit <= 0 {
		checkpointLimit = 8
	}
	checkpoints := 0
	keepFrom := 0
	for i := len(m.wholeHistory) - 1; i >= 0; i-- {
		if m.wholeHistory[i] != nil && m.wholeHistory[i].Full {
			checkpoints++
			if checkpoints == checkpointLimit {
				keepFrom = i
				break
			}
		}
	}
	if checkpoints >= checkpointLimit && keepFrom > 0 {
		m.wholeHistory = m.wholeHistory[keepFrom:]
	}
	for len(m.wholeHistory) > 0 && m.wholeHistory[0] != nil && !m.wholeHistory[0].Full {
		m.wholeHistory = m.wholeHistory[1:]
	}
}

func safeDebugStep(cpu DebuggableCPU) (cycles int, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%v", r)
		}
	}()
	return cpu.Step(), nil
}

func (m *MachineMonitor) recordTraceRing(cpuID int, entry *CPUEntry, pc uint64) {
	ring := m.traceRings[cpuID]
	if ring == nil || !ring.Enabled() || entry == nil {
		return
	}
	lines := entry.CPU.Disassemble(pc, 1)
	tr := TraceRingEntry{CPUName: entry.CPU.CPUName(), PC: pc}
	if len(lines) > 0 {
		tr.HexBytes = lines[0].HexBytes
		tr.Mnemonic = lines[0].Mnemonic
	}
	ring.Add(tr)
	m.recordTimelineEventLocked("instr", cpuID, pc, strings.TrimSpace(tr.Mnemonic))
}

func (m *MachineMonitor) recordTimelineEventLocked(kind string, cpuID int, pc uint64, detail string) {
	m.recordTimelineEventWithSnapshotLocked(kind, cpuID, pc, detail, 0)
}

func (m *MachineMonitor) recordTimelineEventWithSnapshotLocked(kind string, cpuID int, pc uint64, detail string, snapshotID uint64) {
	if m.maxTimeline <= 0 {
		m.maxTimeline = 4096
	}
	ev := TimelineEvent{Seq: m.nextTimelineSeq(), Kind: kind, CPUID: cpuID, PC: pc, Detail: detail, SnapshotID: snapshotID}
	m.timelineEvents = append(m.timelineEvents, ev)
	if len(m.timelineEvents) > m.maxTimeline {
		m.timelineEvents = m.timelineEvents[len(m.timelineEvents)-m.maxTimeline:]
	}
}

func (m *MachineMonitor) cmdGo(cmd MonitorCommand) bool {
	if len(cmd.Args) >= 1 {
		entry := m.cpus[m.focusedID]
		if entry != nil {
			if v, ok := m.evalAddress(cmd.Args[0], entry); ok {
				if err := entry.CPU.ValidateAddress(v); err != nil {
					m.appendOutput(err.Error(), colorRed)
					return false
				}
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
		m.appendOutput("No CPU focussed", colorRed)
		return false
	}

	if len(cmd.Args) < 1 {
		m.appendOutput("Usage: b <addr> [condition]", colorRed)
		return false
	}

	addr, ok := m.evalAddress(cmd.Args[0], entry)
	if !ok {
		m.appendOutput(fmt.Sprintf("Invalid address: %s", cmd.Args[0]), colorRed)
		return false
	}
	if err := entry.CPU.ValidateAddress(addr); err != nil {
		m.appendOutput(err.Error(), colorRed)
		return false
	}
	if err := entry.CPU.ValidateAddress(addr); err != nil {
		m.appendOutput(err.Error(), colorRed)
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
		m.appendOutput("No CPU focussed", colorRed)
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
		m.appendOutput("No CPU focussed", colorRed)
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
		m.appendOutput("No CPU focussed", colorRed)
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
	if end < start {
		m.appendOutput("Invalid range: end below start", colorRed)
		return false
	}
	if len(pattern) == 0 || end-start+1 < uint64(len(pattern)) {
		m.appendOutput("Not found", colorDim)
		return false
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
		m.appendOutput("No CPU focussed", colorRed)
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
		m.appendOutput("No CPU focussed", colorRed)
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
		m.appendOutput("No CPU focussed", colorRed)
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
		entries := make([]*CPUEntry, 0, len(m.cpus))
		for _, entry := range m.cpus {
			entries = append(entries, entry)
		}
		sort.Slice(entries, func(i, j int) bool { return entries[i].ID < entries[j].ID })
		for _, entry := range entries {
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
		if m.coprocMgr != nil {
			registered := make(map[uint32]bool)
			for _, entry := range entries {
				if cpuType, ok := coprocCPUTypeFromLabel(entry.Label); ok {
					registered[cpuType] = true
				}
			}
			m.mu.Unlock()
			slots := m.coprocMgr.WorkerInventory()
			m.mu.Lock()
			for _, slot := range slots {
				if slot.Online || registered[slot.CPUType] {
					continue
				}
				m.appendOutput(fmt.Sprintf(" id:-   %-12s [OFFLINE]  PC=-", slot.Label), colorDim)
			}
		}
		return false
	}

	switch strings.ToLower(cmd.Args[0]) {
	case "online":
		return m.cmdCPUOnline(cmd)
	case "offline":
		return m.cmdCPUOffline(cmd)
	}

	// Switch focus by ID or label
	target := cmd.Args[0]

	// Try numeric ID first
	if id, err := strconv.Atoi(target); err == nil {
		if _, ok := m.cpus[id]; ok {
			m.focusedID = id
			m.saveCurrentRegs()
			m.appendOutput(fmt.Sprintf("Focussed on id:%d %s", id, m.cpus[id].Label), colorCyan)
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
		m.appendOutput(fmt.Sprintf("Focussed on id:%d %s", matches[0].ID, matches[0].Label), colorCyan)
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

func (m *MachineMonitor) cmdCPUOnline(cmd MonitorCommand) bool {
	if m.coprocMgr == nil {
		m.appendOutput("No coprocessor manager attached", colorRed)
		return false
	}
	args := append([]string(nil), cmd.Args[1:]...)
	replace := false
	filtered := args[:0]
	for _, arg := range args {
		if arg == "--replace" {
			replace = true
			continue
		}
		filtered = append(filtered, arg)
	}
	args = filtered
	if len(args) < 1 || len(args) > 2 {
		m.appendOutput("Usage: cpu online [--replace] <type|path.ie*> [path.ie*]", colorRed)
		return false
	}

	var cpuType uint32
	var imagePath string
	if len(args) == 1 {
		if inferred := inferCoprocCPUTypeFromImagePath(args[0]); inferred != EXEC_TYPE_NONE {
			imagePath = args[0]
		} else {
			cpuType = coprocCPUTypeFromString(args[0])
			if cpuType == EXEC_TYPE_NONE {
				m.appendOutput(fmt.Sprintf("Unrecognised coprocessor CPU type or image: %s", args[0]), colorRed)
				return false
			}
		}
	} else {
		cpuType = coprocCPUTypeFromString(args[0])
		if cpuType == EXEC_TYPE_NONE {
			m.appendOutput(fmt.Sprintf("Unrecognised coprocessor CPU type: %s", args[0]), colorRed)
			return false
		}
		imagePath = args[1]
	}

	var err error
	if imagePath == "" {
		m.mu.Unlock()
		imagePath = m.coprocMgr.StagedServicePath()
		if imagePath == "" {
			err = fmt.Errorf("no staged coprocessor service image for %s", coprocCPUTypeToString(cpuType))
		} else {
			_, err = m.coprocMgr.StartWorkerFromImage(cpuType, imagePath, replace)
		}
		m.mu.Lock()
	} else {
		m.mu.Unlock()
		cpuType, err = m.coprocMgr.StartWorkerFromImage(cpuType, imagePath, replace)
		m.mu.Lock()
	}
	if err != nil {
		m.appendOutput(fmt.Sprintf("CPU online failed: %v", err), colorRed)
		return false
	}
	m.appendOutput(fmt.Sprintf("Online %s as %s", coprocCPUTypeToString(cpuType), coprocLabel(cpuType)), colorCyan)
	return false
}

func (m *MachineMonitor) cmdCPUOffline(cmd MonitorCommand) bool {
	if m.coprocMgr == nil {
		m.appendOutput("No coprocessor manager attached", colorRed)
		return false
	}
	if len(cmd.Args) != 2 {
		m.appendOutput("Usage: cpu offline <id|label|type>", colorRed)
		return false
	}
	cpuType, ok := m.coprocCPUTypeFromTarget(cmd.Args[1])
	if !ok {
		return false
	}
	m.mu.Unlock()
	err := m.coprocMgr.StopWorker(cpuType)
	m.mu.Lock()
	if err != nil {
		m.appendOutput(fmt.Sprintf("CPU offline failed: %v", err), colorRed)
		return false
	}
	m.appendOutput(fmt.Sprintf("Offline %s", coprocLabel(cpuType)), colorCyan)
	return false
}

func (m *MachineMonitor) coprocCPUTypeFromTarget(target string) (uint32, bool) {
	if cpuType := coprocCPUTypeFromString(target); cpuType != EXEC_TYPE_NONE {
		return cpuType, true
	}
	if cpuType, ok := coprocCPUTypeFromLabel(target); ok {
		return cpuType, true
	}
	if id, err := strconv.Atoi(target); err == nil {
		entry := m.cpus[id]
		if entry == nil {
			m.appendOutput(fmt.Sprintf("No CPU with id:%d", id), colorRed)
			return 0, false
		}
		cpuType, ok := coprocCPUTypeFromLabel(entry.Label)
		if !ok {
			m.appendOutput(fmt.Sprintf("id:%d %s is not a coprocessor worker", id, entry.Label), colorRed)
			return 0, false
		}
		return cpuType, true
	}
	var matches []uint32
	for _, entry := range m.cpus {
		if !strings.EqualFold(entry.Label, target) {
			continue
		}
		if cpuType, ok := coprocCPUTypeFromLabel(entry.Label); ok {
			matches = append(matches, cpuType)
		}
	}
	if len(matches) == 1 {
		return matches[0], true
	}
	if len(matches) > 1 {
		m.appendOutput("Ambiguous label, use ID:", colorRed)
		for _, entry := range m.cpus {
			if strings.EqualFold(entry.Label, target) {
				m.appendOutput(fmt.Sprintf("  id:%d %s", entry.ID, entry.Label), colorWhite)
			}
		}
		return 0, false
	}
	m.appendOutput(fmt.Sprintf("No coprocessor CPU matching '%s'", target), colorRed)
	return 0, false
}

func coprocCPUTypeFromLabel(label string) (uint32, bool) {
	if !strings.HasPrefix(strings.ToLower(label), "coproc:") {
		return EXEC_TYPE_NONE, false
	}
	cpuType := coprocCPUTypeFromString(label[len("coproc:"):])
	return cpuType, cpuType != EXEC_TYPE_NONE
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

type monitorHelpEntry struct {
	Name     string
	Summary  string
	Syntax   []string
	Examples []string
}

func monitorHelpRegistry() []monitorHelpEntry {
	return []monitorHelpEntry{
		{Name: "r", Summary: "Show or change registers", Syntax: []string{"r", "r <name> <value>"}, Examples: []string{"r", "r pc $1000", "r d0 #42"}},
		{Name: "d", Summary: "Disassemble memory; /s shows source lines when available", Syntax: []string{"d [/s] [addr] [count]"}, Examples: []string{"d", "d main #12", "d /s pc #8"}},
		{Name: "list", Summary: "Show source location for an address", Syntax: []string{"list [addr]"}, Examples: []string{"list", "list main", "list pc"}},
		{Name: "m", Summary: "Dump memory as hex and ASCII", Syntax: []string{"m [addr] [lines]"}, Examples: []string{"m pc", "m $4000 8", "m main+4 2"}},
		{Name: "s", Summary: "Single-step the focussed CPU", Syntax: []string{"s [count]"}, Examples: []string{"s", "s 10", "s 16"}},
		{Name: "bs", Summary: "Step the focussed CPU backwards using CPU-local history", Syntax: []string{"bs", "rs"}, Examples: []string{"bs", "rs", "s; bs"}},
		{Name: "rg", Summary: "Replay or restore to the previous whole-machine reverse boundary", Syntax: []string{"rg"}, Examples: []string{"s; rg", "history horizon", "rg"}},
		{Name: "rt", Summary: "Replay or restore to the latest reverse boundary matching an expression", Syntax: []string{"rt <expr>"}, Examples: []string{"rt pc==$1000", "rt A==1", "rt b($4000)==$ff"}},
		{Name: "tl", Summary: "Show the merged sequence-ordered access and monitor timeline, or scrub backwards", Syntax: []string{"tl [count]", "tl back"}, Examples: []string{"tl", "tl 32", "tl back"}},
		{Name: "g", Summary: "Continue execution, optionally from a new PC", Syntax: []string{"g [addr]"}, Examples: []string{"g", "g main", "g $2000"}},
		{Name: "u", Summary: "Run until an address is reached", Syntax: []string{"u <addr>"}, Examples: []string{"u main", "u pc+20", "u $1000"}},
		{Name: "x", Summary: "Close the monitor and resume CPUs that were running", Syntax: []string{"x"}, Examples: []string{"x", "g", "thaw *; x"}},
		{Name: "b", Summary: "Set a breakpoint with an optional condition", Syntax: []string{"b <addr> [if <expr>]", "b <addr> <legacy-condition>"}, Examples: []string{"b main", "b $1000 if R1==5", "b loop hitcount>3"}},
		{Name: "bc", Summary: "Clear one breakpoint or all breakpoints", Syntax: []string{"bc <addr|*>"}, Examples: []string{"bc main", "bc $1000", "bc *"}},
		{Name: "bl", Summary: "List breakpoints", Syntax: []string{"bl"}, Examples: []string{"bl", "b main; bl", "cpu 1; bl"}},
		{Name: "ww", Summary: "Set a legacy one-byte write watchpoint", Syntax: []string{"ww <addr>"}, Examples: []string{"ww $5000", "ww sprite_x", "ww pc+1"}},
		{Name: "bpm", Summary: "Set read/write watchpoints by mode and width", Syntax: []string{"bpmbr|bpmbw|bpmb <addr>", "bpmwr|bpmww|bpmw <addr>", "bpmdr|bpmdw|bpmd <addr>", "bpmqr|bpmqw|bpmq <addr>"}, Examples: []string{"bpmbr $5000", "bpmdw pixel", "bpmq buffer"}},
		{Name: "wc", Summary: "Clear one watchpoint or all watchpoints", Syntax: []string{"wc <addr|*>"}, Examples: []string{"wc $5000", "wc sprite_x", "wc *"}},
		{Name: "wl", Summary: "List watchpoints", Syntax: []string{"wl"}, Examples: []string{"wl", "ww $5000; wl", "bpmw $8000; wl"}},
		{Name: "bt", Summary: "Show a symbol-aware stack backtrace", Syntax: []string{"bt [depth]"}, Examples: []string{"bt", "bt 8", "sym loadelf kernel.elf; bt"}},
		{Name: "sym", Summary: "Manage symbols for the focussed CPU", Syntax: []string{"sym add <name> <addr> [func|object|label]", "sym loadlbl <file> [base]", "sym loadelf <file>", "sym lookup|resolve|list ..."}, Examples: []string{"sym add main $1000 func", "sym lookup main", "sym loadelf demo.elf"}},
		{Name: "map", Summary: "List the memory map for the focussed CPU", Syntax: []string{"map"}, Examples: []string{"map", "cpu 1; map", "addr $F0000"}},
		{Name: "addr", Summary: "Describe the memory region containing an address", Syntax: []string{"addr <addr>"}, Examples: []string{"addr pc", "addr $F0000", "addr sprite_buffer"}},
		{Name: "pg", Summary: "Add, list, or clear page-access guards", Syntax: []string{"pg add <start> <end> <rwx> [cpu=all|current]", "pg list", "pg clear"}, Examples: []string{"pg add $4000 $4FFF rw cpu=current", "pg add code code+255 x", "pg list"}},
		{Name: "accesslog", Summary: "Record read/write/fetch access events", Syntax: []string{"accesslog on [size]", "accesslog off", "accesslog show [count]"}, Examples: []string{"accesslog on 4096", "accesslog show 20", "accesslog off"}},
		{Name: "who", Summary: "Find the last reader, writer, or fetcher of an address", Syntax: []string{"who read|wrote|fetched <addr>"}, Examples: []string{"who wrote $D020", "who read buffer", "who fetched main"}},
		{Name: "bfirst", Summary: "Break once on the first access to a named region", Syntax: []string{"bfirst read|write|fetch <region-name>"}, Examples: []string{"bfirst write mmio", "bfirst fetch rom", "bfirst read ram"}},
		{Name: "trace", Summary: "Trace instructions, files, write history, or MMIO access events", Syntax: []string{"trace <count>", "trace file <path|off>", "trace watch add|del|list <addr>", "trace history show|clear <addr|*>", "trace mmio <region> [count]"}, Examples: []string{"trace 20", "trace file trace.txt", "trace mmio mmio 32"}},
		{Name: "history", Summary: "Show or tune reverse-debugging snapshot history", Syntax: []string{"history horizon", "history config [delta-interval] [delta-miB] [checkpoints] [snapshots]"}, Examples: []string{"history horizon", "history config", "history config 32 64 8 256"}},
		{Name: "tracering", Summary: "Enable or disable the per-CPU instruction trace ring", Syntax: []string{"tracering on|off [size]"}, Examples: []string{"tracering on", "tracering on 8192", "tracering off"}},
		{Name: "show", Summary: "Show the tail of the instruction trace ring", Syntax: []string{"show [count]"}, Examples: []string{"show", "show 32", "tracering on; s 4; show"}},
		{Name: "fault", Summary: "Break before selected guest fault handlers run", Syntax: []string{"fault on|off|list", "fault break <kind>", "fault clear <kind>"}, Examples: []string{"fault list", "fault break m68k.illegal", "fault off"}},
		{Name: "cpu", Summary: "List CPUs, change focus, or manage coprocessor worker slots", Syntax: []string{"cpu", "cpu <id|label>", "cpu online [--replace] <type|path.ie*> [path.ie*]", "cpu offline <id|label|type>"}, Examples: []string{"cpu", "cpu 1", "cpu online z80", "cpu online z80 svc.ie80", "cpu online --replace svc.ie80", "cpu offline z80"}},
		{Name: "freeze", Summary: "Freeze a CPU or all CPUs", Syntax: []string{"freeze <id|label|*>"}, Examples: []string{"freeze *", "freeze 0", "freeze M68K"}},
		{Name: "thaw", Summary: "Resume a frozen CPU or all CPUs", Syntax: []string{"thaw <id|label|*>"}, Examples: []string{"thaw *", "thaw 0", "thaw M68K"}},
		{Name: "layout", Summary: "Render a named monitor view preset", Syntax: []string{"layout cpu|trace|debug", "layout list", "layout save <name>"}, Examples: []string{"layout cpu", "layout trace", "layout save bringup"}},
		{Name: "alias", Summary: "Create or list command aliases", Syntax: []string{"alias", "alias <name> <command...>"}, Examples: []string{"alias ni s", "alias regs r", "alias"}},
		{Name: "rc", Summary: "List, trust, or load project-local IEMon rc files", Syntax: []string{"rc list", "rc trust [file]", "rc load [file]"}, Examples: []string{"rc list", "rc trust .iemonrc", "rc load .iemonrc"}},
		{Name: "bug", Summary: "Print a copyable debugger report bundle", Syntax: []string{"bug [trace-count]"}, Examples: []string{"bug", "bug 64", "accesslog on; bug"}},
		{Name: "io", Summary: "Show I/O registers", Syntax: []string{"io [device|all]"}, Examples: []string{"io", "io video", "io all"}},
		{Name: "e", Summary: "Enter hex editor mode", Syntax: []string{"e <addr>"}, Examples: []string{"e $4000", "e pc", "e sprite_buffer"}},
		{Name: "f", Summary: "Fill memory", Syntax: []string{"f <start> <end> <byte>"}, Examples: []string{"f $4000 $40FF 00", "f buffer buffer+255 FF", "f #0 #15 #32"}},
		{Name: "w", Summary: "Write bytes to memory", Syntax: []string{"w <addr> <bytes..>"}, Examples: []string{"w $4000 01 02 03", "w pc EA", "w buffer 00 FF"}},
		{Name: "h", Summary: "Hunt for a byte pattern", Syntax: []string{"h <start> <end> <bytes..>"}, Examples: []string{"h $1000 $2000 DE AD", "h #0 $FFFF 4C 00", "h code code+1024 EA"}},
		{Name: "c", Summary: "Compare two memory ranges", Syntax: []string{"c <start> <end> <dest>"}, Examples: []string{"c $1000 $10FF $2000", "c buffer buffer+31 copy", "c #0 #15 $100"}},
		{Name: "t", Summary: "Transfer memory", Syntax: []string{"t <start> <end> <dest>"}, Examples: []string{"t $1000 $10FF $2000", "t buffer buffer+255 scratch", "t #0 #15 $8000"}},
		{Name: "save", Summary: "Save memory to a host file", Syntax: []string{"save <start> <end> <file>"}, Examples: []string{"save $1000 $1FFF dump.bin", "save main main+255 code.bin", "save #0 #255 page0.bin"}},
		{Name: "load", Summary: "Load a host file into memory", Syntax: []string{"load <file> <addr>"}, Examples: []string{"load demo.bin $1000", "load patch.bin pc", "load font.bin charset"}},
		{Name: "ss", Summary: "Save a CPU-local state snapshot", Syntax: []string{"ss [file]"}, Examples: []string{"ss", "ss before.iem", "s; ss step.iem"}},
		{Name: "sl", Summary: "Load a CPU-local state snapshot", Syntax: []string{"sl [file]"}, Examples: []string{"sl", "sl before.iem", "sl step.iem"}},
		{Name: "fa", Summary: "Freeze audio output", Syntax: []string{"fa"}, Examples: []string{"fa", "fa; s 10", "fa; ta"}},
		{Name: "ta", Summary: "Thaw audio output", Syntax: []string{"ta"}, Examples: []string{"ta", "fa; ta", "ta; g"}},
		{Name: "script", Summary: "Run a monitor command script", Syntax: []string{"script <file>"}, Examples: []string{"script bringup.imon", "script tests/boot.imon", "script repro.imon"}},
		{Name: "macro", Summary: "Define a semicolon-separated command macro", Syntax: []string{"macro <name> <cmds..>"}, Examples: []string{"macro regs r;d", "macro boot b main;g", "macro mm map;pg list"}},
	}
}

func monitorHelpByName(name string) (monitorHelpEntry, bool) {
	name = strings.ToLower(name)
	for _, entry := range monitorHelpRegistry() {
		if entry.Name == name {
			return entry, true
		}
	}
	if strings.HasPrefix(name, "bpm") {
		return monitorHelpByName("bpm")
	}
	return monitorHelpEntry{}, false
}

func (m *MachineMonitor) cmdHelp(cmd MonitorCommand) bool {
	if len(cmd.Args) > 0 {
		name := strings.ToLower(cmd.Args[0])
		entry, ok := monitorHelpByName(name)
		if !ok {
			m.appendOutput(fmt.Sprintf("No help for %s", name), colorRed)
			return false
		}
		m.appendOutput(entry.Name+" - "+entry.Summary, colorCyan)
		m.appendOutput("Syntax:", colorCyan)
		for _, line := range entry.Syntax {
			m.appendOutput("  "+line, colorWhite)
		}
		m.appendOutput("Examples:", colorCyan)
		for _, line := range entry.Examples {
			m.appendOutput("  "+line, colorWhite)
		}
		return false
	}

	m.appendOutput("Machine Monitor Commands:", colorCyan)
	for _, entry := range monitorHelpRegistry() {
		m.appendOutput(fmt.Sprintf("  %-10s %s", entry.Name, entry.Summary), colorCyan)
	}
	m.appendOutput("", colorCyan)
	m.appendOutput("Use help <command> for syntax and worked examples.", colorCyan)
	m.appendOutput("Addresses: $hex, 0xhex, bare hex, #decimal, symbols, and expr+expr.", colorCyan)
	m.appendOutput("Conditions: reg==val, [$addr]==val, hitcount>val, or if <expr>.", colorCyan)
	return false
}

func (m *MachineMonitor) rememberRepeatCommand(input, name string) {
	switch strings.ToLower(name) {
	case "s", "d", "m":
		m.lastRepeat = input
	}
}

func (m *MachineMonitor) reverseHistorySearch() bool {
	query := string(m.inputLine)
	continuing := m.historySearchIdx >= 0 && m.historySearchIdx < len(m.history) && string(m.inputLine) == m.history[m.historySearchIdx]
	if continuing {
		query = m.historySearchQuery
	}
	if !continuing && (query != m.historySearchQuery || m.historySearchIdx < 0 || m.historySearchIdx > len(m.history)) {
		m.historySearchQuery = query
		m.historySearchIdx = len(m.history)
	}
	for i := m.historySearchIdx - 1; i >= 0; i-- {
		if query == "" || strings.Contains(m.history[i], query) {
			m.historySearchIdx = i
			m.historyIdx = i
			m.inputLine = []byte(m.history[i])
			m.cursorPos = len(m.inputLine)
			return true
		}
	}
	return false
}

func iemonHomeDir() string {
	if home := os.Getenv("IEMON_HOME"); home != "" {
		return home
	}
	if len(os.Args) > 0 && strings.HasSuffix(os.Args[0], ".test") {
		return ""
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return filepath.Join(home, ".iemon")
	}
	return ""
}

func iemonHistoryPath() string {
	home := iemonHomeDir()
	if home == "" {
		return ""
	}
	return filepath.Join(home, "history")
}

func (m *MachineMonitor) loadPersistentHistory() {
	path := iemonHistoryPath()
	if path == "" {
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	lines := strings.Split(string(data), "\n")
	seen := make(map[string]bool)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || seen[line] {
			continue
		}
		seen[line] = true
		m.history = append(m.history, line)
	}
	if len(m.history) > 1000 {
		m.history = m.history[len(m.history)-1000:]
	}
	m.historyIdx = len(m.history)
}

func (m *MachineMonitor) savePersistentHistory() {
	path := iemonHistoryPath()
	if path == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return
	}
	start := 0
	if len(m.history) > 1000 {
		start = len(m.history) - 1000
	}
	var b strings.Builder
	seen := make(map[string]bool)
	for _, line := range m.history[start:] {
		line = strings.TrimSpace(line)
		if line == "" || seen[line] {
			continue
		}
		seen[line] = true
		b.WriteString(line)
		b.WriteByte('\n')
	}
	_ = os.WriteFile(path, []byte(b.String()), 0600)
}

func (m *MachineMonitor) cmdAlias(cmd MonitorCommand) bool {
	if len(cmd.Args) == 0 {
		if len(m.aliases) == 0 {
			m.appendOutput("No aliases", colorDim)
			return false
		}
		names := make([]string, 0, len(m.aliases))
		for name := range m.aliases {
			names = append(names, name)
		}
		slices.Sort(names)
		for _, name := range names {
			m.appendOutput(fmt.Sprintf("%s = %s", name, m.aliases[name]), colorCyan)
		}
		return false
	}
	if len(cmd.Args) < 2 {
		m.appendOutput("Usage: alias <name> <command...>", colorRed)
		return false
	}
	name := strings.ToLower(cmd.Args[0])
	if _, ok := monitorHelpByName(name); ok {
		m.appendOutput("Alias cannot replace a built-in command", colorRed)
		return false
	}
	m.aliases[name] = strings.Join(cmd.Args[1:], " ")
	m.appendOutput(fmt.Sprintf("Alias %s = %s", name, m.aliases[name]), colorCyan)
	return false
}

func (m *MachineMonitor) cmdLayout(cmd MonitorCommand) bool {
	if len(cmd.Args) == 0 || strings.EqualFold(cmd.Args[0], "list") {
		m.appendOutput("Layouts: cpu, trace, debug", colorCyan)
		return false
	}
	switch strings.ToLower(cmd.Args[0]) {
	case "cpu":
		m.showRegisters()
		m.showDisassembly(0, 8)
	case "trace":
		_ = m.cmdShowTraceRing(MonitorCommand{Name: "show", Args: []string{"16"}})
		_ = m.cmdAccessLog(MonitorCommand{Name: "accesslog", Args: []string{"show", "16"}})
	case "debug":
		m.showRegisters()
		m.showDisassembly(0, 6)
		_ = m.cmdBacktrace(MonitorCommand{Name: "bt", Args: []string{"8"}})
		_ = m.cmdPageGuard(MonitorCommand{Name: "pg", Args: []string{"list"}})
	case "save":
		if len(cmd.Args) < 2 {
			m.appendOutput("Usage: layout save <name>", colorRed)
			return false
		}
		m.aliases["layout-"+strings.ToLower(cmd.Args[1])] = "layout debug"
		m.appendOutput(fmt.Sprintf("Layout %s saved", cmd.Args[1]), colorCyan)
	default:
		m.appendOutput("Usage: layout cpu|trace|debug|list|save <name>", colorRed)
	}
	return false
}

func (m *MachineMonitor) cmdBugReport(cmd MonitorCommand) bool {
	traceCount := 16
	if len(cmd.Args) > 0 {
		if parsed, err := parseCount(cmd.Args[0]); err == nil && parsed > 0 {
			traceCount = int(parsed)
		}
	}
	m.appendOutput("IEMon bug report", colorCyan)
	m.appendOutput("== CPU ==", colorCyan)
	if entry := m.cpus[m.focusedID]; entry != nil {
		m.appendOutput(fmt.Sprintf("focussed=%d label=%s cpu=%s pc=$%X running=%v", entry.ID, entry.Label, entry.CPU.CPUName(), entry.CPU.GetPC(), entry.CPU.IsRunning()), colorWhite)
	}
	m.appendOutput("== Registers ==", colorCyan)
	m.showRegisters()
	m.appendOutput("== Disassembly ==", colorCyan)
	m.showDisassembly(0, 6)
	m.appendOutput("== Stack ==", colorCyan)
	_ = m.cmdBacktrace(MonitorCommand{Name: "bt", Args: []string{"8"}})
	m.appendOutput("== Regions ==", colorCyan)
	_ = m.cmdMap(MonitorCommand{Name: "map"})
	m.appendOutput("== Guards ==", colorCyan)
	_ = m.cmdPageGuard(MonitorCommand{Name: "pg", Args: []string{"list"}})
	m.appendOutput("== Access events ==", colorCyan)
	_ = m.cmdAccessLog(MonitorCommand{Name: "accesslog", Args: []string{"show", strconv.Itoa(traceCount)}})
	m.appendOutput("== Trace ring ==", colorCyan)
	_ = m.cmdShowTraceRing(MonitorCommand{Name: "show", Args: []string{strconv.Itoa(traceCount)}})
	m.appendOutput("== Symbols ==", colorCyan)
	_ = m.cmdSymbols(MonitorCommand{Name: "sym", Args: []string{"list"}})
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
		m.appendOutput("No CPU focussed", colorRed)
		return false
	}

	if len(cmd.Args) < 3 {
		m.appendOutput("Usage: save <start> <end> <filename>", colorRed)
		return false
	}

	start, ok1 := m.evalAddress(cmd.Args[0], entry)
	end, ok2 := m.evalAddress(cmd.Args[1], entry)
	if !ok1 || !ok2 {
		m.appendOutput("Invalid address", colorRed)
		return false
	}
	if end < start {
		m.appendOutput("End must be >= start", colorRed)
		return false
	}

	maxSize := uint64(32 * 1024 * 1024)
	if m.bus != nil && m.bus.TotalGuestRAM() > 0 {
		maxSize = m.bus.TotalGuestRAM()
	}
	size64 := end - start + 1
	maxInt := uint64(^uint(0) >> 1)
	if size64 > maxSize || size64 > maxInt {
		m.appendOutput(fmt.Sprintf("Range too large (max %d bytes)", maxSize), colorRed)
		return false
	}
	size := int(size64)

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
		m.appendOutput("No CPU focussed", colorRed)
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

	addr, ok := m.evalAddress(cmd.Args[1], entry)
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
		m.appendOutput("No CPU focussed", colorRed)
		return false
	}

	if len(cmd.Args) < 1 {
		m.appendOutput("Usage: u <addr>", colorRed)
		return false
	}

	addr, ok := m.evalAddress(cmd.Args[0], entry)
	if !ok {
		m.appendOutput(fmt.Sprintf("Invalid address: %s", cmd.Args[0]), colorRed)
		return false
	}
	if err := entry.CPU.ValidateAddress(addr); err != nil {
		m.appendOutput(err.Error(), colorRed)
		return false
	}

	existingBP, hasBP := entry.CPU.SnapshotBreakpoint(addr)
	if !hasBP {
		// No breakpoint exists - create a temp unconditional one
		entry.CPU.SetBreakpoint(addr)
		if m.tempBreakpoints[m.focusedID] == nil {
			m.tempBreakpoints[m.focusedID] = make(map[uint64]bool)
		}
		m.tempBreakpoints[m.focusedID][addr] = true
	} else if existingBP.Condition != nil {
		// A conditional breakpoint exists - temporarily make it unconditional
		// so run-until always stops. Save the original condition for restore.
		if m.savedConditions[m.focusedID] == nil {
			m.savedConditions[m.focusedID] = make(map[uint64]*BreakpointCondition)
		}
		m.savedConditions[m.focusedID][addr] = cloneCondition(existingBP.Condition)
		entry.CPU.SetBreakpointCondition(addr, nil)
		if m.runUntilHooks[m.focusedID] == nil {
			m.runUntilHooks[m.focusedID] = make(map[uint64]int)
		}
		cpuID := m.focusedID
		cpu := entry.CPU
		saved := cloneCondition(existingBP.Condition)
		hookID := m.nextStopHookID
		m.nextStopHookID++
		m.stopHooks[hookID] = func(stopCPU int, _ StopReason, _ uint64) {
			if stopCPU != -1 && stopCPU != cpuID {
				return
			}
			cpu.SetBreakpointCondition(addr, saved)
			m.UnregisterStopHook(hookID)
			m.mu.Lock()
			if byAddr := m.savedConditions[cpuID]; byAddr != nil {
				delete(byAddr, addr)
				if len(byAddr) == 0 {
					delete(m.savedConditions, cpuID)
				}
			}
			if byAddr := m.runUntilHooks[cpuID]; byAddr != nil {
				delete(byAddr, addr)
				if len(byAddr) == 0 {
					delete(m.runUntilHooks, cpuID)
				}
			}
			m.mu.Unlock()
		}
		m.runUntilHooks[cpuID][addr] = hookID
	}
	// else: unconditional breakpoint already exists - it will fire on its own

	m.appendOutput(fmt.Sprintf("Run until $%X", addr), colorCyan)
	return true // exit monitor to resume execution
}

// --- Feature 5: Watchpoints ---

func (m *MachineMonitor) cmdWatchpointSet(cmd MonitorCommand) bool {
	return m.cmdWatchpointSetMode(cmd, WatchWrite, 1)
}

func (m *MachineMonitor) cmdWatchpointSetMode(cmd MonitorCommand, typ WatchpointType, width uint8) bool {
	entry := m.cpus[m.focusedID]
	if entry == nil {
		m.appendOutput("No CPU focussed", colorRed)
		return false
	}

	if len(cmd.Args) < 1 {
		m.appendOutput("Usage: ww|bpm* <addr>", colorRed)
		return false
	}
	addr, ok := m.evalAddress(cmd.Args[0], entry)
	if !ok {
		m.appendOutput(fmt.Sprintf("Invalid address: %s", cmd.Args[0]), colorRed)
		return false
	}
	if err := entry.CPU.ValidateAddress(addr); err != nil {
		m.appendOutput(err.Error(), colorRed)
		return false
	}

	if setter, ok := entry.CPU.(extendedWatchpointSetter); ok {
		setter.SetWatchpointEx(addr, typ, width)
	} else {
		entry.CPU.SetWatchpoint(addr)
	}
	if m.access != nil && (typ != WatchWrite || isBPMWatchpointCommand(cmd.Name)) {
		m.access.Watch(entry.ID, addr, int(normalizeWatchWidth(width)), typ)
	}
	m.appendOutput(fmt.Sprintf("%s%d watchpoint set at $%X", watchpointTypeString(typ), normalizeWatchWidth(width), addr), colorCyan)
	return false
}

func isBPMWatchpointCommand(name string) bool {
	return strings.HasPrefix(strings.ToLower(name), "bpm")
}

func (m *MachineMonitor) cmdWatchpointClear(cmd MonitorCommand) bool {
	entry := m.cpus[m.focusedID]
	if entry == nil {
		m.appendOutput("No CPU focussed", colorRed)
		return false
	}

	if len(cmd.Args) < 1 {
		m.appendOutput("Usage: wc <addr|*>", colorRed)
		return false
	}

	if cmd.Args[0] == "*" {
		entry.CPU.ClearAllWatchpoints()
		if m.access != nil {
			m.access.ClearWatchesForCPU(entry.ID)
		}
		m.appendOutput("All watchpoints cleared", colorCyan)
		return false
	}

	addr, ok := m.evalAddress(cmd.Args[0], entry)
	if !ok {
		m.appendOutput(fmt.Sprintf("Invalid address: %s", cmd.Args[0]), colorRed)
		return false
	}

	if entry.CPU.ClearWatchpoint(addr) {
		if m.access != nil {
			m.access.ClearWatch(entry.ID, addr)
		}
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
			if wp, ok := entry.CPU.SnapshotWatchpoint(addr); ok {
				m.appendOutput(fmt.Sprintf("%s%d $%X (id:%d %s)", watchpointTypeString(wp.Type), normalizeWatchWidth(wp.Width), addr, entry.ID, entry.Label), colorCyan)
			} else {
				m.appendOutput(fmt.Sprintf("W1 $%X (id:%d %s)", addr, entry.ID, entry.Label), colorCyan)
			}
		}
	}
	return false
}

// --- Feature 6: Stack Trace / Backtrace ---

func (m *MachineMonitor) cmdBacktrace(cmd MonitorCommand) bool {
	entry := m.cpus[m.focusedID]
	if entry == nil {
		m.appendOutput("No CPU focussed", colorRed)
		return false
	}

	depth := 16
	if len(cmd.Args) >= 1 {
		if v, err := parseCount(cmd.Args[0]); err == nil {
			depth = int(v)
		}
	}

	frames := symbolAwareBacktrace(entry.CPU, depth, m.symbols, m.regions)
	if len(frames) == 0 {
		m.appendOutput("No stack frames found", colorDim)
		return false
	}

	for i, frame := range frames {
		line := fmt.Sprintf("#%-3d $%0*X", i, (entry.CPU.AddressWidth()+3)/4, frame.Address)
		if frame.HasSymbol {
			line += " " + formatSymbolResolution(frame.Symbol)
		}
		if frame.LowConfidence {
			line += " (low confidence)"
		}
		m.appendOutput(line, colorCyan)
	}
	return false
}

// --- Feature 7: Save/Load State ---

func (m *MachineMonitor) cmdSaveState(cmd MonitorCommand) bool {
	entry := m.cpus[m.focusedID]
	if entry == nil {
		m.appendOutput("No CPU focussed", colorRed)
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
		m.appendOutput("No CPU focussed", colorRed)
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

	if err := RestoreSnapshot(entry.CPU, snap); err != nil {
		m.appendOutput(fmt.Sprintf("Error: %s", err), colorRed)
		return false
	}
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
	case "mmio":
		return m.cmdTraceMMIO(cmd)
	default:
		return m.cmdTraceRun(cmd)
	}
}

func (m *MachineMonitor) cmdTraceRing(cmd MonitorCommand) bool {
	entry := m.cpus[m.focusedID]
	if entry == nil {
		m.appendOutput("No CPU focussed", colorRed)
		return false
	}
	if len(cmd.Args) < 1 {
		m.appendOutput("Usage: tracering on|off [size]", colorRed)
		return false
	}
	ring := m.traceRings[m.focusedID]
	if ring == nil {
		ring = NewDebugTraceRing(4096)
		m.traceRings[m.focusedID] = ring
	}
	switch strings.ToLower(cmd.Args[0]) {
	case "on":
		if len(cmd.Args) >= 2 {
			size, err := parseCount(cmd.Args[1])
			if err != nil || size == 0 {
				m.appendOutput("Invalid trace ring size", colorRed)
				return false
			}
			ring.Resize(int(size))
		}
		ring.SetEnabled(true)
		m.appendOutput("Trace ring enabled", colorCyan)
	case "off":
		ring.SetEnabled(false)
		m.appendOutput("Trace ring disabled", colorCyan)
	default:
		m.appendOutput("Usage: tracering on|off [size]", colorRed)
	}
	return false
}

func (m *MachineMonitor) cmdShowTraceRing(cmd MonitorCommand) bool {
	entry := m.cpus[m.focusedID]
	if entry == nil {
		m.appendOutput("No CPU focussed", colorRed)
		return false
	}
	count := 16
	if len(cmd.Args) >= 1 {
		if v, err := parseCount(cmd.Args[0]); err == nil {
			count = int(v)
		}
	}
	ring := m.traceRings[m.focusedID]
	if ring == nil {
		m.appendOutput("Trace ring empty", colorDim)
		return false
	}
	entries := ring.Tail(count)
	if len(entries) == 0 {
		m.appendOutput("Trace ring empty", colorDim)
		return false
	}
	addrWidth := (entry.CPU.AddressWidth() + 3) / 4
	for _, tr := range entries {
		m.appendOutput(fmt.Sprintf("%0*X: %-24s %s", addrWidth, tr.PC, tr.HexBytes, tr.Mnemonic), colorWhite)
	}
	return false
}

func (m *MachineMonitor) cmdTraceFile(cmd MonitorCommand) bool {
	if len(cmd.Args) < 2 {
		m.appendOutput("Usage: trace file <path|off>", colorRed)
		return false
	}

	if strings.ToLower(cmd.Args[1]) == "off" {
		if m.traceFile != nil {
			m.traceFile.Sync()
			m.traceFile.Close()
			m.traceFile = nil
		}
		m.appendOutput("Trace file output stopped", colorCyan)
		return false
	}

	if m.traceFile != nil {
		m.traceFile.Sync()
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
		addr, ok := m.evalAddress(cmd.Args[2], entry)
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
		addr, ok := m.evalAddress(cmd.Args[2], entry)
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
		addr, ok := m.evalAddress(cmd.Args[2], entry)
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
			addr, ok := m.evalAddress(cmd.Args[2], entry)
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

func (m *MachineMonitor) cmdTraceMMIO(cmd MonitorCommand) bool {
	if m.access == nil {
		m.appendOutput("Debug access service unavailable", colorRed)
		return false
	}
	entry := m.cpus[m.focusedID]
	if entry == nil {
		m.appendOutput("No CPU focussed", colorRed)
		return false
	}
	if len(cmd.Args) < 2 {
		m.appendOutput("Usage: trace mmio <region-name> [count]", colorRed)
		return false
	}
	region := m.regions.LookupName(entry.CPU.CPUName(), cmd.Args[1])
	if region == nil {
		m.appendOutput(fmt.Sprintf("Unknown region for %s: %s", entry.CPU.CPUName(), cmd.Args[1]), colorRed)
		return false
	}
	count := 16
	if len(cmd.Args) >= 3 {
		parsed, err := parseCount(cmd.Args[2])
		if err != nil {
			m.appendOutput("Usage: trace mmio <region-name> [count]", colorRed)
			return false
		}
		count = int(parsed)
	}
	events := m.access.HistoryTail(0)
	var matches []AccessEvent
	for _, ev := range events {
		if accessEventOverlapsRegion(ev, *region) {
			matches = append(matches, ev)
		}
	}
	if len(matches) == 0 {
		m.appendOutput(fmt.Sprintf("No access events for %s", region.Name), colorDim)
		return false
	}
	if count > 0 && len(matches) > count {
		matches = matches[len(matches)-count:]
	}
	for _, ev := range matches {
		m.appendOutput(formatAccessEvent(ev), colorWhite)
	}
	return false
}

func accessEventOverlapsRegion(ev AccessEvent, region MemoryRegion) bool {
	width := ev.Width
	if width <= 0 {
		width = 1
	}
	end := ev.Address + uint64(width-1)
	return end >= region.Start && ev.Address <= region.End
}

func (m *MachineMonitor) cmdTraceRun(cmd MonitorCommand) bool {
	entry := m.cpus[m.focusedID]
	if entry == nil {
		m.appendOutput("No CPU focussed", colorRed)
		return false
	}

	count, err := parseCount(cmd.Args[0])
	if err != nil || count == 0 {
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
		m.recordTraceRing(m.focusedID, entry, pc)
		lines := entry.CPU.Disassemble(pc, 1)
		mnemonic := ""
		if len(lines) > 0 {
			mnemonic = lines[0].Mnemonic
		}

		// Save pre-step registers
		preRegs := entry.CPU.GetRegisters()
		preByName := make(map[string]uint64, len(preRegs))
		for _, r := range preRegs {
			preByName[r.Name] = r.Value
		}

		// Step
		entry.CPU.Step()

		// Build trace line showing changed registers
		postRegs := entry.CPU.GetRegisters()
		var changes []string
		for _, r := range postRegs {
			if prev, ok := preByName[r.Name]; ok && prev != r.Value {
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

		// Check for breakpoint at new PC - only stop if condition is satisfied
		newPC := entry.CPU.GetPC()
		if bp := entry.CPU.GetConditionalBreakpoint(newPC); bp != nil {
			hitCount, _ := entry.CPU.IncrementBreakpointHit(newPC)
			if evaluateConditionWithHitCount(bp.Condition, entry.CPU, hitCount) {
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
		m.appendOutput("No CPU focussed", colorRed)
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

	if err := RestoreSnapshot(entry.CPU, snap); err != nil {
		m.appendOutput(fmt.Sprintf("Error: %s", err), colorRed)
		return false
	}

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

func (m *MachineMonitor) popWholeSnapshot() (*WholeMachineSnapshot, bool) {
	if len(m.wholeHistory) == 0 {
		return nil, false
	}
	snap := m.wholeHistory[len(m.wholeHistory)-1]
	m.wholeHistory = m.wholeHistory[:len(m.wholeHistory)-1]
	return snap, true
}

func (m *MachineMonitor) cmdReverseContinue(_ MonitorCommand) bool {
	snap, ok := m.popWholeSnapshot()
	if !ok {
		m.appendOutput("No whole-machine reverse history available", colorRed)
		return false
	}
	material, replayed, steps, err := m.restoreWholeMachineSnapshotWithReplayLocked(snap)
	if err != nil {
		m.appendOutput(fmt.Sprintf("Error: %s", err), colorRed)
		return false
	}
	if replayed {
		m.appendOutput(fmt.Sprintf("Reverse continue: replayed %d instruction(s) to boundary; restored %d CPU(s), shared bus state, and %d device snapshot(s)", steps, len(material.CPUs), len(material.Devices)), colorCyan)
	} else {
		m.appendOutput(fmt.Sprintf("Reverse continue: restored %d CPU(s), shared bus state, and %d device snapshot(s)", len(material.CPUs), len(material.Devices)), colorCyan)
	}
	m.saveCurrentRegs()
	m.showDisassembly(0, 1)
	return false
}

func (m *MachineMonitor) restoreWholeMachineSnapshotWithReplayLocked(target *WholeMachineSnapshot) (*WholeMachineSnapshot, bool, int, error) {
	material, err := m.materializeWholeMachineSnapshotLocked(target)
	if err != nil {
		return nil, false, 0, err
	}
	if len(m.wholeHistory) == 0 {
		if err := m.restoreWholeMachineSnapshotLocked(material); err != nil {
			return nil, false, 0, err
		}
		return material, false, 0, nil
	}
	start := m.wholeHistory[len(m.wholeHistory)-1]
	startMaterial, err := m.materializeWholeMachineSnapshotLocked(start)
	if err != nil {
		return nil, false, 0, err
	}
	if err := m.restoreWholeMachineSnapshotLocked(startMaterial); err != nil {
		return nil, false, 0, err
	}
	cpuID := m.replayCPUForTargetLocked(startMaterial, material)
	entry := m.cpus[cpuID]
	if entry == nil || entry.CPU == nil {
		if err := m.restoreWholeMachineSnapshotLocked(material); err != nil {
			return nil, false, 0, err
		}
		return material, false, 0, nil
	}
	const maxReverseReplaySteps = 100000
	for steps := 0; steps <= maxReverseReplaySteps; steps++ {
		current, err := m.takeWholeMachineSnapshotLocked()
		if err != nil {
			return nil, false, steps, err
		}
		if wholeSnapshotsEquivalent(current, material) {
			return material, true, steps, nil
		}
		if steps == maxReverseReplaySteps {
			break
		}
		if _, err := safeDebugStep(entry.CPU); err != nil {
			return nil, false, steps, err
		}
	}
	return nil, false, maxReverseReplaySteps, fmt.Errorf("reverse replay did not reach snapshot boundary within %d instruction(s)", maxReverseReplaySteps)
}

func (m *MachineMonitor) replayCPUForTargetLocked(start, target *WholeMachineSnapshot) int {
	targetByID := make(map[int]WholeMachineCPUState, len(target.CPUs))
	for _, cpu := range target.CPUs {
		targetByID[cpu.ID] = cpu
	}
	for _, startCPU := range start.CPUs {
		targetCPU, ok := targetByID[startCPU.ID]
		if !ok {
			continue
		}
		if !registerInfosEqual(startCPU.Registers, targetCPU.Registers) || !snapshotPagesEqual(startCPU.Pages, targetCPU.Pages) {
			return startCPU.ID
		}
	}
	return m.focusedID
}

func (m *MachineMonitor) cmdReverseRunUntil(cmd MonitorCommand) bool {
	if len(cmd.Args) == 0 {
		m.appendOutput("Usage: rt <expr>", colorRed)
		return false
	}
	entry := m.cpus[m.focusedID]
	if entry == nil {
		m.appendOutput("No CPU focussed", colorRed)
		return false
	}
	expr, err := ParseBreakpointExpr(strings.Join(cmd.Args, " "))
	if err != nil {
		m.appendOutput(fmt.Sprintf("Invalid expression: %s", err), colorRed)
		return false
	}
	current, err := m.takeWholeMachineSnapshotLocked()
	if err != nil {
		m.appendOutput(fmt.Sprintf("Error: %s", err), colorRed)
		return false
	}
	originalHistory := append([]*WholeMachineSnapshot(nil), m.wholeHistory...)
	for len(m.wholeHistory) > 0 {
		snap, _ := m.popWholeSnapshot()
		material, err := m.materializeWholeMachineSnapshotLocked(snap)
		if err != nil {
			m.wholeHistory = originalHistory
			_ = m.restoreWholeMachineSnapshotLocked(current)
			m.appendOutput(fmt.Sprintf("Error: %s", err), colorRed)
			return false
		}
		if err := m.restoreWholeMachineSnapshotLocked(material); err != nil {
			m.wholeHistory = originalHistory
			_ = m.restoreWholeMachineSnapshotLocked(current)
			m.appendOutput(fmt.Sprintf("Error: %s", err), colorRed)
			return false
		}
		entry = m.cpus[m.focusedID]
		if entry != nil && evalBreakpointExpr(expr, entry.CPU, 0) {
			material, replayed, steps, err := m.restoreWholeMachineSnapshotWithReplayLocked(snap)
			if err != nil {
				m.wholeHistory = originalHistory
				_ = m.restoreWholeMachineSnapshotLocked(current)
				m.appendOutput(fmt.Sprintf("Error: %s", err), colorRed)
				return false
			}
			entry = m.cpus[m.focusedID]
			pc := uint64(0)
			if entry != nil {
				pc = entry.CPU.GetPC()
			} else if len(material.CPUs) > 0 {
				for _, r := range material.CPUs[0].Registers {
					if strings.EqualFold(r.Name, "PC") {
						pc = r.Value
						break
					}
				}
			}
			if replayed {
				m.appendOutput(fmt.Sprintf("Reverse run-until: replayed %d instruction(s) to matching snapshot at PC=$%X", steps, pc), colorCyan)
			} else {
				m.appendOutput(fmt.Sprintf("Reverse run-until: restored matching snapshot at PC=$%X", pc), colorCyan)
			}
			m.saveCurrentRegs()
			m.showDisassembly(0, 1)
			return false
		}
	}
	m.wholeHistory = originalHistory
	_ = m.restoreWholeMachineSnapshotLocked(current)
	m.appendOutput("No earlier whole-machine snapshot matched", colorRed)
	return false
}

func (m *MachineMonitor) cmdTimeline(cmd MonitorCommand) bool {
	if len(cmd.Args) >= 1 {
		switch strings.ToLower(cmd.Args[0]) {
		case "back", "prev", "reverse":
			return m.cmdReverseContinue(MonitorCommand{Name: "rg"})
		}
	}
	count := 16
	if len(cmd.Args) >= 1 {
		if v, err := parseCount(cmd.Args[0]); err == nil && v > 0 {
			count = int(v)
		}
	}
	type line struct {
		seq  uint64
		text string
	}
	var lines []line
	for _, ev := range m.access.HistoryTail(count) {
		lines = append(lines, line{seq: ev.Seq, text: "access " + formatAccessEvent(ev)})
	}
	start := len(m.timelineEvents) - count
	if start < 0 {
		start = 0
	}
	for _, ev := range m.timelineEvents[start:] {
		lines = append(lines, line{seq: ev.Seq, text: formatTimelineEvent(ev)})
	}
	if len(lines) == 0 {
		m.appendOutput("Timeline is empty; enable accesslog to collect access events", colorDim)
		return false
	}
	sort.SliceStable(lines, func(i, j int) bool { return lines[i].seq < lines[j].seq })
	if len(lines) > count {
		lines = lines[len(lines)-count:]
	}
	for _, line := range lines {
		m.appendOutput(line.text, colorWhite)
	}
	return false
}

func formatTimelineEvent(ev TimelineEvent) string {
	line := fmt.Sprintf("#%d %s cpu=%d pc=$%X", ev.Seq, ev.Kind, ev.CPUID, ev.PC)
	if ev.SnapshotID != 0 {
		line += fmt.Sprintf(" snap=%d", ev.SnapshotID)
	}
	if ev.Detail != "" {
		line += " " + ev.Detail
	}
	return line
}

func (m *MachineMonitor) cmdHistory(cmd MonitorCommand) bool {
	if len(cmd.Args) > 0 && strings.EqualFold(cmd.Args[0], "config") {
		return m.cmdHistoryConfig(cmd)
	}
	if len(cmd.Args) == 0 || strings.EqualFold(cmd.Args[0], "horizon") {
		checkpoints, deltas, bytes := m.wholeHistoryStatsLocked()
		m.appendOutput(fmt.Sprintf("Whole-machine reverse horizon: %d snapshot(s), %d checkpoint(s), %d delta(s), capacity %d", len(m.wholeHistory), checkpoints, deltas, m.maxWholeHistory), colorCyan)
		m.appendOutput(fmt.Sprintf("Snapshot chain: checkpoint every %d delta(s) or %d MiB, retained checkpoints %d, delta bytes %d", m.wholeCheckpointInterval, m.wholeCheckpointBytes>>20, m.maxWholeCheckpoints, bytes), colorCyan)
		m.appendOutput(fmt.Sprintf("Snapshot devices registered: %d", len(m.devices)), colorCyan)
		if m.access != nil && m.access.HistoryEnabled() {
			m.appendOutput(fmt.Sprintf("Access timeline: %d recent event(s)", len(m.access.HistoryTail(0))), colorCyan)
		}
		return false
	}
	m.appendOutput("Usage: history horizon | history config [delta-interval] [delta-miB] [checkpoints] [snapshots]", colorRed)
	return false
}

func (m *MachineMonitor) cmdHistoryConfig(cmd MonitorCommand) bool {
	if len(cmd.Args) == 1 {
		m.appendOutput(fmt.Sprintf("Snapshot history config: delta-interval=%d delta-miB=%d checkpoints=%d snapshots=%d",
			m.wholeCheckpointInterval, m.wholeCheckpointBytes>>20, m.maxWholeCheckpoints, m.maxWholeHistory), colorCyan)
		return false
	}
	if len(cmd.Args) < 4 || len(cmd.Args) > 5 {
		m.appendOutput("Usage: history config [delta-interval] [delta-miB] [checkpoints] [snapshots]", colorRed)
		return false
	}
	interval, err1 := parseCount(cmd.Args[1])
	miB, err2 := parseCount(cmd.Args[2])
	checkpoints, err3 := parseCount(cmd.Args[3])
	snapshots := uint64(m.maxWholeHistory)
	var err4 error
	if len(cmd.Args) >= 5 {
		snapshots, err4 = parseCount(cmd.Args[4])
	}
	if err1 != nil || err2 != nil || err3 != nil || err4 != nil || interval == 0 || miB == 0 || checkpoints == 0 || snapshots == 0 {
		m.appendOutput("Invalid history config values", colorRed)
		return false
	}
	m.wholeCheckpointInterval = int(interval)
	m.wholeCheckpointBytes = miB << 20
	m.maxWholeCheckpoints = int(checkpoints)
	m.maxWholeHistory = int(snapshots)
	m.pruneWholeHistoryLocked()
	m.appendOutput(fmt.Sprintf("Snapshot history config: delta-interval=%d delta-miB=%d checkpoints=%d snapshots=%d",
		m.wholeCheckpointInterval, m.wholeCheckpointBytes>>20, m.maxWholeCheckpoints, m.maxWholeHistory), colorCyan)
	return false
}

func (m *MachineMonitor) wholeHistoryStatsLocked() (checkpoints, deltas int, bytes uint64) {
	for _, snap := range m.wholeHistory {
		if snap == nil {
			continue
		}
		bytes += snap.DeltaBytes
		if snap.Full {
			checkpoints++
		} else {
			deltas++
		}
	}
	return checkpoints, deltas, bytes
}

// --- Feature 11: I/O Register Viewer ---

func (m *MachineMonitor) cmdIOView(cmd MonitorCommand) bool {
	entry := m.cpus[m.focusedID]
	if entry == nil {
		m.appendOutput("No CPU focussed", colorRed)
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
		m.appendOutput("No CPU focussed", colorRed)
		return false
	}

	if len(cmd.Args) < 1 {
		m.appendOutput("Usage: e <addr>", colorRed)
		return false
	}

	addr, ok := m.evalAddress(cmd.Args[0], entry)
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
		if err := entry.CPU.ValidateAddress(addr); err != nil {
			m.appendOutput(err.Error(), colorRed)
			return
		}
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
	if err := entry.CPU.ValidateAddress(addr); err != nil {
		return
	}

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

	lines := splitScriptLines(string(data))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		cmds, err := splitSemicolonAware(line)
		if err != nil {
			m.scriptDepth--
			m.appendOutput(fmt.Sprintf("Script parse error: %s", err), colorRed)
			return false
		}
		for _, scriptCmd := range cmds {
			if m.executeCommand(scriptCmd) {
				m.scriptDepth--
				return true
			}
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
	cleaned, err := splitSemicolonAware(body)
	if err != nil {
		m.appendOutput(fmt.Sprintf("Invalid macro: %s", err), colorRed)
		return false
	}

	m.macros[name] = cleaned
	m.appendOutput(fmt.Sprintf("Macro '%s' defined (%d commands)", name, len(cleaned)), colorCyan)
	return false
}

func splitScriptLines(text string) []string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	return strings.Split(text, "\n")
}

func splitSemicolonAware(text string) ([]string, error) {
	var out []string
	var cur strings.Builder
	inQuote := false
	escaped := false
	for _, r := range text {
		switch {
		case escaped:
			cur.WriteRune(r)
			escaped = false
		case r == '\\' && inQuote:
			cur.WriteRune(r)
			escaped = true
		case r == '"':
			cur.WriteRune(r)
			inQuote = !inQuote
		case r == ';' && !inQuote:
			if s := strings.TrimSpace(cur.String()); s != "" {
				out = append(out, s)
			}
			cur.Reset()
		default:
			cur.WriteRune(r)
		}
	}
	if escaped || inQuote {
		return nil, fmt.Errorf("unterminated quote")
	}
	if s := strings.TrimSpace(cur.String()); s != "" {
		out = append(out, s)
	}
	return out, nil
}

func (m *MachineMonitor) executeMacro(cmds []string) bool {
	m.scriptDepth++
	if m.scriptDepth > 8 {
		m.scriptDepth--
		m.appendOutput("Macro recursion limit reached", colorRed)
		return false
	}

	if slices.ContainsFunc(cmds, m.executeCommand) {
		m.scriptDepth--
		return true
	}

	m.scriptDepth--
	return false
}
