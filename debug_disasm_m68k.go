// debug_disasm_m68k.go - M68K disassembler for Machine Monitor

package main

import (
	"fmt"
	"strings"
)

func readM68KWord(readMem func(addr uint64, size int) []byte, addr uint64) (uint16, bool) {
	data := readMem(addr, 2)
	if len(data) < 2 {
		return 0, false
	}
	return uint16(data[0])<<8 | uint16(data[1]), true
}

func readM68KLong(readMem func(addr uint64, size int) []byte, addr uint64) (uint32, bool) {
	data := readMem(addr, 4)
	if len(data) < 4 {
		return 0, false
	}
	return uint32(data[0])<<24 | uint32(data[1])<<16 | uint32(data[2])<<8 | uint32(data[3]), true
}

var m68kSizeNames = [4]string{".B", ".W", ".L", ""}
var m68kCondCodes = [16]string{
	"T", "F", "HI", "LS", "CC", "CS", "NE", "EQ",
	"VC", "VS", "PL", "MI", "GE", "LT", "GT", "LE",
}

func formatM68KEA(readMem func(addr uint64, size int) []byte, mode, reg int, addr *uint64, size int) string {
	switch mode {
	case 0: // Dn
		return fmt.Sprintf("D%d", reg)
	case 1: // An
		return fmt.Sprintf("A%d", reg)
	case 2: // (An)
		return fmt.Sprintf("(A%d)", reg)
	case 3: // (An)+
		return fmt.Sprintf("(A%d)+", reg)
	case 4: // -(An)
		return fmt.Sprintf("-(A%d)", reg)
	case 5: // d16(An)
		w, ok := readM68KWord(readMem, *addr)
		if !ok {
			return "d16(An)"
		}
		*addr += 2
		return fmt.Sprintf("%d(A%d)", int16(w), reg)
	case 6: // d8(An,Xn)
		w, ok := readM68KWord(readMem, *addr)
		if !ok {
			return "d8(An,Xn)"
		}
		*addr += 2
		xr := (w >> 12) & 7
		xl := "D"
		if w&0x8000 != 0 {
			xl = "A"
		}
		xs := ".W"
		if w&0x0800 != 0 {
			xs = ".L"
		}
		d8 := int8(w & 0xFF)
		return fmt.Sprintf("%d(A%d,%s%d%s)", d8, reg, xl, xr, xs)
	case 7:
		switch reg {
		case 0: // abs.W
			w, ok := readM68KWord(readMem, *addr)
			if !ok {
				return "$????"
			}
			*addr += 2
			return fmt.Sprintf("$%04X.W", w)
		case 1: // abs.L
			l, ok := readM68KLong(readMem, *addr)
			if !ok {
				return "$????????"
			}
			*addr += 4
			return fmt.Sprintf("$%08X", l)
		case 2: // d16(PC)
			w, ok := readM68KWord(readMem, *addr)
			if !ok {
				return "d16(PC)"
			}
			pcAddr := *addr
			*addr += 2
			target := uint32(pcAddr) + uint32(int16(w))
			return fmt.Sprintf("$%08X(PC)", target)
		case 3: // d8(PC,Xn)
			w, ok := readM68KWord(readMem, *addr)
			if !ok {
				return "d8(PC,Xn)"
			}
			*addr += 2
			xr := (w >> 12) & 7
			xl := "D"
			if w&0x8000 != 0 {
				xl = "A"
			}
			xs := ".W"
			if w&0x0800 != 0 {
				xs = ".L"
			}
			d8 := int8(w & 0xFF)
			return fmt.Sprintf("%d(PC,%s%d%s)", d8, xl, xr, xs)
		case 4: // #imm
			if size == 1 { // byte
				w, ok := readM68KWord(readMem, *addr)
				if !ok {
					return "#$??"
				}
				*addr += 2
				return fmt.Sprintf("#$%02X", w&0xFF)
			} else if size == 4 { // long
				l, ok := readM68KLong(readMem, *addr)
				if !ok {
					return "#$????????"
				}
				*addr += 4
				return fmt.Sprintf("#$%08X", l)
			} else { // word
				w, ok := readM68KWord(readMem, *addr)
				if !ok {
					return "#$????"
				}
				*addr += 2
				return fmt.Sprintf("#$%04X", w)
			}
		}
	}
	return "???"
}

