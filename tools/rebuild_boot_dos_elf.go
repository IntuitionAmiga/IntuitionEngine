package main

import (
	"bytes"
	"encoding/binary"
	"flag"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
)

const (
	elfMachineIE64 = 0x4945
	pageSize       = 0x1000
	baseVA         = 0x600000
	listingBias    = 0x1000
)

var labelRe = regexp.MustCompile(`^([0-9A-F]{8})\s+.*$`)

func main() {
	listingPath := flag.String("listing", "", "path to ie64asm listing file")
	imagePath := flag.String("image", "", "path to assembled iexec.ie64 image")
	outPath := flag.String("out", "", "output path for boot_dos_library.elf")
	label := flag.String("label", "prog_doslib", "flat program label to export as ELF")
	flag.Parse()

	if *listingPath == "" || *imagePath == "" || *outPath == "" {
		fmt.Fprintln(os.Stderr, "usage: rebuild_boot_dos_elf -listing iexec.lst -image iexec.ie64 -out out.elf [-label prog_doslib]")
		os.Exit(2)
	}

	progStart, err := parseProgramStart(*listingPath, *label)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	image, err := os.ReadFile(*imagePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read image: %v\n", err)
		os.Exit(1)
	}
	if uint64(len(image)) < progStart+32 {
		fmt.Fprintf(os.Stderr, "image too small for prog_doslib header at 0x%X\n", progStart)
		os.Exit(1)
	}

	codeSize := binary.LittleEndian.Uint32(image[progStart+8 : progStart+12])
	dataSize := binary.LittleEndian.Uint32(image[progStart+12 : progStart+16])
	codeStart := progStart + 32
	codeEnd := codeStart + uint64(codeSize)
	dataStart := codeEnd
	dataEnd := dataStart + uint64(dataSize)
	if uint64(len(image)) < dataEnd {
		fmt.Fprintf(os.Stderr, "image too small: len=0x%X need dataEnd=0x%X\n", len(image), dataEnd)
		os.Exit(1)
	}

	code := image[codeStart:codeEnd]
	data := image[dataStart:dataEnd]
	elf := buildELF(code, data)

	if err := os.WriteFile(*outPath, elf, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "write output: %v\n", err)
		os.Exit(1)
	}
}

func parseProgramStart(path string, label string) (uint64, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return 0, fmt.Errorf("read listing: %w", err)
	}
	lines := bytes.Split(body, []byte{'\n'})
	for i, raw := range lines {
		line := string(raw)
		if strings.Contains(line, label+":") {
			return parseNextAddress(lines, i+1)
		}
	}
	return 0, fmt.Errorf("missing %s start in listing", label)
}

func parseNextAddress(lines [][]byte, start int) (uint64, error) {
	for i := start; i < len(lines); i++ {
		if m := labelRe.FindSubmatch(lines[i]); m != nil {
			addr, err := strconv.ParseUint(string(m[1]), 16, 64)
			if err != nil {
				return 0, err
			}
			if addr < listingBias {
				return 0, fmt.Errorf("listing address 0x%X below bias", addr)
			}
			return addr - listingBias, nil
		}
	}
	return 0, fmt.Errorf("could not find next address in listing")
}

func buildELF(code []byte, data []byte) []byte {
	codeFileOff := uint64(pageSize)
	codeFileSize := uint64(len(code))
	codeMemSize := roundUp(codeFileSize, pageSize)
	dataVA := baseVA + codeMemSize
	dataFileOff := codeFileOff + codeMemSize
	dataFileSize := uint64(len(data))
	dataMemSize := roundUp(dataFileSize, pageSize)

	out := make([]byte, dataFileOff+dataFileSize)
	copy(out[codeFileOff:], code)
	copy(out[dataFileOff:], data)

	copy(out[0:16], []byte{0x7F, 'E', 'L', 'F', 2, 1, 1})
	binary.LittleEndian.PutUint16(out[16:18], 2)
	binary.LittleEndian.PutUint16(out[18:20], elfMachineIE64)
	binary.LittleEndian.PutUint32(out[20:24], 1)
	binary.LittleEndian.PutUint64(out[24:32], baseVA)
	binary.LittleEndian.PutUint64(out[32:40], 64)
	binary.LittleEndian.PutUint16(out[52:54], 64)
	binary.LittleEndian.PutUint16(out[54:56], 56)
	binary.LittleEndian.PutUint16(out[56:58], 2)

	putProgramHeader(out[64:120], codeFileOff, baseVA, codeFileSize, codeMemSize, 5)
	putProgramHeader(out[120:176], dataFileOff, dataVA, dataFileSize, dataMemSize, 6)

	return out
}

func putProgramHeader(dst []byte, off uint64, vaddr uint64, filesz uint64, memsz uint64, flags uint32) {
	binary.LittleEndian.PutUint32(dst[0:4], 1)
	binary.LittleEndian.PutUint32(dst[4:8], flags)
	binary.LittleEndian.PutUint64(dst[8:16], off)
	binary.LittleEndian.PutUint64(dst[16:24], vaddr)
	binary.LittleEndian.PutUint64(dst[24:32], 0)
	binary.LittleEndian.PutUint64(dst[32:40], filesz)
	binary.LittleEndian.PutUint64(dst[40:48], memsz)
	binary.LittleEndian.PutUint64(dst[48:56], pageSize)
}

func roundUp(v uint64, align uint64) uint64 {
	if v == 0 {
		return 0
	}
	return (v + align - 1) &^ (align - 1)
}
