package main

// Multi-instance coprocessor workers: the JIT-capable types (M68K, x86, IE64)
// run two worker instances selected via COPROC_INSTANCE, each with its own ring
// (ring index = cpuTypeIndex*2 + instance) and computed RAM window
// (workerWindow). IE32, 6502, and Z80 run a single instance.

import (
	"testing"
	"time"
)

// m68kSpinLoop is BRA.S -2 stored big-endian (see TestCoprocWorkerM68KStartStop).
var m68kSpinLoop = []byte{0x60, 0xFE}

func TestCoprocRingIndex_InstanceMapping(t *testing.T) {
	cases := []struct {
		cpuType  uint32
		instance uint32
		want     int
	}{
		// cpuTypeToIndex: IE32=0, 6502=1, M68K=2, Z80=3, x86=4, IE64=5.
		{EXEC_TYPE_IE32, 0, 0},
		{EXEC_TYPE_IE32, 1, -1}, // single instance
		{EXEC_TYPE_6502, 0, 2},
		{EXEC_TYPE_6502, 1, -1},
		{EXEC_TYPE_M68K, 0, 4},
		{EXEC_TYPE_M68K, 1, 5},
		{EXEC_TYPE_M68K, 2, -1},
		{EXEC_TYPE_Z80, 0, 6},
		{EXEC_TYPE_Z80, 1, -1},
		{EXEC_TYPE_X86, 0, 8},
		{EXEC_TYPE_X86, 1, 9},
		{EXEC_TYPE_IE64, 0, 10},
		{EXEC_TYPE_IE64, 1, 11},
		{EXEC_TYPE_NONE, 0, -1},
	}
	for _, c := range cases {
		if got := coprocRingIndex(c.cpuType, c.instance); got != c.want {
			t.Errorf("coprocRingIndex(%d, %d) = %d, want %d", c.cpuType, c.instance, got, c.want)
		}
	}
}

// TestRingBase_UniformRule pins the single ring-address rule: every ring sits at
// MAILBOX_BASE + i*RING_STRIDE with no special case.
func TestRingBase_UniformRule(t *testing.T) {
	for i := 0; i < COPROC_RING_COUNT; i++ {
		want := uint32(MAILBOX_BASE) + uint32(i)*RING_STRIDE
		if got := ringBaseAddr(i); got != want {
			t.Errorf("ringBaseAddr(%d) = %#x, want %#x", i, got, want)
		}
	}
}

// TestRingGeometry_NoOverlap asserts no ring's slot-15 response overflows into
// the next ring, and the final ring stays inside the mailbox window.
func TestRingGeometry_NoOverlap(t *testing.T) {
	const content = RING_RESPONSES_OFFSET + RING_CAPACITY*RESP_DESC_SIZE
	for i := 0; i < COPROC_RING_COUNT-1; i++ {
		if ringBaseAddr(i+1) < ringBaseAddr(i)+content {
			t.Errorf("ring %d content overflows into ring %d", i, i+1)
		}
	}
	last := ringBaseAddr(COPROC_RING_COUNT-1) + content
	if last > MAILBOX_END+1 {
		t.Errorf("final ring content end %#x exceeds mailbox end %#x", last, MAILBOX_END+1)
	}
}

// TestCoprocErr_InvalidInstanceDistinct pins that a valid CPU type with an
// out-of-range instance reports COPROC_ERR_INVALID_INSTANCE, distinct from the
// COPROC_ERR_INVALID_CPU reported for an unknown CPU type.
func TestCoprocErr_InvalidInstanceDistinct(t *testing.T) {
	if COPROC_ERR_INVALID_INSTANCE == COPROC_ERR_INVALID_CPU {
		t.Fatal("INVALID_INSTANCE must be a distinct error code from INVALID_CPU")
	}
	if e := coprocSelectionError(EXEC_TYPE_M68K, 2); e != COPROC_ERR_INVALID_INSTANCE {
		t.Errorf("(M68K,2) = %d, want INVALID_INSTANCE", e)
	}
	if e := coprocSelectionError(EXEC_TYPE_IE32, 1); e != COPROC_ERR_INVALID_INSTANCE {
		t.Errorf("(IE32,1) = %d, want INVALID_INSTANCE", e)
	}
	if e := coprocSelectionError(EXEC_TYPE_NONE, 0); e != COPROC_ERR_INVALID_CPU {
		t.Errorf("(NONE,0) = %d, want INVALID_CPU", e)
	}
	if e := coprocSelectionError(EXEC_TYPE_M68K, 0); e != COPROC_ERR_NONE {
		t.Errorf("(M68K,0) = %d, want NONE", e)
	}
}

