// bootstrap_hostfs_mem_test.go - contract tests for the in-memory hostfs store
// used by the js/wasm build (Phase 3). They drive the device through its
// command surface (arg registers + dispatch) exactly as the guest would, and
// run on the host because the store is pure Go.

package main

import "testing"

// stageCString writes a NUL-terminated string into guest memory at addr.
func stageCString(bus *MachineBus, addr uint32, s string) {
	for i := 0; i < len(s); i++ {
		bus.Write8(addr+uint32(i), s[i])
	}
	bus.Write8(addr+uint32(len(s)), 0)
}

// stageBytes writes raw bytes into guest memory at addr.
func stageBytes(bus *MachineBus, addr uint32, b []byte) {
	for i := 0; i < len(b); i++ {
		bus.Write8(addr+uint32(i), b[i])
	}
}

// readBytes reads n bytes from guest memory at addr.
func readBytes(bus *MachineBus, addr uint32, n int) []byte {
	out := make([]byte, n)
	for i := 0; i < n; i++ {
		out[i] = bus.Read8(addr + uint32(i))
	}
	return out
}

// TestWasmHostFSMem_SaveLoadRoundTrip proves a guest SAVE (create/write/close)
// commits to the store and a subsequent LOAD (open/read) reads the same bytes,
// with no host filesystem.
func TestWasmHostFSMem_SaveLoadRoundTrip(t *testing.T) {
	bus := NewMachineBus()
	dev := NewMemoryHostFSDevice(bus)

	const fnAddr, srcAddr, dstAddr = 0x1000, 0x2000, 0x3000
	payload := []byte("ROTOZOOM!\x00\x01\x02\xff")
	stageCString(bus, fnAddr, "SAVED.DAT")
	stageBytes(bus, srcAddr, payload)

	// SAVE: create + write + close.
	dev.arg1 = fnAddr
	dev.dispatch(BOOT_HOSTFS_CREATE_WRITE)
	if dev.err != 0 {
		t.Fatalf("CREATE_WRITE err=%d", dev.err)
	}
	handle := dev.res1

	dev.arg1, dev.arg2, dev.arg3 = handle, srcAddr, uint32(len(payload))
	dev.dispatch(BOOT_HOSTFS_WRITE)
	if dev.err != 0 || dev.res1 != uint32(len(payload)) {
		t.Fatalf("WRITE err=%d n=%d, want n=%d", dev.err, dev.res1, len(payload))
	}

	dev.arg1 = handle
	dev.dispatch(BOOT_HOSTFS_CLOSE)
	if dev.err != 0 {
		t.Fatalf("CLOSE err=%d", dev.err)
	}

	// LOAD: open + read.
	dev.arg1 = fnAddr
	dev.dispatch(BOOT_HOSTFS_OPEN)
	if dev.err != 0 {
		t.Fatalf("OPEN after save err=%d", dev.err)
	}
	rh := dev.res1
	dev.arg1, dev.arg2, dev.arg3 = rh, dstAddr, uint32(len(payload))
	dev.dispatch(BOOT_HOSTFS_READ)
	if dev.err != 0 || dev.res1 != uint32(len(payload)) {
		t.Fatalf("READ err=%d n=%d, want %d", dev.err, dev.res1, len(payload))
	}
	got := readBytes(bus, dstAddr, len(payload))
	if string(got) != string(payload) {
		t.Fatalf("round-trip = %q, want %q", got, payload)
	}
}

// TestWasmHostFSMem_StatReportsSize proves STAT of a store file returns its
// size and file kind from memory.
func TestWasmHostFSMem_StatReportsSize(t *testing.T) {
	bus := NewMachineBus()
	dev := NewMemoryHostFSDevice(bus)
	dev.SetSpecialFile("TUNE.SID", make([]byte, 4242))

	const fnAddr, statAddr = 0x1000, 0x4000
	stageCString(bus, fnAddr, "TUNE.SID")
	dev.arg1, dev.arg2 = fnAddr, statAddr
	dev.dispatch(BOOT_HOSTFS_STAT)
	if dev.err != 0 {
		t.Fatalf("STAT err=%d", dev.err)
	}
	if dev.res1 != 4242 || dev.res2 != BOOT_HOSTFS_KIND_FILE {
		t.Fatalf("STAT res1=%d res2=%d, want 4242/%d", dev.res1, dev.res2, BOOT_HOSTFS_KIND_FILE)
	}
}

