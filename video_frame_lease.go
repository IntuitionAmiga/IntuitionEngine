// video_frame_lease.go - immutable frame lease primitives for compositor paths.
//
// (c) 2024-2026 Zayn Otley - GPLv3 or later

package main

import (
	"encoding/binary"
	"sync"
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
	for i := 0; i+BYTES_PER_PIXEL <= len(pixels); i += BYTES_PER_PIXEL {
		src := binary.LittleEndian.Uint32(pixels[i:])
		if out, ok := compositorOpaquePixel(src); ok && out != src {
			binary.LittleEndian.PutUint32(pixels[i:], out)
		}
	}
}