func disassembleM68K(readMem func(addr uint64, size int) []byte, startAddr uint64, count int) []DisassembledLine {
	var lines []DisassembledLine
	addr := startAddr

	for i := 0; i < count; i++ {
		instrAddr := addr
		w, ok := readM68KWord(readMem, addr)
		if !ok {
			break
		}
		addr += 2

		mnemonic := decodeM68KInstruction(readMem, w, &addr)

		// Build hex bytes
		instrSize := int(addr - instrAddr)
		hexData := readMem(instrAddr, instrSize)
		var hexParts []string
		for j := 0; j < len(hexData); j += 2 {
			if j+1 < len(hexData) {
				hexParts = append(hexParts, fmt.Sprintf("%02X%02X", hexData[j], hexData[j+1]))
			} else {
				hexParts = append(hexParts, fmt.Sprintf("%02X", hexData[j]))
			}
		}

		lines = append(lines, DisassembledLine{
			Address:  instrAddr,
			HexBytes: strings.Join(hexParts, " "),
			Mnemonic: mnemonic,
			Size:     instrSize,
		})
	}
	return lines
}

func decodeM68KInstruction(readMem func(addr uint64, size int) []byte, w uint16, addr *uint64) string {
	group := (w >> 12) & 0xF

	switch group {
	case 0x0:
		return decodeM68KGroup0(readMem, w, addr)
	case 0x1: // MOVE.B
		return decodeM68KMove(readMem, w, addr, ".B", 1)
	case 0x2: // MOVE.L
		return decodeM68KMove(readMem, w, addr, ".L", 4)
	case 0x3: // MOVE.W
		return decodeM68KMove(readMem, w, addr, ".W", 2)
	case 0x4:
		return decodeM68KGroup4(readMem, w, addr)
	case 0x5:
		return decodeM68KGroup5(readMem, w, addr)
	case 0x6:
		return decodeM68KGroup6(readMem, w, addr)
	case 0x7: // MOVEQ
		if w&0x0100 == 0 {
			dn := (w >> 9) & 7
			data := int8(w & 0xFF)
			return fmt.Sprintf("MOVEQ #%d, D%d", data, dn)
		}
	case 0x8:
		return decodeM68KGroup8(readMem, w, addr)
	case 0x9:
		return decodeM68KArith(readMem, w, addr, "SUB")
	case 0xA:
		return fmt.Sprintf("dc.w $%04X ; Line-A", w)
	case 0xB:
		return decodeM68KGroupB(readMem, w, addr)
	case 0xC:
		return decodeM68KGroupC(readMem, w, addr)
	case 0xD:
		return decodeM68KArith(readMem, w, addr, "ADD")
	case 0xE:
		return decodeM68KGroupE(readMem, w, addr)
	case 0xF:
		return fmt.Sprintf("dc.w $%04X ; Line-F", w)
	}
	return fmt.Sprintf("dc.w $%04X", w)
}