// TestWasmHostFSMem_EmbeddedReadServesLoad proves a seeded (embedded) asset is
// readable through open/read, which is the LOAD/BLOAD/SOUND PLAY path.
func TestWasmHostFSMem_EmbeddedReadServesLoad(t *testing.T) {
	bus := NewMachineBus()
	dev := NewMemoryHostFSDevice(bus)
	asset := []byte{0xDE, 0xAD, 0xBE, 0xEF, 0x10, 0x20}
	dev.SetSpecialFile("ROTOZOOMTEXTURE.RAW", asset)

	const fnAddr, dstAddr = 0x1000, 0x2000
	stageCString(bus, fnAddr, "rotozoomtexture.raw") // lower-case: key normalises
	dev.arg1 = fnAddr
	dev.dispatch(BOOT_HOSTFS_OPEN)
	if dev.err != 0 {
		t.Fatalf("OPEN embedded err=%d", dev.err)
	}
	dev.arg1, dev.arg2, dev.arg3 = dev.res1, dstAddr, uint32(len(asset))
	dev.dispatch(BOOT_HOSTFS_READ)
	if dev.err != 0 || dev.res1 != uint32(len(asset)) {
		t.Fatalf("READ embedded err=%d n=%d", dev.err, dev.res1)
	}
	if got := readBytes(bus, dstAddr, len(asset)); string(got) != string(asset) {
		t.Fatalf("embedded read = %v, want %v", got, asset)
	}
}

// TestWasmHostFSMem_ReadDirListsRoot proves READDIR enumerates root files from
// the store, sorted, so a guest DIR works.
func TestWasmHostFSMem_ReadDirListsRoot(t *testing.T) {
	bus := NewMachineBus()
	dev := NewMemoryHostFSDevice(bus)
	dev.SetSpecialFile("beta.bas", []byte("b"))
	dev.SetSpecialFile("Alpha.SID", []byte("a"))

	const direntAddr = 0x5000
	want := []string{"ALPHA.SID", "BETA.BAS"} // keys are upper-cased, sorted CI
	for i, wantName := range want {
		dev.arg1, dev.arg2, dev.arg3 = 0, uint32(i), direntAddr // arg1=0 -> root
		dev.dispatch(BOOT_HOSTFS_READDIR)
		if dev.err != 0 || dev.res1 != 1 {
			t.Fatalf("READDIR idx=%d err=%d res1=%d", i, dev.err, dev.res1)
		}
		name := readCStringFromBus(bus, direntAddr+BOOT_HOSTFS_DIRENT_NAME_OFF, BOOT_HOSTFS_DIRENT_NAME_MAX)
		if name != wantName {
			t.Fatalf("READDIR idx=%d name=%q, want %q", i, name, wantName)
		}
	}
	// One past the end reports not-found.
	dev.arg1, dev.arg2, dev.arg3 = 0, uint32(len(want)), direntAddr
	dev.dispatch(BOOT_HOSTFS_READDIR)
	if dev.err != 4 {
		t.Fatalf("READDIR past end err=%d, want 4", dev.err)
	}
}

