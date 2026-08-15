//go:build amd64 && goexperiment.simd

package main

import (
	"simd/archsimd"
	"unsafe"
)

// fillUint32LESpanSIMD broadcasts v across dst in 8-word vectors. On amd64 a
// uint32 lane stores little-endian, matching binary.LittleEndian.PutUint32, so
// the bytes are identical to the scalar leaf. Sub-8-word remainder defers to the
// scalar leaf.
func fillUint32LESpanSIMD(dst []byte, v uint32) {
	words := len(dst) / 4
	full := words &^ (simdPixelLanes - 1)
	if full > 0 {
		du := unsafe.Slice((*uint32)(unsafe.Pointer(&dst[0])), words)
		bc := archsimd.BroadcastUint32x8(v)
		for i := 0; i < full; i += simdPixelLanes {
			bc.Store(du[i : i+simdPixelLanes])
		}
	}
	if tail := full * 4; tail < len(dst) {
		fillUint32LESpanScalar(dst[tail:], v)
	}
}

// Colour-expand row: no SIMD variant. The scalar fast path already removes the
// per-pixel bus dispatch that dominated the generic loop. A vector variant would
// still gather MSB-first bit-packed template bits lane by lane (Go 1.27
// archsimd has no gather or bit-to-lane expand), so the mask unpack, not the
// fg/bg select, bounds the kernel; a vector store cannot clear the 10% stop
// rule. colorExpand
// RowImpl therefore stays scalar. See simd_evidence notes.
