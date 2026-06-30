//go:build amd64 && (linux || windows || darwin)

package main

import (
	"bytes"
	"testing"
)

// Slice 2 of native M68K FPU JIT: SSE2 scalar-double encoders. The 68881 FP
// register file is float64, and Go's float64 arithmetic lowers to these exact
// SSE2 scalar-double instructions, so emitting them is bit-identical to the
// interpreter. Expected byte sequences are hand-encoded against the Intel SDM
// (F2 0F xx /r, with REX/ModRM/SIB/disp as needed).

func emitToBytes(emit func(cb *CodeBuffer)) []byte {
	cb := NewCodeBuffer(64)
	emit(cb)
	return cb.Bytes()
}

func TestAMD64_SSEsd_RegReg(t *testing.T) {
	cases := []struct {
		name string
		emit func(cb *CodeBuffer)
		want []byte
	}{
		{"addsd xmm0,xmm1", func(cb *CodeBuffer) { amd64ADDSD_rr(cb, 0, 1) }, []byte{0xF2, 0x0F, 0x58, 0xC1}},
		{"subsd xmm0,xmm1", func(cb *CodeBuffer) { amd64SUBSD_rr(cb, 0, 1) }, []byte{0xF2, 0x0F, 0x5C, 0xC1}},
		{"mulsd xmm0,xmm1", func(cb *CodeBuffer) { amd64MULSD_rr(cb, 0, 1) }, []byte{0xF2, 0x0F, 0x59, 0xC1}},
		{"divsd xmm0,xmm1", func(cb *CodeBuffer) { amd64DIVSD_rr(cb, 0, 1) }, []byte{0xF2, 0x0F, 0x5E, 0xC1}},
		{"sqrtsd xmm0,xmm1", func(cb *CodeBuffer) { amd64SQRTSD_rr(cb, 0, 1) }, []byte{0xF2, 0x0F, 0x51, 0xC1}},
		{"movsd xmm0,xmm1", func(cb *CodeBuffer) { amd64MOVSD_rr(cb, 0, 1) }, []byte{0xF2, 0x0F, 0x10, 0xC1}},
		// Extended register requires REX.B (xmm10 as source).
		{"addsd xmm2,xmm10", func(cb *CodeBuffer) { amd64ADDSD_rr(cb, 2, 10) }, []byte{0xF2, 0x41, 0x0F, 0x58, 0xD2}},
		// Extended dest requires REX.R (xmm9 as dest).
		{"mulsd xmm9,xmm1", func(cb *CodeBuffer) { amd64MULSD_rr(cb, 9, 1) }, []byte{0xF2, 0x44, 0x0F, 0x59, 0xC9}},
	}
	for _, c := range cases {
		if got := emitToBytes(c.emit); !bytes.Equal(got, c.want) {
			t.Errorf("%s: got % X, want % X", c.name, got, c.want)
		}
	}
}

func TestAMD64_SSEsd_Memory(t *testing.T) {
	cases := []struct {
		name string
		emit func(cb *CodeBuffer)
		want []byte
	}{
		// movsd xmm0, [rax]   — base RAX, disp 0, no SIB
		{"movsd xmm0,[rax]", func(cb *CodeBuffer) { amd64MOVSD_load(cb, 0, amd64RAX, 0) }, []byte{0xF2, 0x0F, 0x10, 0x00}},
		// movsd xmm1, [rax+8] — disp8
		{"movsd xmm1,[rax+8]", func(cb *CodeBuffer) { amd64MOVSD_load(cb, 1, amd64RAX, 8) }, []byte{0xF2, 0x0F, 0x10, 0x48, 0x08}},
		// movsd [rcx+0x10], xmm2 — store form (0x11)
		{"movsd [rcx+0x10],xmm2", func(cb *CodeBuffer) { amd64MOVSD_store(cb, amd64RCX, 0x10, 2) }, []byte{0xF2, 0x0F, 0x11, 0x51, 0x10}},
		// addsd xmm0, [rcx+8]
		{"addsd xmm0,[rcx+8]", func(cb *CodeBuffer) { amd64ADDSD_rm(cb, 0, amd64RCX, 8) }, []byte{0xF2, 0x0F, 0x58, 0x41, 0x08}},
		// RBP base with disp 0 must still encode a disp8 (mod=01).
		{"movsd xmm0,[rbp]", func(cb *CodeBuffer) { amd64MOVSD_load(cb, 0, amd64RBP, 0) }, []byte{0xF2, 0x0F, 0x10, 0x45, 0x00}},
		// R12 base requires SIB and REX.B.
		{"movsd xmm0,[r12]", func(cb *CodeBuffer) { amd64MOVSD_load(cb, 0, amd64R12, 0) }, []byte{0xF2, 0x41, 0x0F, 0x10, 0x04, 0x24}},
		// Large displacement → disp32.
		{"movsd xmm0,[rax+0x200]", func(cb *CodeBuffer) { amd64MOVSD_load(cb, 0, amd64RAX, 0x200) }, []byte{0xF2, 0x0F, 0x10, 0x80, 0x00, 0x02, 0x00, 0x00}},
	}
	for _, c := range cases {
		if got := emitToBytes(c.emit); !bytes.Equal(got, c.want) {
			t.Errorf("%s: got % X, want % X", c.name, got, c.want)
		}
	}
}