// TestWasmHostFSMem_ReadDirReportsNestedDirKind proves a synthesised virtual
// directory (a name with deeper keys beneath it) is reported as KIND_DIR, not
// KIND_FILE, so guests can browse embedded directory trees.
func TestWasmHostFSMem_ReadDirReportsNestedDirKind(t *testing.T) {
	bus := NewMachineBus()
	dev := NewMemoryHostFSDevice(bus)
	dev.SetSpecialFile("DEMOS/INTRO.BAS", []byte("x"))
	dev.SetSpecialFile("readme.txt", []byte("y"))

	const direntAddr = 0x5000
	// Root listing, sorted CI: DEMOS (dir), README.TXT (file).
	type want struct {
		name string
		kind uint32
	}
	wants := []want{
		{"DEMOS", BOOT_HOSTFS_KIND_DIR},
		{"README.TXT", BOOT_HOSTFS_KIND_FILE},
	}
	for i, w := range wants {
		dev.arg1, dev.arg2, dev.arg3 = 0, uint32(i), direntAddr
		dev.dispatch(BOOT_HOSTFS_READDIR)
		if dev.err != 0 {
			t.Fatalf("READDIR idx=%d err=%d", i, dev.err)
		}
		if dev.res2 != w.kind {
			t.Fatalf("idx=%d kind=%d, want %d", i, dev.res2, w.kind)
		}
		kindInBuf := bus.Read8(direntAddr + BOOT_HOSTFS_DIRENT_KIND_OFF)
		if uint32(kindInBuf) != w.kind {
			t.Fatalf("idx=%d dirent-buf kind=%d, want %d", i, kindInBuf, w.kind)
		}
		name := readCStringFromBus(bus, direntAddr+BOOT_HOSTFS_DIRENT_NAME_OFF, BOOT_HOSTFS_DIRENT_NAME_MAX)
		if name != w.name {
			t.Fatalf("idx=%d name=%q, want %q", i, name, w.name)
		}
	}
}

// TestWasmHostFSMem_MissDoesNotReachHost proves a miss on OPEN/STAT returns a
// clean not-found from the store, never falling through to a host filesystem
// lookup (which on the host would resolve the process cwd).
func TestWasmHostFSMem_MissDoesNotReachHost(t *testing.T) {
	bus := NewMachineBus()
	dev := NewMemoryHostFSDevice(bus)
	if dev.hostRoot != "" {
		t.Fatalf("memory device unexpectedly has hostRoot %q", dev.hostRoot)
	}

	const fnAddr = 0x1000
	stageCString(bus, fnAddr, "NOPE.DAT")

	dev.arg1 = fnAddr
	dev.dispatch(BOOT_HOSTFS_OPEN)
	if dev.err != 4 {
		t.Fatalf("OPEN miss err=%d, want 4 (not-found)", dev.err)
	}

	dev.arg1, dev.arg2 = fnAddr, 0x4000
	dev.dispatch(BOOT_HOSTFS_STAT)
	if dev.err != 4 {
		t.Fatalf("STAT miss err=%d, want 4 (not-found)", dev.err)
	}
}

// TestWasmHostFSMem_FaultedWriteLeavesBufferUnchanged proves a write that
// faults partway through does not partially mutate the pending file: closing
// after the fault commits nothing, and a clean retry does not duplicate the
// readable prefix.
func TestWasmHostFSMem_FaultedWriteLeavesBufferUnchanged(t *testing.T) {
	bus := NewMachineBus()
	dev := NewMemoryHostFSDevice(bus)

	const fnAddr = 0x1000
	stageCString(bus, fnAddr, "PARTIAL.DAT")
	dev.arg1 = fnAddr
	dev.dispatch(BOOT_HOSTFS_CREATE_WRITE)
	if dev.err != 0 {
		t.Fatalf("CREATE_WRITE err=%d", dev.err)
	}
	handle := dev.res1

	// Source straddles the top of guest RAM: the first bytes read, a later
	// byte faults. len(bus.memory) is the first unbacked address.
	top := uint32(len(bus.memory))
	src := top - 2
	stageBytes(bus, src, []byte{0xAA, 0xBB})
	dev.arg1, dev.arg2, dev.arg3 = handle, src, 4 // 2 readable + 2 faulting
	dev.dispatch(BOOT_HOSTFS_WRITE)
	if dev.err != 5 {
		t.Fatalf("faulting WRITE err=%d, want 5", dev.err)
	}
	if mh := dev.memWriteHandles[handle]; mh == nil || len(mh.buf) != 0 {
		t.Fatalf("pending buffer mutated by faulted write: %v", mh)
	}

	// Clean retry of just the readable bytes, then close: exactly those bytes
	// are committed, no duplicated prefix.
	dev.arg1, dev.arg2, dev.arg3 = handle, src, 2
	dev.dispatch(BOOT_HOSTFS_WRITE)
	if dev.err != 0 || dev.res1 != 2 {
		t.Fatalf("retry WRITE err=%d n=%d", dev.err, dev.res1)
	}
	dev.arg1 = handle
	dev.dispatch(BOOT_HOSTFS_CLOSE)
	if got, ok := dev.specialFile("PARTIAL.DAT"); !ok || string(got) != string([]byte{0xAA, 0xBB}) {
		t.Fatalf("committed = %v ok=%v, want [AA BB]", got, ok)
	}
}

