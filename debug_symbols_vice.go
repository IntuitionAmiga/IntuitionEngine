package main

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
)

func (st *SymbolTable) LoadVICELabels(cpu string, r io.Reader, base uint64) error {
	scanner := bufio.NewScanner(r)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 3 || strings.ToLower(fields[0]) != "al" {
			continue
		}
		addrText := fields[1]
		if idx := strings.LastIndex(addrText, ":"); idx >= 0 {
			addrText = addrText[idx+1:]
		}
		addrText = strings.TrimPrefix(strings.TrimPrefix(addrText, "$"), "0x")
		addr, err := strconv.ParseUint(addrText, 16, 64)
		if err != nil {
			return fmt.Errorf("VICE label line %d: invalid address %q", lineNo, fields[1])
		}
		st.Add(cpu, base+addr, fields[2], 0, SymbolLabel)
	}
	return scanner.Err()
}