func TestAMD64_SSEcvt_RegReg(t *testing.T) {
	cases := []struct {
		name string
		emit func(cb *CodeBuffer)
		want []byte
	}{
		// cvtsd2ss (double->single): F2 0F 5A /r
		{"cvtsd2ss xmm0,xmm0", func(cb *CodeBuffer) { amd64CVTSD2SS_rr(cb, 0, 0) }, []byte{0xF2, 0x0F, 0x5A, 0xC0}},
		{"cvtsd2ss xmm1,xmm2", func(cb *CodeBuffer) { amd64CVTSD2SS_rr(cb, 1, 2) }, []byte{0xF2, 0x0F, 0x5A, 0xCA}},
		// cvtss2sd (single->double): F3 0F 5A /r
		{"cvtss2sd xmm0,xmm0", func(cb *CodeBuffer) { amd64CVTSS2SD_rr(cb, 0, 0) }, []byte{0xF3, 0x0F, 0x5A, 0xC0}},
	}
	for _, c := range cases {
		if got := emitToBytes(c.emit); !bytes.Equal(got, c.want) {
			t.Errorf("%s: got % X, want % X", c.name, got, c.want)
		}
	}
}

func TestAMD64_CCPrimitives(t *testing.T) {
	cases := []struct {
		name string
		emit func(cb *CodeBuffer)
		want []byte
	}{
		{"movq rax,xmm0", func(cb *CodeBuffer) { amd64MOVQ_reg_xmm(cb, amd64RAX, 0) }, []byte{0x66, 0x48, 0x0F, 0x7E, 0xC0}},
		{"movq rcx,xmm0", func(cb *CodeBuffer) { amd64MOVQ_reg_xmm(cb, amd64RCX, 0) }, []byte{0x66, 0x48, 0x0F, 0x7E, 0xC1}},
		{"and edx,0xF0FFFFFF", func(cb *CodeBuffer) { amd64ALU_reg_imm32_32bit(cb, 4, amd64RDX, -251658241) }, []byte{0x81, 0xE2, 0xFF, 0xFF, 0xFF, 0xF0}},
		{"and r10d,0x7FF", func(cb *CodeBuffer) { amd64ALU_reg_imm32_32bit(cb, 4, amd64R10, 0x7FF) }, []byte{0x41, 0x81, 0xE2, 0xFF, 0x07, 0x00, 0x00}},
		{"cmp r10d,0x7FF", func(cb *CodeBuffer) { amd64ALU_reg_imm32_32bit(cb, 7, amd64R10, 0x7FF) }, []byte{0x41, 0x81, 0xFA, 0xFF, 0x07, 0x00, 0x00}},
		{"or r11d,edx", func(cb *CodeBuffer) { amd64OR_reg_reg32(cb, amd64R11, amd64RDX) }, []byte{0x41, 0x09, 0xD3}},
		{"or eax,ecx", func(cb *CodeBuffer) { amd64OR_reg_reg32(cb, amd64RAX, amd64RCX) }, []byte{0x09, 0xC8}},
	}
	for _, c := range cases {
		if got := emitToBytes(c.emit); !bytes.Equal(got, c.want) {
			t.Errorf("%s: got % X, want % X", c.name, got, c.want)
		}
	}
}

func TestAMD64_FABSFNEGPrimitives(t *testing.T) {
	cases := []struct {
		name string
		emit func(cb *CodeBuffer)
		want []byte
	}{
		{"movq xmm1,rcx", func(cb *CodeBuffer) { amd64MOVQ_xmm_reg(cb, 1, amd64RCX) }, []byte{0x66, 0x48, 0x0F, 0x6E, 0xC9}},
		{"andpd xmm0,xmm1", func(cb *CodeBuffer) { amd64ANDPD_rr(cb, 0, 1) }, []byte{0x66, 0x0F, 0x54, 0xC1}},
		{"xorpd xmm0,xmm1", func(cb *CodeBuffer) { amd64XORPD_rr(cb, 0, 1) }, []byte{0x66, 0x0F, 0x57, 0xC1}},
	}
	for _, c := range cases {
		if got := emitToBytes(c.emit); !bytes.Equal(got, c.want) {
			t.Errorf("%s: got % X, want % X", c.name, got, c.want)
		}
	}
}

func TestAMD64_UCOMISD(t *testing.T) {
	// ucomisd xmm0, [rax+0x10]  →  66 0F 2E 40 10
	got := emitToBytes(func(cb *CodeBuffer) { amd64UCOMISD_rm(cb, 0, amd64RAX, 0x10) })
	want := []byte{0x66, 0x0F, 0x2E, 0x40, 0x10}
	if !bytes.Equal(got, want) {
		t.Errorf("ucomisd: got % X, want % X", got, want)
	}
}
