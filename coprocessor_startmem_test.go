package main

import "testing"

// COPROC_CMD_START_MEM takes a guest-RAM blob via REQ_PTR/REQ_LEN. A
// pointer near the top of the 32-bit range with a small length wraps in
// uint32 arithmetic; the bounds check and the slice must both use the
// 64-bit end or the command panics instead of erroring. Pin both the
// wrap case and the straightforward out-of-range case.
func TestCoprocStartMem_WrappedRangeErrorsCleanly(t *testing.T) {
	bus, mgr := newTestBusAndManager(t)
	defer mgr.StopAll()

	write := func(reg, val uint32) { bus.Write32(reg, val) }

	for _, tc := range []struct {
		name string
		ptr  uint32
		len  uint32
	}{
		{"wrapped-end", 0xFFFFFF00, 0x200},
		{"past-ram", 0x7FFFFFFF, 0x1000},
		{"zero-length", 0x1000, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("CMD_START_MEM panicked for ptr=%#x len=%#x: %v", tc.ptr, tc.len, r)
				}
			}()
			write(COPROC_CPU_TYPE, EXEC_TYPE_IE64)
			write(COPROC_REQ_PTR, tc.ptr)
			write(COPROC_REQ_LEN, tc.len)
			write(COPROC_CMD, COPROC_CMD_START_MEM)
			if status := bus.Read32(COPROC_CMD_STATUS); status == COPROC_STATUS_OK {
				t.Fatalf("CMD_START_MEM accepted invalid blob ptr=%#x len=%#x", tc.ptr, tc.len)
			}
		})
	}
}
