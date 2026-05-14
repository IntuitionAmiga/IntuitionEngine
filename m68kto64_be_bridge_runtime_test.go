package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func transpileAndAssembleWithArgs(t *testing.T, m68kSrc string, extraArgs ...string) []byte {
	t.Helper()
	m68 := findM68KTo64ForTest(t)
	asm := findIE64AsmForTest(t)
	if m68 == "" || asm == "" {
		t.Skip("sdk/bin/m68kto64 or sdk/bin/ie64asm missing (run `make m68kto64 ie64asm`)")
	}

	dir := t.TempDir()
	mPath := filepath.Join(dir, "in.s")
	iePath := filepath.Join(dir, "out_ie64.s")
	binPath := filepath.Join(dir, "out.bin")
	if err := os.WriteFile(mPath, []byte(m68kSrc), 0o644); err != nil {
		t.Fatalf("write m68k source: %v", err)
	}

	args := append([]string{"-no-header", "-o", iePath}, extraArgs...)
	args = append(args, mPath)
	cmd := exec.Command(m68, args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("m68kto64 failed: %v\n%s", err, out)
	}
	body, err := os.ReadFile(iePath)
	if err != nil {
		t.Fatalf("read transpiled: %v", err)
	}
	wrapped := "\torg $1000\ntest_entry:\n" + string(body) + "\n\thalt\n"
	wrapPath := filepath.Join(dir, "wrapped.s")
	if err := os.WriteFile(wrapPath, []byte(wrapped), 0o644); err != nil {
		t.Fatalf("write wrapped source: %v", err)
	}
	cmd = exec.Command(asm, "-o", binPath, wrapPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("ie64asm failed: %v\n--- m68kto64 output ---\n%s\n--- asm err ---\n%s", err, body, out)
	}
	bin, err := os.ReadFile(binPath)
	if err != nil {
		t.Fatalf("read bin: %v", err)
	}
	return bin
}

func runToHaltOnBus(t *testing.T, bus *MachineBus, bin []byte, maxSteps int) *CPU64 {
	t.Helper()
	cpu := NewCPU64(bus)
	cpu.LoadProgramBytes(bin)
	cpu.PC = PROG_START
	for i := 0; i < maxSteps; i++ {
		if cpu.PC == 0 || cpu.memory[cpu.PC] == OP_HALT64 {
			return cpu
		}
		cpu.StepOne()
	}
	t.Fatalf("CPU did not halt within %d steps; PC=%#x", maxSteps, cpu.PC)
	return cpu
}

func TestM68KTo64_BEBridge_RAMWordAndLongLayout(t *testing.T) {
	src := `
		move.l #$80000,a0
		move.w #$1234,(a0)
		move.l #$89ABCDEF,2(a0)
		moveq #0,d0
		moveq #0,d1
		move.w (a0),d0
		move.l 2(a0),d1
	`
	bin := transpileAndAssembleWithArgs(t, src)
	bus := NewMachineBus()
	cpu := runToHaltOnBus(t, bus, bin, 2000)

	wantBytes := []byte{0x12, 0x34, 0x89, 0xAB, 0xCD, 0xEF}
	for i, want := range wantBytes {
		if got := cpu.memory[0x80000+uint32(i)]; got != want {
			t.Fatalf("RAM byte[%d]=0x%02X, want 0x%02X; bytes=% X", i, got, want, cpu.memory[0x80000:0x80006])
		}
	}
	if got := cpu.regs[1]; got != 0x1234 {
		t.Fatalf("d0/r1=0x%X, want 0x1234", got)
	}
	if got := cpu.regs[2]; got != 0x89ABCDEF {
		t.Fatalf("d1/r2=0x%X, want 0x89ABCDEF", got)
	}
}

func TestM68KTo64_BEBridge_MMIORegisterIndirectStaysNative(t *testing.T) {
	src := `
VIDEO_PAL_INDEX equ $F0078
VIDEO_PAL_DATA  equ $F007C
		lea VIDEO_PAL_INDEX,a0
		move.l #255,d0
		move.l d0,(a0)
		lea VIDEO_PAL_DATA,a0
		move.l #$00112233,d0
		move.l d0,(a0)
	`
	bin := transpileAndAssembleWithArgs(t, src, "-mmio-range", "0xF0000-0xF0FFF")
	video, bus := newCLUT8TestRig(t)
	runToHaltOnBus(t, bus, bin, 2000)

	if got := video.HandleRead(VIDEO_PAL_INDEX); got != 0 {
		t.Fatalf("VIDEO_PAL_INDEX after one data write = 0x%X, want wrapped 0", got)
	}
	if got := video.HandleRead(VIDEO_PAL_TABLE + 255*4); got != 0x00112233 {
		t.Fatalf("palette[255]=0x%08X, want 0x00112233", got)
	}
}