func readCStringFromBus(bus *MachineBus, addr uint32, limit int) string {
	var b []byte
	for i := 0; i < limit; i++ {
		c := bus.Read8(addr + uint32(i))
		if c == 0 {
			break
		}
		b = append(b, c)
	}
	return string(b)
}

// TestWasmHostFSMem_CreateWriteRejectsInvalidPaths proves the in-memory
// CREATE_WRITE path applies the same lexical rejection as the native
// resolveForCreate: absolute paths and traversal segments never become
// committed store entries (P2 review finding).
func TestWasmHostFSMem_CreateWriteRejectsInvalidPaths(t *testing.T) {
	cases := []struct {
		name    string
		path    string
		wantErr uint32
	}{
		{"parent traversal", "../escape.dat", 5},
		{"embedded traversal", "a/../../escape.dat", 5},
		{"absolute", "/abs.dat", 5},
		{"dot", ".", 3},
		{"root", "/", 3},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bus := NewMachineBus()
			dev := NewMemoryHostFSDevice(bus)
			const fnAddr = 0x1000
			stageCString(bus, fnAddr, tc.path)
			dev.arg1 = fnAddr
			dev.dispatch(BOOT_HOSTFS_CREATE_WRITE)
			if dev.err != tc.wantErr {
				t.Fatalf("CREATE_WRITE %q err=%d, want %d", tc.path, dev.err, tc.wantErr)
			}
			if len(dev.memWriteHandles) != 0 {
				t.Fatalf("CREATE_WRITE %q opened a handle despite rejection", tc.path)
			}
		})
	}
}

// TestWasmHostFSMem_StatReportsSyntheticDir proves STAT of a name that READDIR
// advertises as a directory (a prefix of deeper store keys) returns KIND_DIR
// instead of not-found, matching native os.Stat semantics (P2 review finding).
func TestWasmHostFSMem_StatReportsSyntheticDir(t *testing.T) {
	bus := NewMachineBus()
	dev := NewMemoryHostFSDevice(bus)
	dev.SetSpecialFile("Demos/ie64/foo.ie64", []byte{1, 2, 3})

	const fnAddr, statAddr = 0x1000, 0x4000
	for _, name := range []string{"Demos", "demos", "Demos/ie64"} {
		stageCString(bus, fnAddr, name)
		dev.arg1, dev.arg2 = fnAddr, statAddr
		dev.dispatch(BOOT_HOSTFS_STAT)
		if dev.err != 0 {
			t.Fatalf("STAT %q err=%d, want directory hit", name, dev.err)
		}
		if dev.res2 != BOOT_HOSTFS_KIND_DIR {
			t.Fatalf("STAT %q kind=%d, want %d (DIR)", name, dev.res2, BOOT_HOSTFS_KIND_DIR)
		}
	}

	// A genuine miss must still be a clean not-found.
	stageCString(bus, fnAddr, "NoSuchDir")
	dev.arg1, dev.arg2 = fnAddr, statAddr
	dev.dispatch(BOOT_HOSTFS_STAT)
	if dev.err != 4 {
		t.Fatalf("STAT miss err=%d, want 4", dev.err)
	}
}
