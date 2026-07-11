//go:build headless

package main

import "testing"

// Per-swap hashing (IE_SWAP_HASH=1) must record one hash per presented
// swap, keyed by sequence number, deterministic with respect to the
// guest's swap stream: hash N is the Nth SWAP_BUFFER_CMD's frame no
// matter when a consumer looks. Identical frames hash identically;
// different frames differ; evicted or unknown sequences read as 0.
func TestVoodooSwapHash_PerSwapDeterministic(t *testing.T) {
	saved := voodooSwapHashOn
	voodooSwapHashOn = true
	defer func() { voodooSwapHashOn = saved }()

	_, v := newMappedTestVoodoo(t)
	testVoodooSoftwareBackend(t, v)

	v.HandleWrite(VOODOO_ENABLE, 1)
	v.HandleWrite(VOODOO_FBZ_MODE, VOODOO_FBZ_RGB_WRITE)

	fillAndSwap := func(color uint32) {
		v.HandleWrite(VOODOO_COLOR0, color)
		v.HandleWrite(VOODOO_FAST_FILL_CMD, 0)
		v.HandleWrite(VOODOO_SWAP_BUFFER_CMD, 0)
	}
	fillAndSwap(0xFF0000FF) // swap 1: red
	fillAndSwap(0xFF0000FF) // swap 2: red again
	fillAndSwap(0xFFFF0000) // swap 3: blue
	v.WaitSwapIdle()

	if got := v.HandleRead(VOODOO_SWAP_HASH_SEQ); got != 3 {
		t.Fatalf("SWAP_HASH_SEQ = %d, want 3", got)
	}
	hash := func(seq uint32) uint32 {
		v.HandleWrite(VOODOO_SWAP_HASH_QUERY, seq)
		return v.HandleRead(VOODOO_SWAP_HASH_VALUE)
	}
	h1, h2, h3 := hash(1), hash(2), hash(3)
	if h1 == 0 || h2 == 0 || h3 == 0 {
		t.Fatalf("recorded hashes must be non-zero, got %#x %#x %#x", h1, h2, h3)
	}
	if h1 != h2 {
		t.Fatalf("identical frames must hash identically: %#x vs %#x", h1, h2)
	}
	if h3 == h1 {
		t.Fatalf("different frames must hash differently: both %#x", h3)
	}
	if got := hash(99); got != 0 {
		t.Fatalf("unknown sequence must read 0, got %#x", got)
	}
}