// TestCoprocWorkerBus_MailboxReachable asserts the 6502/Z80 worker bus window
// (mailbox mapped at CPU $2000) reaches the highest ring's last content byte
// and stays inside the 16-bit CPU address space.
func TestCoprocWorkerBus_MailboxReachable(t *testing.T) {
	const mailboxStart = 0x2000
	mailboxEnd := mailboxStart + MAILBOX_SIZE // $5000
	const content = RING_RESPONSES_OFFSET + RING_CAPACITY*RESP_DESC_SIZE
	highestRing := COPROC_RING_COUNT - 1
	lastByte := mailboxStart + highestRing*RING_STRIDE + content - 1
	if lastByte >= mailboxEnd {
		t.Errorf("highest ring last byte %#x not within mailbox window [%#x,%#x)",
			lastByte, mailboxStart, mailboxEnd)
	}
	if mailboxEnd > 0x10000 {
		t.Errorf("mailbox window end %#x exceeds the 16-bit CPU address space", mailboxEnd)
	}
	// The shipped worker bus adapters map exactly this window.
	cb := &CoprocBus32{mailboxStart: mailboxStart, mailboxEnd: uint16(mailboxEnd)}
	zb := &CoprocZ80Bus{mailboxStart: mailboxStart, mailboxEnd: uint16(mailboxEnd)}
	if int(cb.mailboxEnd) != mailboxEnd || int(zb.mailboxEnd) != mailboxEnd {
		t.Errorf("adapter mailbox window mismatch: cb=%#x zb=%#x want %#x",
			cb.mailboxEnd, zb.mailboxEnd, mailboxEnd)
	}
}

func TestCoprocInstanceLimit(t *testing.T) {
	for _, ct := range []uint32{EXEC_TYPE_M68K, EXEC_TYPE_X86, EXEC_TYPE_IE64} {
		if got := coprocInstanceLimit(ct); got != 2 {
			t.Errorf("type %d instance limit = %d, want 2", ct, got)
		}
	}
	for _, ct := range []uint32{EXEC_TYPE_IE32, EXEC_TYPE_6502, EXEC_TYPE_Z80} {
		if got := coprocInstanceLimit(ct); got != 1 {
			t.Errorf("type %d instance limit = %d, want 1", ct, got)
		}
	}
	if got := coprocInstanceLimit(EXEC_TYPE_NONE); got != 0 {
		t.Errorf("invalid type instance limit = %d, want 0", got)
	}
}

// TestCoprocReset_SelectorsReturnToInstanceZero pins that a reset clears the
// instance selector, so BASIC's COINSTANCE() and raw MMIO reads return to
// instance 0 after a machine reset.
func TestCoprocReset_SelectorsReturnToInstanceZero(t *testing.T) {
	_, mgr := newTestBusAndManager(t)
	defer mgr.StopAll()

	mgr.HandleWrite(COPROC_CPU_TYPE, EXEC_TYPE_M68K)
	mgr.HandleWrite(COPROC_INSTANCE, 1)
	if got := mgr.HandleRead(COPROC_INSTANCE); got != 1 {
		t.Fatalf("instance selector = %d, want 1 before reset", got)
	}

	mgr.Reset()

	if got := mgr.HandleRead(COPROC_INSTANCE); got != 0 {
		t.Errorf("instance selector = %d after reset, want 0", got)
	}
}

func TestCoprocInstanceLabel(t *testing.T) {
	if got := coprocInstanceLabel(EXEC_TYPE_M68K, 0); got != "coproc:M68K" {
		t.Errorf("instance 0 label = %q", got)
	}
	if got := coprocInstanceLabel(EXEC_TYPE_M68K, 1); got != "coproc:M68K#1" {
		t.Errorf("instance 1 label = %q", got)
	}
}

