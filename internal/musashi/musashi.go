//go:build musashi

// Package musashi provides a CGO bridge to the Musashi 68020 CPU emulator
// for use as a test oracle.
package musashi

// #cgo CFLAGS: -I../../third_party/musashi -I../../third_party/musashi/softfloat -O2 -w
// #cgo LDFLAGS: -lm
// #include "../../third_party/musashi/m68k_musashi_bridge.c"
import "C"

// Register constants (from m68k.h m68k_register_t enum)
const (
	RegD0 = iota
	RegD1
	RegD2
	RegD3
	RegD4
	RegD5
	RegD6
	RegD7
	RegA0
	RegA1
	RegA2
	RegA3
	RegA4
	RegA5
	RegA6
	RegA7
	RegPC
	RegSR
	RegSP
	RegUSP
	RegISP
	RegMSP
)

// CPU wraps the Musashi 68020 core with a static 16MB memory array.
type CPU struct{}

func New() *CPU { return &CPU{} }

func (c *CPU) Init()     { C.musashi_init() }
func (c *CPU) Reset()    { C.musashi_reset() }
func (c *CPU) ClearMem() { C.musashi_clear_mem() }

func (c *CPU) SetReg(reg int, val uint32) {
	C.musashi_set_reg(C.int(reg), C.uint(val))
}

func (c *CPU) GetReg(reg int) uint32 {
	return uint32(C.musashi_get_reg(C.int(reg)))
}

func (c *CPU) Execute(cycles int) int {
	return int(C.musashi_execute(C.int(cycles)))
}

func (c *CPU) WriteByte(addr uint32, val byte) {
	C.musashi_write_byte(C.uint(addr), C.uchar(val))
}

func (c *CPU) Read8(addr uint32) byte {
	return byte(C.musashi_read_byte(C.uint(addr)))
}

func (c *CPU) Read32(addr uint32) uint32 {
	return uint32(C.musashi_read_32(C.uint(addr)))
}