func TestM68KTo64_BEBridge_FileIOMMIORangeStaysNative(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "clip.bin"), []byte{0xFF, 0xFF, 0xFF, 0xFE}, 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	src := `
FILE_IO_NAME   equ $F2200
FILE_IO_DATA   equ $F2204
FILE_IO_CTRL   equ $F220C
FILE_IO_STATUS equ $F2210
FILE_IO_LEN    equ $F2214
		move.l #filename,FILE_IO_NAME
		move.l #$80000,FILE_IO_DATA
		move.l #1,FILE_IO_CTRL
		move.l FILE_IO_STATUS,d0
		move.l FILE_IO_LEN,d1
		bra done
filename:
		dc.b 'clip.bin',0
		even
done:
	`
	bin := transpileAndAssembleWithArgs(t, src, "-mmio-range", "0xF0000-0xF2FFF")
	bus := NewMachineBus()
	fileIO := NewFileIODevice(bus, dir)
	bus.MapIO(FILE_IO_BASE, FILE_IO_BASE+0x1F, fileIO.HandleRead, fileIO.HandleWrite)
	runToHaltOnBus(t, bus, bin, 2000)

	if got := fileIO.HandleRead(FILE_STATUS); got != 0 {
		t.Fatalf("FILE_IO_STATUS=0x%X, want 0", got)
	}
	if got := fileIO.HandleRead(FILE_RESULT_LEN); got != 4 {
		t.Fatalf("FILE_IO_LEN=0x%X, want 4", got)
	}
	if got := []byte{bus.Read8(0x80000), bus.Read8(0x80001), bus.Read8(0x80002), bus.Read8(0x80003)}; string(got) != string([]byte{0xFF, 0xFF, 0xFF, 0xFE}) {
		t.Fatalf("loaded bytes=% X, want FF FF FF FE", got)
	}
}

func TestM68KTo64_BytePostincrementStringCopyFromProgramData(t *testing.T) {
	src := `
		lea src,a0
		lea $80000,a1
.loop:
		move.b (a0)+,d0
		beq.s .done
		move.b d0,(a1)+
		bra.s .loop
.done:
		bra.s after
src:
		dc.b '_build/ie_media/redux-high/',0
		even
after:
	`
	bin := transpileAndAssembleWithArgs(t, src)
	bus := NewMachineBus()
	runToHaltOnBus(t, bus, bin, 5000)

	want := []byte("_build/ie_media/redux-high/")
	got := make([]byte, len(want))
	for i := range got {
		got[i] = bus.Read8(0x80000 + uint32(i))
	}
	if string(got) != string(want) {
		t.Fatalf("copied string %q (% X), want %q", got, got, want)
	}
}

func TestM68KTo64_PostIncAddressRegisterStorePreservesPointerTable(t *testing.T) {
	src := `
		lea ptrs,a3
		lea starts,a4
		lea out,a1
		moveq #0,d7
outer:
		lea ptrs,a3
		lea starts,a4
loop:
		move.l (a3),d0
		blt done_loop
		move.l d0,a2
		move.w (a2)+,d0
		cmp.w #999,d0
		bne.s bright
		move.l (a4),a2
		move.w (a2)+,d0
bright:
		move.l a2,(a3)+
		addq #4,a4
		move.w d0,(a1)+
		bra.s loop
done_loop:
		addq #1,d7
		cmp.w #2,d7
		blt outer
		bra after
ptrs:
		dc.l seq1,seq2,-1
starts:
		dc.l seq1,seq2
seq1:
		dc.w 1,2,999
seq2:
		dc.w 3,4,999
out:
		ds.w 8
after:
	`
	bin := transpileAndAssembleWithArgs(t, src)
	bus := NewMachineBus()
	cpu := runToHaltOnBus(t, bus, bin, 5000)

	outAddr := uint32(cpu.regs[10]) - 8 // a1 after four word writes.
	readBE16 := func(addr uint32) uint16 {
		return uint16(cpu.memory[addr])<<8 | uint16(cpu.memory[addr+1])
	}
	readBE32 := func(addr uint32) uint32 {
		return uint32(cpu.memory[addr])<<24 | uint32(cpu.memory[addr+1])<<16 | uint32(cpu.memory[addr+2])<<8 | uint32(cpu.memory[addr+3])
	}
	got := []uint16{
		readBE16(outAddr),
		readBE16(outAddr + 2),
		readBE16(outAddr + 4),
		readBE16(outAddr + 6),
	}
	want := []uint16{1, 3, 2, 4}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("out[%d]=%d, want %d; all=%v", i, got[i], want[i], got)
		}
	}

	ptrsAddr := outAddr - 3*2 - 3*2 - 2*4 - 3*4 // back over seq data, starts, ptrs.
	if got := readBE32(ptrsAddr + 8); got != 0xFFFFFFFF {
		t.Fatalf("ptr terminator=0x%08X, want FFFFFFFF", got)
	}
}

