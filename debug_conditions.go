// debug_conditions.go - Breakpoint condition parser and evaluator for Machine Monitor

package main

import (
	"fmt"
	"strings"
)

// ParseCondition parses a condition string into a BreakpointCondition.
// Formats:
//
//	r1==$FF        - register R1, op ==, value 0xFF
//	[$1000]==$42   - memory byte at 0x1000, op ==, value 0x42
//	[$1000].W==$42 - memory word at 0x1000
//	[$1000].L==$42 - memory long at 0x1000
//	hitcount>10    - hit count, op >, value 10
func ParseCondition(text string) (*BreakpointCondition, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, fmt.Errorf("empty condition")
	}

	// Find operator
	var op ConditionOp
	var opStr string
	var opIdx int

	for _, candidate := range []string{"==", "!=", "<=", ">=", "<", ">"} {
		idx := strings.Index(text, candidate)
		if idx >= 0 {
			opStr = candidate
			opIdx = idx
			break
		}
	}

	if opStr == "" {
		return nil, fmt.Errorf("no operator found (use ==, !=, <, >, <=, >=)")
	}

	switch opStr {
	case "==":
		op = CondOpEqual
	case "!=":
		op = CondOpNotEqual
	case "<":
		op = CondOpLess
	case ">":
		op = CondOpGreater
	case "<=":
		op = CondOpLessEqual
	case ">=":
		op = CondOpGreaterEqual
	}

	lhs := strings.TrimSpace(text[:opIdx])
	rhs := strings.TrimSpace(text[opIdx+len(opStr):])

	value, ok := ParseAddress(rhs)
	if !ok {
		return nil, fmt.Errorf("invalid value: %s", rhs)
	}

	// Memory dereference: [$1000], optionally suffixed with .B/.W/.L.
	if strings.HasPrefix(lhs, "[") {
		width := uint8(1)
		memExpr := lhs
		if idx := strings.LastIndex(lhs, "]."); idx >= 0 {
			memExpr = lhs[:idx+1]
			switch strings.ToUpper(lhs[idx+2:]) {
			case "B":
				width = 1
			case "W":
				width = 2
			case "L":
				width = 4
			default:
				return nil, fmt.Errorf("invalid memory width: %s", lhs[idx+2:])
			}
		}
		if !strings.HasSuffix(memExpr, "]") {
			return nil, fmt.Errorf("invalid memory dereference: %s", lhs)
		}
		addrStr := memExpr[1 : len(memExpr)-1]
		addr, ok := ParseAddress(addrStr)
		if !ok {
			return nil, fmt.Errorf("invalid memory address: %s", addrStr)
		}
		return &BreakpointCondition{
			Source:  CondSourceMemory,
			MemAddr: addr,
			Width:   width,
			Op:      op,
			Value:   value,
		}, nil
	}

	// Hit count: hitcount
	if strings.EqualFold(lhs, "hitcount") {
		return &BreakpointCondition{
			Source: CondSourceHitCount,
			Op:     op,
			Value:  value,
		}, nil
	}

	// Register name
	return &BreakpointCondition{
		Source:  CondSourceRegister,
		RegName: strings.ToUpper(lhs),
		Op:      op,
		Value:   value,
	}, nil
}

// evaluateCondition checks whether a breakpoint condition is satisfied.
// Returns true if cond is nil (unconditional) or the condition holds.
func evaluateCondition(cond *BreakpointCondition, cpu DebuggableCPU) bool {
	if cond == nil {
		return true
	}

	var actual uint64
	switch cond.Source {
	case CondSourceRegister:
		val, ok := cpu.GetRegister(cond.RegName)
		if !ok {
			return false // unknown register - don't fire
		}
		actual = val

	case CondSourceMemory:
		width := conditionWidth(cond)
		data := cpu.ReadMemory(cond.MemAddr, int(width))
		if len(data) < int(width) {
			return false
		}
		actual = bytesToConditionValue(data, conditionLittleEndian(cpu))

	case CondSourceHitCount:
		// Hit count is passed via the ConditionalBreakpoint.HitCount field.
		// The caller must set actual from bp.HitCount before calling.
		// Since we don't have access to the bp here, we use a sentinel:
		// the trapLoop increments HitCount before calling us, so we
		// can't get it here. Instead, the caller should pass hitcount
		// through a different mechanism. For simplicity, hitcount
		// conditions are evaluated in the trapLoop directly.
		return false
	}

	return compareValues(actual, cond.Op, cond.Value)
}

// evaluateConditionWithHitCount evaluates a condition, using the provided
// hit count for CondSourceHitCount conditions.
func evaluateConditionWithHitCount(cond *BreakpointCondition, cpu DebuggableCPU, hitCount uint64) bool {
	if cond == nil {
		return true
	}

	var actual uint64
	switch cond.Source {
	case CondSourceRegister:
		val, ok := cpu.GetRegister(cond.RegName)
		if !ok {
			return false
		}
		actual = val
	case CondSourceMemory:
		width := conditionWidth(cond)
		data := cpu.ReadMemory(cond.MemAddr, int(width))
		if len(data) < int(width) {
			return false
		}
		actual = bytesToConditionValue(data, conditionLittleEndian(cpu))
	case CondSourceHitCount:
		actual = hitCount
	}

	return compareValues(actual, cond.Op, cond.Value)
}

func compareValues(actual uint64, op ConditionOp, expected uint64) bool {
	switch op {
	case CondOpEqual:
		return actual == expected
	case CondOpNotEqual:
		return actual != expected
	case CondOpLess:
		return actual < expected
	case CondOpGreater:
		return actual > expected
	case CondOpLessEqual:
		return actual <= expected
	case CondOpGreaterEqual:
		return actual >= expected
	}
	return false
}

func conditionWidth(cond *BreakpointCondition) uint8 {
	if cond == nil || cond.Width == 0 {
		return 1
	}
	if cond.Width == 2 || cond.Width == 4 {
		return cond.Width
	}
	return 1
}

func conditionLittleEndian(cpu DebuggableCPU) bool {
	if cpu == nil {
		return true
	}
	switch strings.ToUpper(cpu.CPUName()) {
	case "M68K":
		return false
	default:
		return true
	}
}

func bytesToConditionValue(data []byte, littleEndian bool) uint64 {
	var v uint64
	if littleEndian {
		for i := len(data) - 1; i >= 0; i-- {
			v = (v << 8) | uint64(data[i])
		}
	} else {
		for _, b := range data {
			v = (v << 8) | uint64(b)
		}
	}
	return v
}

// FormatCondition returns a human-readable string for a condition.
func FormatCondition(cond *BreakpointCondition) string {
	if cond == nil {
		return ""
	}

	var lhs string
	switch cond.Source {
	case CondSourceRegister:
		lhs = cond.RegName
	case CondSourceMemory:
		lhs = fmt.Sprintf("[$%X]", cond.MemAddr)
		switch conditionWidth(cond) {
		case 2:
			lhs += ".W"
		case 4:
			lhs += ".L"
		}
	case CondSourceHitCount:
		lhs = "hitcount"
	}

	var opStr string
	switch cond.Op {
	case CondOpEqual:
		opStr = "=="
	case CondOpNotEqual:
		opStr = "!="
	case CondOpLess:
		opStr = "<"
	case CondOpGreater:
		opStr = ">"
	case CondOpLessEqual:
		opStr = "<="
	case CondOpGreaterEqual:
		opStr = ">="
	}

	return fmt.Sprintf("%s%s$%X", lhs, opStr, cond.Value)
}
