package main

import "fmt"

func formatMonitorRegisterLine(reg RegisterInfo, addrWidth int) string {
	width := 16
	if addrWidth <= 16 {
		width = 4
	} else if addrWidth <= 32 && reg.BitWidth <= 32 {
		width = 8
	}
	return fmt.Sprintf("%-4s $%0*X", reg.Name, width, reg.Value)
}
