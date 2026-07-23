// video_frame_lease.go - immutable frame lease primitives for compositor paths.
//
// (c) 2024-2026 Zayn Otley - GPLv3 or later

package main

import (
	"encoding/binary"
	"sync"
	"unsafe"
)

type VideoFrameLeaseRing struct {
	mu     sync.Mutex
	slots  [][]byte
	inUse  []bool
	next   int
	size   int
	closed bool
}

type VideoFrameLease struct {
	ring     *VideoFrameLeaseRing
	slot     int
	pixels   []byte
	refs     int
	released bool
}

func NewVideoFrameLeaseRing(depth int, frameBytes int) *VideoFrameLeaseRing {
	if depth < 1 {
		depth = 1
	}
	if frameBytes < 0 {
		frameBytes = 0
	}
	r := &VideoFrameLeaseRing{
		slots: make([][]byte, depth),
		inUse: make([]bool, depth),
		size:  frameBytes,
	}
	for i := range r.slots {
		r.slots[i] = make([]byte, frameBytes)
	}
	return r
}

func (r *VideoFrameLeaseRing) Acquire() (*VideoFrameLease, bool) {
	if r == nil {
		return nil, false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil, false
	}
	for step := range len(r.slots) {
		idx := (r.next + step) % len(r.slots)
		if r.inUse[idx] {
			continue
		}
		r.inUse[idx] = true
		r.next = (idx + 1) % len(r.slots)
		return &VideoFrameLease{
			ring:   r,
			slot:   idx,
			pixels: r.slots[idx],
			refs:   1,
		}, true
	}
	return nil, false
}

func (l *VideoFrameLease) Pixels() []byte {
	if l == nil {
		return nil
	}
	return l.pixels
}

// Slot returns the ring slot backing this lease, or -1 for a nil lease. Slot
// pixels persist until the slot is handed out again, which is what lets the
// compositor retain composed tiles across frames.
func (l *VideoFrameLease) Slot() int {
	if l == nil {
		return -1
	}
	return l.slot
}

func (l *VideoFrameLease) Snapshot() []byte {
	if l == nil {
		return nil
	}
	out := make([]byte, len(l.pixels))
	copy(out, l.pixels)
	return out
}

func (l *VideoFrameLease) NormaliseAlpha() {
	if l == nil {
		return
	}
	normaliseFrameLeaseAlphaRGBA(l.pixels)
}

func (l *VideoFrameLease) Retain() bool {
	if l == nil || l.ring == nil {
		return false
	}
	r := l.ring
	r.mu.Lock()
	defer r.mu.Unlock()
	if l.released {
		return false
	}
	l.refs++
	return true
}

func (l *VideoFrameLease) Release() {
	if l == nil || l.ring == nil {
		return
	}
	r := l.ring
	r.mu.Lock()
	defer r.mu.Unlock()
	if l.released {
		return
	}
	if l.refs > 1 {
		l.refs--
		return
	}
	l.released = true
	l.refs = 0
	if l.slot >= 0 && l.slot < len(r.inUse) {
		r.inUse[l.slot] = false
	}
}

func normaliseFrameLeaseAlphaRGBA(pixels []byte) {
	normaliseFrameLeaseSpanImpl(pixels)
}

// normaliseFrameLeaseSpanImpl defaults to the scalar leaf and is reassigned to
// the SIMD variant in assignSIMDKernels on supported hosts. Differential tests
// call normaliseFrameLeaseSpanScalar directly.
var normaliseFrameLeaseSpanImpl = normaliseFrameLeaseSpanScalar

// normaliseFrameLeaseSpanScalar promotes zero-alpha nonzero-rgb pixels in place
// to 0xFFRRGGBB, writing only pixels that change. Fully-zero and already
// alpha-set pixels are left untouched.
func normaliseFrameLeaseSpanScalar(pixels []byte) {
	i := 0
	n := len(pixels)
	// Two pixels per iteration. A pixel is promoted only when it is nonzero
	// with a zero alpha byte; alpha-set pixels (any alpha value) and fully
	// zero pixels pass through untouched, matching compositorOpaquePixel.
	for ; i+8 <= n; i += 8 {
		v := *(*uint64)(unsafe.Pointer(&pixels[i]))
		lo := uint32(v)
		hi := uint32(v >> 32)
		if lo != 0 && lo&0xFF000000 == 0 {
			lo |= 0xFF000000
		}
		if hi != 0 && hi&0xFF000000 == 0 {
			hi |= 0xFF000000
		}
		if nv := uint64(hi)<<32 | uint64(lo); nv != v {
			*(*uint64)(unsafe.Pointer(&pixels[i])) = nv
		}
	}
	for ; i+BYTES_PER_PIXEL <= n; i += BYTES_PER_PIXEL {
		src := binary.LittleEndian.Uint32(pixels[i:])
		if out, ok := compositorOpaquePixel(src); ok && out != src {
			binary.LittleEndian.PutUint32(pixels[i:], out)
		}
	}
}