func decodeM68KGroup0(readMem func(addr uint64, size int) []byte, w uint16, addr *uint64) string {
	if w&0x0100 != 0 {
		// BTST/BCHG/BCLR/BSET Dn,<ea>
		dn := (w >> 9) & 7
		mode := (w >> 3) & 7
		reg := w & 7
		op := (w >> 6) & 3
		ops := [4]string{"BTST", "BCHG", "BCLR", "BSET"}
		ea := formatM68KEA(readMem, int(mode), int(reg), addr, 1)
		return fmt.Sprintf("%s D%d, %s", ops[op], dn, ea)
	}

	switch (w >> 9) & 7 {
	case 0: // ORI
		sz := (w >> 6) & 3
		if w == 0x003C {
			imm, _ := readM68KWord(readMem, *addr)
			*addr += 2
			return fmt.Sprintf("ORI #$%02X, CCR", imm&0xFF)
		}
		if w == 0x007C {
			imm, _ := readM68KWord(readMem, *addr)
			*addr += 2
			return fmt.Sprintf("ORI #$%04X, SR", imm)
		}
		return decodeM68KImmediate(readMem, w, addr, "ORI", sz)
	case 1: // ANDI
		sz := (w >> 6) & 3
		if w == 0x023C {
			imm, _ := readM68KWord(readMem, *addr)
			*addr += 2
			return fmt.Sprintf("ANDI #$%02X, CCR", imm&0xFF)
		}
		if w == 0x027C {
			imm, _ := readM68KWord(readMem, *addr)
			*addr += 2
			return fmt.Sprintf("ANDI #$%04X, SR", imm)
		}
		return decodeM68KImmediate(readMem, w, addr, "ANDI", sz)
	case 2: // SUBI
		sz := (w >> 6) & 3
		return decodeM68KImmediate(readMem, w, addr, "SUBI", sz)
	case 3: // ADDI
		sz := (w >> 6) & 3
		return decodeM68KImmediate(readMem, w, addr, "ADDI", sz)
	case 4: // BTST/BCHG/BCLR/BSET #imm
		op := (w >> 6) & 3
		ops := [4]string{"BTST", "BCHG", "BCLR", "BSET"}
		imm, _ := readM68KWord(readMem, *addr)
		*addr += 2
		mode := (w >> 3) & 7
		reg := w & 7
		ea := formatM68KEA(readMem, int(mode), int(reg), addr, 1)
		return fmt.Sprintf("%s #%d, %s", ops[op], imm&31, ea)
	case 5: // EORI
		sz := (w >> 6) & 3
		if w == 0x0A3C {
			imm, _ := readM68KWord(readMem, *addr)
			*addr += 2
			return fmt.Sprintf("EORI #$%02X, CCR", imm&0xFF)
		}
		if w == 0x0A7C {
			imm, _ := readM68KWord(readMem, *addr)
			*addr += 2
			return fmt.Sprintf("EORI #$%04X, SR", imm)
		}
		return decodeM68KImmediate(readMem, w, addr, "EORI", sz)
	case 6: // CMPI
		sz := (w >> 6) & 3
		return decodeM68KImmediate(readMem, w, addr, "CMPI", sz)
	}
	return fmt.Sprintf("dc.w $%04X", w)
}

func decodeM68KImmediate(readMem func(addr uint64, size int) []byte, w uint16, addr *uint64, name string, sz uint16) string {
	sizeBytes := 2
	sizeName := ".W"
	switch sz {
	case 0:
		sizeName = ".B"
		sizeBytes = 1
	case 2:
		sizeName = ".L"
		sizeBytes = 4
	}

	var immStr string
	if sizeBytes == 4 {
		l, _ := readM68KLong(readMem, *addr)
		*addr += 4
		immStr = fmt.Sprintf("#$%08X", l)
	} else if sizeBytes == 1 {
		ww, _ := readM68KWord(readMem, *addr)
		*addr += 2
		immStr = fmt.Sprintf("#$%02X", ww&0xFF)
	} else {
		ww, _ := readM68KWord(readMem, *addr)
		*addr += 2
		immStr = fmt.Sprintf("#$%04X", ww)
	}

	mode := (w >> 3) & 7
	reg := w & 7
	ea := formatM68KEA(readMem, int(mode), int(reg), addr, sizeBytes)
	return fmt.Sprintf("%s%s %s, %s", name, sizeName, immStr, ea)
}

func decodeM68KMove(readMem func(addr uint64, size int) []byte, w uint16, addr *uint64, sizeName string, sizeBytes int) string {
	srcMode := (w >> 3) & 7
	srcReg := w & 7
	dstReg := (w >> 9) & 7
	dstMode := (w >> 6) & 7

	src := formatM68KEA(readMem, int(srcMode), int(srcReg), addr, sizeBytes)
	dst := formatM68KEA(readMem, int(dstMode), int(dstReg), addr, sizeBytes)

	// MOVEA
	if dstMode == 1 {
		return fmt.Sprintf("MOVEA%s %s, A%d", sizeName, src, dstReg)
	}
	return fmt.Sprintf("MOVE%s %s, %s", sizeName, src, dst)
}