// TestMailboxBelowBasicArena pins the grown mailbox clear of the moved BASIC
// heap base (BASIC_MEMALLOC_BASE1 = 0x793000 after this revision).
func TestMailboxBelowBasicArena(t *testing.T) {
	const basicMemallocBase1 = 0x793000
	if MAILBOX_END >= basicMemallocBase1 {
		t.Errorf("mailbox end %#x collides with BASIC_MEMALLOC_BASE1 %#x", MAILBOX_END, basicMemallocBase1)
	}
}

// startM68KInstanceViaMMIO stages a spin-loop image in guest RAM and issues
// COPROC_CMD_START_MEM for the given instance through the MMIO surface.
func startM68KInstanceViaMMIO(t *testing.T, bus *MachineBus, mgr *CoprocessorManager, instance uint32) {
	t.Helper()
	const blobAddr = 0x20000
	mem := bus.GetMemory()
	copy(mem[blobAddr:], m68kSpinLoop)

	mgr.HandleWrite(COPROC_CPU_TYPE, EXEC_TYPE_M68K)
	mgr.HandleWrite(COPROC_INSTANCE, instance)
	mgr.HandleWrite(COPROC_REQ_PTR, blobAddr)
	mgr.HandleWrite(COPROC_REQ_LEN, uint32(len(m68kSpinLoop)))
	mgr.HandleWrite(COPROC_CMD, COPROC_CMD_START_MEM)
	if st := mgr.HandleRead(COPROC_CMD_STATUS); st != COPROC_STATUS_OK {
		t.Fatalf("START_MEM instance %d failed: status %d error %d",
			instance, st, mgr.HandleRead(COPROC_CMD_ERROR))
	}
}

func TestCoprocInstance_SecondM68KWorkerLoadsInSecondWindow(t *testing.T) {
	bus, mgr := newTestBusAndManager(t)
	defer mgr.StopAll()

	startM68KInstanceViaMMIO(t, bus, mgr, 0)
	startM68KInstanceViaMMIO(t, bus, mgr, 1)

	base0, _, _, _ := workerWindow(EXEC_TYPE_M68K, 0)
	base1, end1, _, _ := workerWindow(EXEC_TYPE_M68K, 1)
	mem := bus.GetMemory()
	if mem[base0] != 0x60 || mem[base0+1] != 0xFE {
		t.Errorf("instance 0 image not at %#x", base0)
	}
	if mem[base1] != 0x60 || mem[base1+1] != 0xFE {
		t.Errorf("instance 1 image not at %#x", base1)
	}

	state := mgr.HandleRead(COPROC_WORKER_STATE)
	if state&(1<<EXEC_TYPE_M68K) == 0 {
		t.Errorf("worker state %#x missing M68K bit", state)
	}

	mgr.mu.Lock()
	w0 := mgr.workers[EXEC_TYPE_M68K][0]
	w1 := mgr.workers[EXEC_TYPE_M68K][1]
	mgr.mu.Unlock()
	if w0 == nil || w1 == nil {
		t.Fatalf("expected both instances online, got %v / %v", w0 != nil, w1 != nil)
	}
	if w1.loadBase != base1 || w1.loadEnd != end1 {
		t.Errorf("instance 1 window %#x-%#x, want %#x-%#x", w1.loadBase, w1.loadEnd, base1, end1)
	}
}