func TestM68KTo64_PostIncAddressRegisterLoadRestoresStackPointerValue(t *testing.T) {
	src := `
		lea menu,a0
		move.l a0,-(a7)
		bsr clobber
		move.l (a7)+,a0
		move.l a0,out
		bra after
clobber:
		lea $90000,a0
		lea $91000,a1
		moveq #0,d0
		moveq #0,d1
		rts
menu:
		dc.w 0,0
		dc.l title
		dc.w 0,40
		dc.w 20
		dc.w 9
title:
		dc.b 'PLAY GAME',0
		even
out:
		dc.l 0
after:
	`
	bin := transpileAndAssembleWithArgs(t, src)
	bus := NewMachineBus()
	cpu := runToHaltOnBus(t, bus, bin, 5000)

	outAddr := uint32(cpu.regs[9]) + 26 // a0 restored to menu; out follows the 16-byte menu header and 10-byte title.
	readBE32 := func(addr uint32) uint32 {
		return uint32(cpu.memory[addr])<<24 | uint32(cpu.memory[addr+1])<<16 | uint32(cpu.memory[addr+2])<<8 | uint32(cpu.memory[addr+3])
	}
	got := readBE32(outAddr)
	want := uint32(cpu.regs[9])
	if got != want {
		t.Fatalf("restored stack pointer value stored 0x%08X, want menu pointer 0x%08X (a0)", got, want)
	}
}

func TestM68KTo64_MenuOpenStackSaveSurvivesClearLoop(t *testing.T) {
	src := `
		lea menu,a0
		move.l a0,-(a7)
		move.l #0,d1
		bsr cls
		move.l (a7)+,a0
		move.l a0,saved_menu
		move.l 4(a0),a0
		move.l a0,saved_title
		bra after
cls:
		lea $145E000,a1
		moveq #0,d1
		move.w #7,d0
.loop:
		move.l d1,-(a1)
		move.l d1,-(a1)
		move.l d1,-(a1)
		move.l d1,-(a1)
		dbra d0,.loop
		rts
menu:
		dc.w 0,0
		dc.l title
		dc.w 0,40
		dc.w 20
		dc.w 9
title:
		dc.b 'PLAY GAME',0
		even
saved_menu:
		dc.l 0
saved_title:
		dc.l 0
after:
	`
	bin := transpileAndAssembleWithArgs(t, src)
	bus := NewMachineBus()
	cpu := runToHaltOnBus(t, bus, bin, 10000)

	menuAddr := uint32(cpu.regs[9]) - 16 // a0 ends at title; title follows the 16-byte menu header.
	savedMenuAddr := menuAddr + 26
	readBE32 := func(addr uint32) uint32 {
		return uint32(cpu.memory[addr])<<24 | uint32(cpu.memory[addr+1])<<16 | uint32(cpu.memory[addr+2])<<8 | uint32(cpu.memory[addr+3])
	}
	if got := readBE32(savedMenuAddr); got != menuAddr {
		t.Fatalf("saved_menu=0x%08X, want menu 0x%08X", got, menuAddr)
	}
	if got, want := readBE32(savedMenuAddr+4), menuAddr+16; got != want {
		t.Fatalf("saved_title=0x%08X, want title 0x%08X", got, want)
	}
}