func decodeM68KGroup4(readMem func(addr uint64, size int) []byte, w uint16, addr *uint64) string {
	if w == 0x4E70 {
		return "RESET"
	}
	if w == 0x4E71 {
		return "NOP"
	}
	if w == 0x4E73 {
		return "RTE"
	}
	if w == 0x4E75 {
		return "RTS"
	}
	if w == 0x4E76 {
		return "TRAPV"
	}
	if w == 0x4E77 {
		return "RTR"
	}
	if w&0xFFF8 == 0x4E50 { // LINK
		reg := w & 7
		disp, _ := readM68KWord(readMem, *addr)
		*addr += 2
		return fmt.Sprintf("LINK A%d, #%d", reg, int16(disp))
	}
	if w&0xFFF8 == 0x4E58 { // UNLK
		return fmt.Sprintf("UNLK A%d", w&7)
	}
	if w&0xFFF0 == 0x4E40 { // TRAP #n
		return fmt.Sprintf("TRAP #%d", w&0xF)
	}
	if w&0xFFF8 == 0x4E60 { // MOVE USP
		reg := w & 7
		if w&0x0008 != 0 {
			return fmt.Sprintf("MOVE USP, A%d", reg)
		}
		return fmt.Sprintf("MOVE A%d, USP", reg)
	}
	if w&0xFFC0 == 0x4E80 { // JSR
		mode := (w >> 3) & 7
		reg := w & 7
		ea := formatM68KEA(readMem, int(mode), int(reg), addr, 4)
		return fmt.Sprintf("JSR %s", ea)
	}
	if w&0xFFC0 == 0x4EC0 { // JMP
		mode := (w >> 3) & 7
		reg := w & 7
		ea := formatM68KEA(readMem, int(mode), int(reg), addr, 4)
		return fmt.Sprintf("JMP %s", ea)
	}
	if w&0xFB80 == 0x4880 { // MOVEM
		dir := (w >> 10) & 1
		sz := (w >> 6) & 1
		mode := (w >> 3) & 7
		reg := w & 7
		mask, _ := readM68KWord(readMem, *addr)
		*addr += 2
		sizeName := ".W"
		sizeBytes := 2
		if sz == 1 {
			sizeName = ".L"
			sizeBytes = 4
		}
		ea := formatM68KEA(readMem, int(mode), int(reg), addr, sizeBytes)
		regList := formatM68KRegList(mask, mode == 4)
		if dir == 0 {
			return fmt.Sprintf("MOVEM%s %s, %s", sizeName, regList, ea)
		}
		return fmt.Sprintf("MOVEM%s %s, %s", sizeName, ea, regList)
	}
	if w&0xFF00 == 0x4200 { // CLR
		sz := (w >> 6) & 3
		mode := (w >> 3) & 7
		reg := w & 7
		sizeBytes := 1 << sz
		if sz == 3 {
			sizeBytes = 1
		}
		ea := formatM68KEA(readMem, int(mode), int(reg), addr, sizeBytes)
		return fmt.Sprintf("CLR%s %s", m68kSizeNames[sz], ea)
	}
	if w&0xFF00 == 0x4A00 { // TST
		sz := (w >> 6) & 3
		mode := (w >> 3) & 7
		reg := w & 7
		sizeBytes := 1 << sz
		if sz == 3 {
			sizeBytes = 1
		}
		ea := formatM68KEA(readMem, int(mode), int(reg), addr, sizeBytes)
		return fmt.Sprintf("TST%s %s", m68kSizeNames[sz], ea)
	}
	if w&0xFF00 == 0x4400 { // NEG
		sz := (w >> 6) & 3
		mode := (w >> 3) & 7
		reg := w & 7
		sizeBytes := 1 << sz
		if sz == 3 {
			sizeBytes = 1
		}
		ea := formatM68KEA(readMem, int(mode), int(reg), addr, sizeBytes)
		return fmt.Sprintf("NEG%s %s", m68kSizeNames[sz], ea)
	}
	if w&0xFF00 == 0x4000 { // NEGX
		sz := (w >> 6) & 3
		mode := (w >> 3) & 7
		reg := w & 7
		sizeBytes := 1 << sz
		if sz == 3 {
			sizeBytes = 1
		}
		ea := formatM68KEA(readMem, int(mode), int(reg), addr, sizeBytes)
		return fmt.Sprintf("NEGX%s %s", m68kSizeNames[sz], ea)
	}
	if w&0xFF00 == 0x4600 { // NOT
		sz := (w >> 6) & 3
		mode := (w >> 3) & 7
		reg := w & 7
		sizeBytes := 1 << sz
		if sz == 3 {
			sizeBytes = 1
		}
		ea := formatM68KEA(readMem, int(mode), int(reg), addr, sizeBytes)
		return fmt.Sprintf("NOT%s %s", m68kSizeNames[sz], ea)
	}
	if w&0xFFC0 == 0x4800 { // NBCD
		mode := (w >> 3) & 7
		reg := w & 7
		ea := formatM68KEA(readMem, int(mode), int(reg), addr, 1)
		return fmt.Sprintf("NBCD %s", ea)
	}
	if w&0xFFF8 == 0x4840 { // SWAP
		return fmt.Sprintf("SWAP D%d", w&7)
	}
	if w&0xFFC0 == 0x4840 { // PEA
		mode := (w >> 3) & 7
		reg := w & 7
		ea := formatM68KEA(readMem, int(mode), int(reg), addr, 4)
		return fmt.Sprintf("PEA %s", ea)
	}
	if w&0xFFF8 == 0x4880 { // EXT.W
		return fmt.Sprintf("EXT.W D%d", w&7)
	}
	if w&0xFFF8 == 0x48C0 { // EXT.L
		return fmt.Sprintf("EXT.L D%d", w&7)
	}
	if w&0xFFC0 == 0x4AC0 { // TAS
		mode := (w >> 3) & 7
		reg := w & 7
		ea := formatM68KEA(readMem, int(mode), int(reg), addr, 1)
		return fmt.Sprintf("TAS %s", ea)
	}
	if w&0xF1C0 == 0x41C0 { // LEA
		an := (w >> 9) & 7
		mode := (w >> 3) & 7
		reg := w & 7
		ea := formatM68KEA(readMem, int(mode), int(reg), addr, 4)
		return fmt.Sprintf("LEA %s, A%d", ea, an)
	}
	if w&0xF1C0 == 0x4180 { // CHK
		dn := (w >> 9) & 7
		mode := (w >> 3) & 7
		reg := w & 7
		ea := formatM68KEA(readMem, int(mode), int(reg), addr, 2)
		return fmt.Sprintf("CHK %s, D%d", ea, dn)
	}
	return fmt.Sprintf("dc.w $%04X", w)
}