func TestCoprocInstance_EnqueueRoutesToSecondRing(t *testing.T) {
	bus, mgr := newTestBusAndManager(t)
	defer mgr.StopAll()

	startM68KInstanceViaMMIO(t, bus, mgr, 1)

	mgr.HandleWrite(COPROC_CPU_TYPE, EXEC_TYPE_M68K)
	mgr.HandleWrite(COPROC_INSTANCE, 1)
	mgr.HandleWrite(COPROC_OP, 42)
	mgr.HandleWrite(COPROC_REQ_PTR, 0)
	mgr.HandleWrite(COPROC_REQ_LEN, 0)
	mgr.HandleWrite(COPROC_RESP_PTR, 0)
	mgr.HandleWrite(COPROC_RESP_CAP, 0)
	mgr.HandleWrite(COPROC_CMD, COPROC_CMD_ENQUEUE)
	if st := mgr.HandleRead(COPROC_CMD_STATUS); st != COPROC_STATUS_OK {
		t.Fatalf("enqueue failed: error %d", mgr.HandleRead(COPROC_CMD_ERROR))
	}
	ticket := mgr.HandleRead(COPROC_TICKET)
	if ticket == 0 {
		t.Fatal("no ticket returned")
	}

	ring := ringBaseAddr(coprocRingIndex(EXEC_TYPE_M68K, 1)) // ring 5
	if head := bus.Read8(ring + RING_HEAD_OFFSET); head != 1 {
		t.Errorf("instance-1 ring head = %d, want 1", head)
	}
	entry := ring + RING_ENTRIES_OFFSET
	if got := bus.Read32(entry + REQ_TICKET_OFF); got != ticket {
		t.Errorf("ring descriptor ticket = %d, want %d", got, ticket)
	}
	if got := bus.Read32(entry + REQ_OP_OFF); got != 42 {
		t.Errorf("ring descriptor op = %d, want 42", got)
	}
	// The default M68K ring (index 4) must be untouched.
	if head := bus.Read8(ringBaseAddr(coprocRingIndex(EXEC_TYPE_M68K, 0)) + RING_HEAD_OFFSET); head != 0 {
		t.Errorf("instance-0 ring head = %d, want 0", head)
	}
}

func TestCoprocInstance_InvalidInstanceRejected(t *testing.T) {
	bus, mgr := newTestBusAndManager(t)
	defer mgr.StopAll()

	const blobAddr = 0x20000
	mem := bus.GetMemory()
	copy(mem[blobAddr:], m68kSpinLoop)

	// IE32 has no second instance: distinct INVALID_INSTANCE error.
	mgr.HandleWrite(COPROC_CPU_TYPE, EXEC_TYPE_IE32)
	mgr.HandleWrite(COPROC_INSTANCE, 1)
	mgr.HandleWrite(COPROC_REQ_PTR, blobAddr)
	mgr.HandleWrite(COPROC_REQ_LEN, uint32(len(m68kSpinLoop)))
	mgr.HandleWrite(COPROC_CMD, COPROC_CMD_START_MEM)
	if st := mgr.HandleRead(COPROC_CMD_STATUS); st != COPROC_STATUS_ERROR {
		t.Fatal("expected START_MEM to fail for IE32 instance 1")
	}
	if e := mgr.HandleRead(COPROC_CMD_ERROR); e != COPROC_ERR_INVALID_INSTANCE {
		t.Errorf("error = %d, want COPROC_ERR_INVALID_INSTANCE", e)
	}

	// M68K instance 2 is beyond the limit.
	mgr.HandleWrite(COPROC_CPU_TYPE, EXEC_TYPE_M68K)
	mgr.HandleWrite(COPROC_INSTANCE, 2)
	mgr.HandleWrite(COPROC_CMD, COPROC_CMD_ENQUEUE)
	if e := mgr.HandleRead(COPROC_CMD_ERROR); e != COPROC_ERR_INVALID_INSTANCE {
		t.Errorf("enqueue instance 2 error = %d, want COPROC_ERR_INVALID_INSTANCE", e)
	}
}