func decodeM68KGroup5(readMem func(addr uint64, size int) []byte, w uint16, addr *uint64) string {
	if w&0xF0F8 == 0x50C8 { // DBcc
		cond := (w >> 8) & 0xF
		reg := w & 7
		disp, _ := readM68KWord(readMem, *addr)
		target := uint32(*addr) + uint32(int16(disp))
		*addr += 2
		return fmt.Sprintf("DB%s D%d, $%08X", m68kCondCodes[cond], reg, target)
	}
	if w&0xF0C0 == 0x50C0 { // Scc
		cond := (w >> 8) & 0xF
		mode := (w >> 3) & 7
		reg := w & 7
		ea := formatM68KEA(readMem, int(mode), int(reg), addr, 1)
		return fmt.Sprintf("S%s %s", m68kCondCodes[cond], ea)
	}
	// ADDQ/SUBQ
	sz := (w >> 6) & 3
	if sz == 3 {
		return fmt.Sprintf("dc.w $%04X", w)
	}
	data := (w >> 9) & 7
	if data == 0 {
		data = 8
	}
	mode := (w >> 3) & 7
	reg := w & 7
	sizeBytes := 1 << sz
	ea := formatM68KEA(readMem, int(mode), int(reg), addr, sizeBytes)
	op := "ADDQ"
	if w&0x0100 != 0 {
		op = "SUBQ"
	}
	return fmt.Sprintf("%s%s #%d, %s", op, m68kSizeNames[sz], data, ea)
}

func decodeM68KGroup6(readMem func(addr uint64, size int) []byte, w uint16, addr *uint64) string {
	cond := (w >> 8) & 0xF
	disp := int8(w & 0xFF)

	op := "B" + m68kCondCodes[cond]
	if cond == 0 {
		op = "BRA"
	} else if cond == 1 {
		op = "BSR"
	}

	if disp == 0 {
		// 16-bit displacement
		d16, _ := readM68KWord(readMem, *addr)
		*addr += 2
		target := uint32(*addr-2) + uint32(int16(d16))
		return fmt.Sprintf("%s.W $%08X", op, target)
	}
	if disp == -1 {
		// 32-bit displacement (68020+)
		d32, _ := readM68KLong(readMem, *addr)
		*addr += 4
		target := uint32(*addr-4) + d32
		return fmt.Sprintf("%s.L $%08X", op, target)
	}
	target := uint32(*addr) + uint32(int32(disp))
	return fmt.Sprintf("%s.S $%08X", op, target)
}

func decodeM68KGroup8(readMem func(addr uint64, size int) []byte, w uint16, addr *uint64) string {
	if w&0xF1C0 == 0x80C0 { // DIVU
		dn := (w >> 9) & 7
		mode := (w >> 3) & 7
		reg := w & 7
		ea := formatM68KEA(readMem, int(mode), int(reg), addr, 2)
		return fmt.Sprintf("DIVU.W %s, D%d", ea, dn)
	}
	if w&0xF1C0 == 0x81C0 { // DIVS
		dn := (w >> 9) & 7
		mode := (w >> 3) & 7
		reg := w & 7
		ea := formatM68KEA(readMem, int(mode), int(reg), addr, 2)
		return fmt.Sprintf("DIVS.W %s, D%d", ea, dn)
	}
	if w&0xF1F0 == 0x8100 { // SBCD Dy,Dx
		dx := (w >> 9) & 7
		dy := w & 7
		if w&0x0008 != 0 {
			return fmt.Sprintf("SBCD -(A%d), -(A%d)", dy, dx)
		}
		return fmt.Sprintf("SBCD D%d, D%d", dy, dx)
	}
	// OR
	dn := (w >> 9) & 7
	dir := (w >> 8) & 1
	sz := (w >> 6) & 3
	mode := (w >> 3) & 7
	reg := w & 7
	sizeBytes := 1 << sz
	if sz == 3 {
		return fmt.Sprintf("dc.w $%04X", w)
	}
	ea := formatM68KEA(readMem, int(mode), int(reg), addr, sizeBytes)
	if dir == 0 {
		return fmt.Sprintf("OR%s %s, D%d", m68kSizeNames[sz], ea, dn)
	}
	return fmt.Sprintf("OR%s D%d, %s", m68kSizeNames[sz], dn, ea)
}

func decodeM68KGroupB(readMem func(addr uint64, size int) []byte, w uint16, addr *uint64) string {
	if w&0xF138 == 0xB108 { // CMPM
		sz := (w >> 6) & 3
		ax := (w >> 9) & 7
		ay := w & 7
		return fmt.Sprintf("CMPM%s (A%d)+, (A%d)+", m68kSizeNames[sz], ay, ax)
	}
	dn := (w >> 9) & 7
	sz := (w >> 6) & 3
	mode := (w >> 3) & 7
	reg := w & 7
	if sz == 3 {
		// CMPA
		sizeBytes := 2
		sizeName := ".W"
		if w&0x0100 != 0 {
			sizeBytes = 4
			sizeName = ".L"
		}
		ea := formatM68KEA(readMem, int(mode), int(reg), addr, sizeBytes)
		return fmt.Sprintf("CMPA%s %s, A%d", sizeName, ea, dn)
	}
	sizeBytes := 1 << sz
	ea := formatM68KEA(readMem, int(mode), int(reg), addr, sizeBytes)
	if w&0x0100 != 0 {
		return fmt.Sprintf("EOR%s D%d, %s", m68kSizeNames[sz], dn, ea)
	}
	return fmt.Sprintf("CMP%s %s, D%d", m68kSizeNames[sz], ea, dn)
}

func decodeM68KGroupC(readMem func(addr uint64, size int) []byte, w uint16, addr *uint64) string {
	if w&0xF1C0 == 0xC0C0 { // MULU
		dn := (w >> 9) & 7
		mode := (w >> 3) & 7
		reg := w & 7
		ea := formatM68KEA(readMem, int(mode), int(reg), addr, 2)
		return fmt.Sprintf("MULU.W %s, D%d", ea, dn)
	}
	if w&0xF1C0 == 0xC1C0 { // MULS
		dn := (w >> 9) & 7
		mode := (w >> 3) & 7
		reg := w & 7
		ea := formatM68KEA(readMem, int(mode), int(reg), addr, 2)
		return fmt.Sprintf("MULS.W %s, D%d", ea, dn)
	}
	if w&0xF1F0 == 0xC100 { // ABCD
		dx := (w >> 9) & 7
		dy := w & 7
		if w&0x0008 != 0 {
			return fmt.Sprintf("ABCD -(A%d), -(A%d)", dy, dx)
		}
		return fmt.Sprintf("ABCD D%d, D%d", dy, dx)
	}
	if w&0xF1F8 == 0xC140 { // EXG Dn,Dn
		rx := (w >> 9) & 7
		ry := w & 7
		return fmt.Sprintf("EXG D%d, D%d", rx, ry)
	}
	if w&0xF1F8 == 0xC148 { // EXG An,An
		rx := (w >> 9) & 7
		ry := w & 7
		return fmt.Sprintf("EXG A%d, A%d", rx, ry)
	}
	if w&0xF1F8 == 0xC188 { // EXG Dn,An
		dn := (w >> 9) & 7
		an := w & 7
		return fmt.Sprintf("EXG D%d, A%d", dn, an)
	}
	// AND
	dn := (w >> 9) & 7
	dir := (w >> 8) & 1
	sz := (w >> 6) & 3
	mode := (w >> 3) & 7
	reg := w & 7
	if sz == 3 {
		return fmt.Sprintf("dc.w $%04X", w)
	}
	sizeBytes := 1 << sz
	ea := formatM68KEA(readMem, int(mode), int(reg), addr, sizeBytes)
	if dir == 0 {
		return fmt.Sprintf("AND%s %s, D%d", m68kSizeNames[sz], ea, dn)
	}
	return fmt.Sprintf("AND%s D%d, %s", m68kSizeNames[sz], dn, ea)
}