func TestCoprocInstance_StopIsPerInstance(t *testing.T) {
	bus, mgr := newTestBusAndManager(t)
	defer mgr.StopAll()

	startM68KInstanceViaMMIO(t, bus, mgr, 0)
	startM68KInstanceViaMMIO(t, bus, mgr, 1)

	mgr.HandleWrite(COPROC_CPU_TYPE, EXEC_TYPE_M68K)
	mgr.HandleWrite(COPROC_INSTANCE, 1)
	mgr.HandleWrite(COPROC_CMD, COPROC_CMD_STOP)
	if st := mgr.HandleRead(COPROC_CMD_STATUS); st != COPROC_STATUS_OK {
		t.Fatalf("stop instance 1 failed: error %d", mgr.HandleRead(COPROC_CMD_ERROR))
	}

	deadline := time.Now().Add(time.Second)
	for {
		mgr.mu.Lock()
		up1 := mgr.workers[EXEC_TYPE_M68K][1] != nil
		up0 := mgr.workers[EXEC_TYPE_M68K][0] != nil
		mgr.mu.Unlock()
		if !up1 {
			if !up0 {
				t.Error("instance 0 went down with instance 1")
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("instance 1 still reported online after stop")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestCoprocInstance_WorkerDownMarksOnlyItsTickets(t *testing.T) {
	bus, mgr := newTestBusAndManager(t)
	defer mgr.StopAll()

	startM68KInstanceViaMMIO(t, bus, mgr, 0)
	startM68KInstanceViaMMIO(t, bus, mgr, 1)

	enqueue := func(instance uint32) uint32 {
		mgr.HandleWrite(COPROC_CPU_TYPE, EXEC_TYPE_M68K)
		mgr.HandleWrite(COPROC_INSTANCE, instance)
		mgr.HandleWrite(COPROC_OP, 7)
		mgr.HandleWrite(COPROC_CMD, COPROC_CMD_ENQUEUE)
		return mgr.HandleRead(COPROC_TICKET)
	}
	t0 := enqueue(0)
	t1 := enqueue(1)

	mgr.HandleWrite(COPROC_INSTANCE, 1)
	mgr.HandleWrite(COPROC_CMD, COPROC_CMD_STOP)

	poll := func(ticket uint32) uint32 {
		mgr.HandleWrite(COPROC_TICKET, ticket)
		mgr.HandleWrite(COPROC_CMD, COPROC_CMD_POLL)
		return mgr.HandleRead(COPROC_TICKET_STATUS)
	}
	if st := poll(t1); st != COPROC_TICKET_WORKER_DOWN {
		t.Errorf("instance 1 ticket status = %d, want WORKER_DOWN", st)
	}
	// The spin-loop worker never serves its ring; instance 0's ticket must
	// still be pending, not worker-down.
	if st := poll(t0); st != COPROC_TICKET_PENDING && st != COPROC_TICKET_RUNNING {
		t.Errorf("instance 0 ticket status = %d, want pending/running", st)
	}
}

// TestWorkerWindow_NoOverlap asserts the nine windows (6 instance-0 + 3
// instance-1) are mutually disjoint and clear of the mailbox and BASIC arenas.
func TestWorkerWindow_NoOverlap(t *testing.T) {
	type win struct {
		name     string
		lo, hi   uint32
	}
	var wins []win
	types := map[uint32]string{
		EXEC_TYPE_IE32: "IE32", EXEC_TYPE_M68K: "M68K", EXEC_TYPE_6502: "6502",
		EXEC_TYPE_Z80: "Z80", EXEC_TYPE_X86: "X86", EXEC_TYPE_IE64: "IE64",
	}
	for ct, name := range types {
		for inst := uint32(0); inst < coprocInstanceLimit(ct); inst++ {
			b, e, sz, ok := workerWindow(ct, inst)
			if !ok {
				t.Fatalf("%s#%d unexpectedly not ok", name, inst)
			}
			if e-b+1 != sz {
				t.Errorf("%s#%d size mismatch", name, inst)
			}
			wins = append(wins, win{name, b, e})
		}
	}
	overlap := func(a, b win) bool { return a.lo <= b.hi && b.lo <= a.hi }
	for i := 0; i < len(wins); i++ {
		for j := i + 1; j < len(wins); j++ {
			if overlap(wins[i], wins[j]) {
				t.Errorf("windows overlap: %v and %v", wins[i], wins[j])
			}
		}
		// Clear of the mailbox and of the BASIC arenas.
		if wins[i].lo <= MAILBOX_END && MAILBOX_BASE <= wins[i].hi {
			t.Errorf("%v overlaps mailbox", wins[i])
		}
		if wins[i].hi >= 0x600000 {
			t.Errorf("%v reaches into the BASIC arena at 0x600000", wins[i])
		}
		if wins[i].lo < 0x200000 {
			t.Errorf("%v dips below the worker-safe aperture (VRAM)", wins[i])
		}
	}
}

// TestWorkerWindow_InstanceLimits pins the exact table and instance limits.
func TestWorkerWindow_InstanceLimits(t *testing.T) {
	type tc struct {
		ct         uint32
		inst       uint32
		base, end  uint32
		ok         bool
	}
	cases := []tc{
		{EXEC_TYPE_IE32, 0, 0x200000, 0x27FFFF, true},
		{EXEC_TYPE_IE32, 1, 0, 0, false},
		{EXEC_TYPE_M68K, 0, 0x280000, 0x2FFFFF, true},
		{EXEC_TYPE_M68K, 1, 0x420000, 0x49FFFF, true},
		{EXEC_TYPE_6502, 0, 0x300000, 0x30FFFF, true},
		{EXEC_TYPE_6502, 1, 0, 0, false},
		{EXEC_TYPE_Z80, 0, 0x310000, 0x31FFFF, true},
		{EXEC_TYPE_Z80, 1, 0, 0, false},
		{EXEC_TYPE_X86, 0, 0x320000, 0x39FFFF, true},
		{EXEC_TYPE_X86, 1, 0x4A0000, 0x51FFFF, true},
		{EXEC_TYPE_IE64, 0, 0x3A0000, 0x41FFFF, true},
		{EXEC_TYPE_IE64, 1, 0x520000, 0x59FFFF, true},
	}
	for _, c := range cases {
		b, e, _, ok := workerWindow(c.ct, c.inst)
		if ok != c.ok {
			t.Errorf("workerWindow(%d,%d) ok=%v want %v", c.ct, c.inst, ok, c.ok)
			continue
		}
		if ok && (b != c.base || e != c.end) {
			t.Errorf("workerWindow(%d,%d) = %#x-%#x, want %#x-%#x", c.ct, c.inst, b, e, c.base, c.end)
		}
	}
	// Instance 2 rejected for all types.
	for ct := uint32(1); ct <= 6; ct++ {
		if _, _, _, ok := workerWindow(ct, 2); ok {
			t.Errorf("workerWindow(%d,2) unexpectedly ok", ct)
		}
	}
}

// TestM68KCoprocSharedAddr_WorkerWindowsNotShared asserts both M68K worker
// windows keep byte-swap active (isCoprocSharedAddr false) while the mailbox
// and generic user-data range stay shared.
func TestM68KCoprocSharedAddr_WorkerWindowsNotShared(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewM68KCPU(bus)
	cpu.CoprocMode = true

	if !cpu.isCoprocSharedAddr(0x400000) {
		t.Error("0x400000 should stay in the shared skip range")
	}
	if !cpu.isCoprocSharedAddr(MAILBOX_BASE + 0x600) {
		t.Error("mailbox should stay in the shared skip range")
	}
	for inst := uint32(0); inst < coprocInstanceLimit(EXEC_TYPE_M68K); inst++ {
		b, e, _, _ := workerWindow(EXEC_TYPE_M68K, inst)
		for _, a := range []uint32{b, b + 0x1000, e} {
			if cpu.isCoprocSharedAddr(a) {
				t.Errorf("%#x is M68K worker RAM and must byte-swap like normal BE memory", a)
			}
		}
	}
}

func TestCalibration_RestoresGuestShadowRegisters(t *testing.T) {
	_, mgr := newTestBusAndManager(t)
	defer mgr.StopAll()

	// Guest-visible command state staged before calibration runs.
	mgr.HandleWrite(COPROC_CPU_TYPE, EXEC_TYPE_M68K)
	mgr.HandleWrite(COPROC_INSTANCE, 1)
	mgr.HandleWrite(COPROC_OP, 0x1234)
	mgr.HandleWrite(COPROC_REQ_PTR, 0xAAAA)
	mgr.HandleWrite(COPROC_REQ_LEN, 0x40)
	mgr.HandleWrite(COPROC_RESP_PTR, 0xBBBB)
	mgr.HandleWrite(COPROC_RESP_CAP, 0x20)

	mgr.calibrateDispatchOverhead()

	checks := []struct {
		reg  uint32
		want uint32
		name string
	}{
		{COPROC_CPU_TYPE, EXEC_TYPE_M68K, "cpuType"},
		{COPROC_INSTANCE, 1, "instance"},
		{COPROC_OP, 0x1234, "op"},
		{COPROC_REQ_PTR, 0xAAAA, "reqPtr"},
		{COPROC_REQ_LEN, 0x40, "reqLen"},
		{COPROC_RESP_PTR, 0xBBBB, "respPtr"},
		{COPROC_RESP_CAP, 0x20, "respCap"},
	}
	for _, c := range checks {
		if got := mgr.HandleRead(c.reg); got != c.want {
			t.Errorf("calibration clobbered %s: got %#x, want %#x", c.name, got, c.want)
		}
	}
}