func decodeM68KArith(readMem func(addr uint64, size int) []byte, w uint16, addr *uint64, base string) string {
	dn := (w >> 9) & 7
	sz := (w >> 6) & 3
	mode := (w >> 3) & 7
	reg := w & 7

	if sz == 3 {
		// ADDA/SUBA
		sizeBytes := 2
		sizeName := ".W"
		if w&0x0100 != 0 {
			sizeBytes = 4
			sizeName = ".L"
		}
		ea := formatM68KEA(readMem, int(mode), int(reg), addr, sizeBytes)
		return fmt.Sprintf("%sA%s %s, A%d", base, sizeName, ea, dn)
	}

	dir := (w >> 8) & 1
	if dir == 1 && mode <= 1 {
		// ADDX/SUBX
		rx := (w >> 9) & 7
		ry := w & 7
		op := base + "X"
		if w&0x0008 != 0 {
			return fmt.Sprintf("%s%s -(A%d), -(A%d)", op, m68kSizeNames[sz], ry, rx)
		}
		return fmt.Sprintf("%s%s D%d, D%d", op, m68kSizeNames[sz], ry, rx)
	}

	sizeBytes := 1 << sz
	ea := formatM68KEA(readMem, int(mode), int(reg), addr, sizeBytes)
	if dir == 0 {
		return fmt.Sprintf("%s%s %s, D%d", base, m68kSizeNames[sz], ea, dn)
	}
	return fmt.Sprintf("%s%s D%d, %s", base, m68kSizeNames[sz], dn, ea)
}

func decodeM68KGroupE(readMem func(addr uint64, size int) []byte, w uint16, addr *uint64) string {
	sz := (w >> 6) & 3
	if sz == 3 {
		// Memory shifts (word only)
		dir := (w >> 8) & 1
		op := (w >> 9) & 3
		ops := [4]string{"ASR", "LSR", "ROXR", "ROR"}
		if dir == 1 {
			ops = [4]string{"ASL", "LSL", "ROXL", "ROL"}
		}
		mode := (w >> 3) & 7
		reg := w & 7
		ea := formatM68KEA(readMem, int(mode), int(reg), addr, 2)
		return fmt.Sprintf("%s %s", ops[op], ea)
	}

	dir := (w >> 8) & 1
	ir := (w >> 5) & 1
	op := (w >> 3) & 3
	dn := w & 7
	count := (w >> 9) & 7

	ops := [4]string{"ASR", "LSR", "ROXR", "ROR"}
	if dir == 1 {
		ops = [4]string{"ASL", "LSL", "ROXL", "ROL"}
	}

	if ir == 0 {
		if count == 0 {
			count = 8
		}
		return fmt.Sprintf("%s%s #%d, D%d", ops[op], m68kSizeNames[sz], count, dn)
	}
	return fmt.Sprintf("%s%s D%d, D%d", ops[op], m68kSizeNames[sz], count, dn)
}

func formatM68KRegList(mask uint16, reversed bool) string {
	var parts []string
	names := [16]string{"D0", "D1", "D2", "D3", "D4", "D5", "D6", "D7",
		"A0", "A1", "A2", "A3", "A4", "A5", "A6", "A7"}

	for i := 0; i < 16; i++ {
		bit := i
		if reversed {
			bit = 15 - i
		}
		if mask&(1<<uint(bit)) != 0 {
			parts = append(parts, names[i])
		}
	}
	if len(parts) == 0 {
		return "<none>"
	}
	return strings.Join(parts, "/")
}
