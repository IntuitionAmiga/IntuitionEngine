package main

import (
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"
)

type countingArosFile struct {
	*os.File
	readAtCalls int
}

func (f *countingArosFile) ReadAt(p []byte, off int64) (int, error) {
	f.readAtCalls++
	return f.File.ReadAt(p, off)
}

func arosTestReadBE32(bus *MachineBus, addr uint32) uint32 {
	var buf [4]byte
	for i := range buf {
		buf[i] = bus.Read8(addr + uint32(i))
	}
	return binary.BigEndian.Uint32(buf[:])
}

func arosTestWriteBE32(bus *MachineBus, addr uint32, value uint32) {
	var buf [4]byte
	binary.BigEndian.PutUint32(buf[:], value)
	for i, b := range buf {
		bus.Write8(addr+uint32(i), b)
	}
}

var _ arosFileOps = (*countingArosFile)(nil)

func writeArosDOSString(t *testing.T, bus *MachineBus, addr uint32, s string) {
	t.Helper()
	for i := range s {
		bus.Write8(addr+uint32(i), s[i])
	}
	bus.Write8(addr+uint32(len(s)), 0)
}

func dispatchArosDOS(d *ArosDOSDevice, cmd uint32, args ...uint32) (uint32, uint32) {
	if len(args) > 0 {
		d.arg1 = args[0]
	}
	if len(args) > 1 {
		d.arg2 = args[1]
	}
	if len(args) > 2 {
		d.arg3 = args[2]
	}
	if len(args) > 3 {
		d.arg4 = args[3]
	}
	d.dispatch(cmd)
	return d.res1, d.res2
}

func TestArosDOS_CommandsUseContainmentHelpers(t *testing.T) {
	bus, d, root := newTestArosDOSDevice(t)
	if err := os.WriteFile(filepath.Join(root, "file"), []byte("abc"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := os.Mkdir(filepath.Join(root, "dir"), 0o755); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	if err := os.Symlink("/etc/passwd", filepath.Join(root, "linkOut")); err != nil {
		t.Fatalf("Symlink: %v", err)
	}

	writeArosDOSString(t, bus, 0x1000, "../escape")
	for _, cmd := range []uint32{ADOS_CMD_LOCK, ADOS_CMD_FINDINPUT, ADOS_CMD_FINDOUTPUT, ADOS_CMD_FINDUPDATE} {
		if _, res2 := dispatchArosDOS(d, cmd, 0x1000, 0); res2 != ADOS_ERROR_OBJECT_NOT_FOUND {
			t.Fatalf("cmd %d escape res2=%d, want OBJECT_NOT_FOUND", cmd, res2)
		}
	}
	if _, res2 := dispatchArosDOS(d, ADOS_CMD_CREATEDIR, 0, 0x1000); res2 != ADOS_ERROR_OBJECT_NOT_FOUND {
		t.Fatalf("CREATEDIR escape res2=%d, want OBJECT_NOT_FOUND", res2)
	}
	if _, res2 := dispatchArosDOS(d, ADOS_CMD_DELETE, 0, 0x1000); res2 != ADOS_ERROR_OBJECT_NOT_FOUND {
		t.Fatalf("DELETE escape res2=%d, want OBJECT_NOT_FOUND", res2)
	}

	writeArosDOSString(t, bus, 0x1100, "linkOut")
	if _, res2 := dispatchArosDOS(d, ADOS_CMD_FINDINPUT, 0x1100, 0); res2 != ADOS_ERROR_OBJECT_NOT_FOUND {
		t.Fatalf("FINDINPUT external symlink res2=%d, want OBJECT_NOT_FOUND", res2)
	}
	if _, res2 := dispatchArosDOS(d, ADOS_CMD_FINDUPDATE, 0x1100, 0); res2 != ADOS_ERROR_OBJECT_WRONG_TYPE {
		t.Fatalf("FINDUPDATE symlink res2=%d, want OBJECT_WRONG_TYPE", res2)
	}
	if _, res2 := dispatchArosDOS(d, ADOS_CMD_FINDOUTPUT, 0x1100, 0); res2 != ADOS_ERROR_OBJECT_WRONG_TYPE {
		t.Fatalf("FINDOUTPUT symlink res2=%d, want OBJECT_WRONG_TYPE", res2)
	}
	if _, res2 := dispatchArosDOS(d, ADOS_CMD_CREATEDIR, 0, 0x1100); res2 != ADOS_ERROR_OBJECT_EXISTS {
		t.Fatalf("CREATEDIR symlink res2=%d, want OBJECT_EXISTS", res2)
	}

	writeArosDOSString(t, bus, 0x1200, "file")
	writeArosDOSString(t, bus, 0x1300, "dir")
	if _, res2 := dispatchArosDOS(d, ADOS_CMD_RENAME, 0, 0x1200, 0, 0x1300); res2 != ADOS_ERROR_OBJECT_EXISTS {
		t.Fatalf("RENAME occupied dst res2=%d, want OBJECT_EXISTS", res2)
	}
}

func TestArosDOS_LoadSegSymbolsAppliesGuestRelocationBase(t *testing.T) {
	bus, d, root := newTestArosDOSDevice(t)
	if err := os.WriteFile(filepath.Join(root, "Demo"), []byte{0, 0, 3, 0}, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "Demo.iesym"), []byte("al 0000 .entry\nal 0010 .data\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	symbols := NewSymbolTable()
	d.SetSymbolTable(symbols)
	writeArosDOSString(t, bus, 0x1000, "Demo")

	base := uint32(0x00420000)
	res1, res2 := dispatchArosDOS(d, ADOS_CMD_LOADSEG_SYMS, 0x1000, 0, base)
	if res1 != ADOS_DOSTRUE || res2 != ADOS_ERR_NONE {
		t.Fatalf("LOADSEG_SYMS res1=$%X res2=%d, want true/none", res1, res2)
	}
	if got, ok := symbols.Lookup("M68K", "entry"); !ok || got != uint64(base) {
		t.Fatalf("entry symbol=$%X ok=%v, want base $%X", got, ok, base)
	}
	if got, ok := symbols.Lookup("M68K", "data"); !ok || got != uint64(base)+0x10 {
		t.Fatalf("data symbol=$%X ok=%v, want $%X", got, ok, uint64(base)+0x10)
	}
}

func TestArosDOS_PacketClampAndGuestBounds(t *testing.T) {
	_, d, root := newTestArosDOSDevice(t)
	path := filepath.Join(root, "rw")
	if err := os.WriteFile(path, []byte("abc"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		t.Fatalf("OpenFile: %v", err)
	}
	defer f.Close()
	d.handles[1] = &arosFileHandle{file: f, name: "rw", hostPath: path, mode: arosHandleUpdate, firstRead: true}

	if _, res2 := dispatchArosDOS(d, ADOS_CMD_READ, 1, 0x1000, 0xFFFFFFFF); res2 != ADOS_ERROR_OBJECT_TOO_LARGE {
		t.Fatalf("READ huge res2=%d, want OBJECT_TOO_LARGE", res2)
	}
	d.handles[1].pos = 0
	if _, res2 := dispatchArosDOS(d, ADOS_CMD_READ, 1, 0xFFFFFFF0, 0x100); res2 != ADOS_ERROR_OBJECT_TOO_LARGE {
		t.Fatalf("READ wrap res2=%d, want OBJECT_TOO_LARGE", res2)
	}
	if _, res2 := dispatchArosDOS(d, ADOS_CMD_WRITE, 1, 0x1000, 0xFFFFFFFF); res2 != ADOS_ERROR_OBJECT_TOO_LARGE {
		t.Fatalf("WRITE huge res2=%d, want OBJECT_TOO_LARGE", res2)
	}
	if _, res2 := dispatchArosDOS(d, ADOS_CMD_WRITE, 1, 0xFFFFFFF0, 0x100); res2 != ADOS_ERROR_OBJECT_TOO_LARGE {
		t.Fatalf("WRITE wrap res2=%d, want OBJECT_TOO_LARGE", res2)
	}
}

func TestArosDOS_ReadAheadCoalescesSequentialSmallReads(t *testing.T) {
	bus, d, root := newTestArosDOSDevice(t)
	data := []byte("abcdefghijklmnopqrstuvwxyz")
	if err := os.WriteFile(filepath.Join(root, "seq"), data, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	var opened *countingArosFile
	oldOpen := arosOpenFile
	arosOpenFile = func(name string, flag int, perm os.FileMode) (arosFileOps, error) {
		f, err := os.OpenFile(name, flag, perm)
		if err != nil {
			return nil, err
		}
		opened = &countingArosFile{File: f}
		return opened, nil
	}
	t.Cleanup(func() { arosOpenFile = oldOpen })

	writeArosDOSString(t, bus, 0x1000, "seq")
	handle, res2 := dispatchArosDOS(d, ADOS_CMD_FINDINPUT, 0x1000, 0)
	if res2 != ADOS_ERR_NONE {
		t.Fatalf("FINDINPUT res2=%d", res2)
	}
	for i := 0; i < 10; i++ {
		res1, res2 := dispatchArosDOS(d, ADOS_CMD_READ, handle, 0x2000+uint32(i), 1)
		if res1 != 1 || res2 != ADOS_ERR_NONE {
			t.Fatalf("READ %d = (%d,%d), want 1/OK", i, res1, res2)
		}
	}
	got := make([]byte, 10)
	if err := ReadGuestBytes(bus, 0x2000, 0, got); err != nil {
		t.Fatalf("ReadGuestBytes: %v", err)
	}
	if string(got) != string(data[:10]) {
		t.Fatalf("guest bytes=%q, want %q", got, data[:10])
	}
	if opened == nil {
		t.Fatalf("file was not opened")
	}
	if opened.readAtCalls != 1 {
		t.Fatalf("ReadAt calls=%d, want 1", opened.readAtCalls)
	}
}

func TestArosDOS_ExamineAllPacksEntriesAndContinuation(t *testing.T) {
	bus, d, root := newTestArosDOSDevice(t)
	if err := os.WriteFile(filepath.Join(root, "alpha"), []byte("abc"), 0o644); err != nil {
		t.Fatalf("WriteFile alpha: %v", err)
	}
	if err := os.Mkdir(filepath.Join(root, "beta"), 0o755); err != nil {
		t.Fatalf("Mkdir beta: %v", err)
	}

	const (
		req     = 0x1000
		control = 0x1100
		buffer  = 0x2000
		size    = 1024
	)
	arosTestWriteBE32(bus, req+ADOS_EXALL_REQ_LOCK_KEY, 0)
	arosTestWriteBE32(bus, req+ADOS_EXALL_REQ_BUFFER, buffer)
	arosTestWriteBE32(bus, req+ADOS_EXALL_REQ_BUFFER_LEN, size)
	arosTestWriteBE32(bus, req+ADOS_EXALL_REQ_TYPE, ADOS_ED_COMMENT_TYPE)
	arosTestWriteBE32(bus, req+ADOS_EXALL_REQ_CONTROL, control)

	res1, res2 := dispatchArosDOS(d, ADOS_CMD_EXAMINE_ALL, req)
	if res1 != ADOS_DOSTRUE || res2 != ADOS_ERR_NONE {
		t.Fatalf("EXAMINE_ALL = (%d,%d), want true/OK", res1, res2)
	}
	if got := arosTestReadBE32(bus, control+ADOS_EAC_ENTRIES); got != 2 {
		t.Fatalf("eac_Entries=%d, want 2", got)
	}
	if got := arosTestReadBE32(bus, control+ADOS_EAC_LAST_KEY); got != 2 {
		t.Fatalf("eac_LastKey=%d, want 2", got)
	}

	firstNamePtr := arosTestReadBE32(bus, buffer+ADOS_ED_NAME)
	if firstNamePtr != buffer+ADOS_ED_OWNER_UID {
		t.Fatalf("first ed_Name=$%X, want $%X", firstNamePtr, buffer+ADOS_ED_OWNER_UID)
	}
	if name := d.readString(firstNamePtr); name != "alpha" {
		t.Fatalf("first name=%q, want alpha", name)
	}
	if got := arosTestReadBE32(bus, buffer+ADOS_ED_TYPE); got != ADOS_ST_FILE {
		t.Fatalf("first type=$%X, want ST_FILE", got)
	}
	if got := arosTestReadBE32(bus, buffer+ADOS_ED_SIZE); got != 3 {
		t.Fatalf("first size=%d, want 3", got)
	}
	nextPtr := arosTestReadBE32(bus, buffer+ADOS_ED_NEXT)
	if nextPtr == 0 {
		t.Fatalf("first ed_Next is zero")
	}
	if name := d.readString(arosTestReadBE32(bus, nextPtr+ADOS_ED_NAME)); name != "beta" {
		t.Fatalf("second name=%q, want beta", name)
	}
	if got := arosTestReadBE32(bus, nextPtr+ADOS_ED_TYPE); got != ADOS_ST_USERDIR {
		t.Fatalf("second type=$%X, want ST_USERDIR", got)
	}

	res1, res2 = dispatchArosDOS(d, ADOS_CMD_EXAMINE_ALL, req)
	if res1 != ADOS_DOSFALSE || res2 != ADOS_ERROR_NO_MORE_ENTRIES {
		t.Fatalf("final EXAMINE_ALL = (%d,%d), want false/NO_MORE_ENTRIES", res1, res2)
	}

	arosTestWriteBE32(bus, control+ADOS_EAC_LAST_KEY, 0)
	arosTestWriteBE32(bus, control+ADOS_EAC_ENTRIES, 123)
	arosTestWriteBE32(bus, control+ADOS_EAC_MATCH_STRING, 0x1234)
	res1, res2 = dispatchArosDOS(d, ADOS_CMD_EXAMINE_ALL, req)
	if res1 != ADOS_DOSFALSE || res2 != ADOS_ERROR_ACTION_NOT_KNOWN {
		t.Fatalf("matched EXAMINE_ALL = (%d,%d), want false/ACTION_NOT_KNOWN", res1, res2)
	}
	if got := arosTestReadBE32(bus, control+ADOS_EAC_ENTRIES); got != 123 {
		t.Fatalf("unsupported match mutated entries=%d, want 123", got)
	}
	if got := arosTestReadBE32(bus, control+ADOS_EAC_LAST_KEY); got != 0 {
		t.Fatalf("unsupported match mutated last key=%d, want 0", got)
	}
}

func TestArosDOS_ExamineAllRejectsOversizedAndInvalidBuffers(t *testing.T) {
	bus, d, root := newTestArosDOSDevice(t)
	if err := os.WriteFile(filepath.Join(root, "alpha"), []byte("abc"), 0o644); err != nil {
		t.Fatalf("WriteFile alpha: %v", err)
	}

	const (
		req     = 0x1000
		control = 0x1100
		buffer  = 0x2000
	)
	arosTestWriteBE32(bus, req+ADOS_EXALL_REQ_LOCK_KEY, 0)
	arosTestWriteBE32(bus, req+ADOS_EXALL_REQ_BUFFER, buffer)
	arosTestWriteBE32(bus, req+ADOS_EXALL_REQ_TYPE, ADOS_ED_NAME_TYPE)
	arosTestWriteBE32(bus, req+ADOS_EXALL_REQ_CONTROL, control)

	arosTestWriteBE32(bus, req+ADOS_EXALL_REQ_BUFFER_LEN, arosDOSMaxPacket+1)
	if _, res2 := dispatchArosDOS(d, ADOS_CMD_EXAMINE_ALL, req); res2 != ADOS_ERROR_OBJECT_TOO_LARGE {
		t.Fatalf("oversized EXAMINE_ALL res2=%d, want OBJECT_TOO_LARGE", res2)
	}

	arosTestWriteBE32(bus, req+ADOS_EXALL_REQ_BUFFER, 0xFFFFFFF0)
	arosTestWriteBE32(bus, req+ADOS_EXALL_REQ_BUFFER_LEN, 0x100)
	if _, res2 := dispatchArosDOS(d, ADOS_CMD_EXAMINE_ALL, req); res2 != ADOS_ERROR_OBJECT_TOO_LARGE {
		t.Fatalf("invalid-span EXAMINE_ALL res2=%d, want OBJECT_TOO_LARGE", res2)
	}
}

func TestArosDOS_WriteInvalidatesSiblingReadAhead(t *testing.T) {
	bus, d, root := newTestArosDOSDevice(t)
	if err := os.WriteFile(filepath.Join(root, "shared"), []byte("abcdef"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	writeArosDOSString(t, bus, 0x1000, "shared")

	reader, res2 := dispatchArosDOS(d, ADOS_CMD_FINDINPUT, 0x1000, 0)
	if res2 != ADOS_ERR_NONE {
		t.Fatalf("FINDINPUT res2=%d", res2)
	}
	writer, res2 := dispatchArosDOS(d, ADOS_CMD_FINDUPDATE, 0x1000, 0)
	if res2 != ADOS_ERR_NONE {
		t.Fatalf("FINDUPDATE res2=%d", res2)
	}

	if res1, res2 := dispatchArosDOS(d, ADOS_CMD_READ, reader, 0x2000, 1); res1 != 1 || res2 != ADOS_ERR_NONE {
		t.Fatalf("initial READ=(%d,%d), want 1/OK", res1, res2)
	}
	bus.Write8(0x2100, 'Z')
	if res1, res2 := dispatchArosDOS(d, ADOS_CMD_WRITE, writer, 0x2100, 1); res1 != 1 || res2 != ADOS_ERR_NONE {
		t.Fatalf("WRITE=(%d,%d), want 1/OK", res1, res2)
	}
	if _, res2 := dispatchArosDOS(d, ADOS_CMD_SEEK, reader, 0, ADOS_OFFSET_BEGINNING); res2 != ADOS_ERR_NONE {
		t.Fatalf("reader SEEK res2=%d", res2)
	}
	if res1, res2 := dispatchArosDOS(d, ADOS_CMD_READ, reader, 0x2200, 1); res1 != 1 || res2 != ADOS_ERR_NONE {
		t.Fatalf("second READ=(%d,%d), want 1/OK", res1, res2)
	}
	if got := bus.Read8(0x2200); got != 'Z' {
		t.Fatalf("sibling cached byte=%q, want Z", got)
	}
}

func TestArosDOS_FindUpdateCreatesMissingFile(t *testing.T) {
	bus, d, root := newTestArosDOSDevice(t)
	writeArosDOSString(t, bus, 0x1000, "created-by-update")
	res1, res2 := dispatchArosDOS(d, ADOS_CMD_FINDUPDATE, 0x1000, 0)
	if res1 == ADOS_DOSFALSE || res2 != ADOS_ERR_NONE {
		t.Fatalf("FINDUPDATE missing file = (%d,%d), want handle/OK", res1, res2)
	}
	if _, err := os.Stat(filepath.Join(root, "created-by-update")); err != nil {
		t.Fatalf("FINDUPDATE did not create file: %v", err)
	}
}

func TestArosDOS_BadModeAndFIBProtection(t *testing.T) {
	bus, d, root := newTestArosDOSDevice(t)
	path := filepath.Join(root, "ro")
	if err := os.WriteFile(path, []byte("abc"), 0o444); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer f.Close()
	d.handles[1] = &arosFileHandle{file: f, name: "ro", hostPath: path, mode: arosHandleRead, firstRead: true}

	if _, res2 := dispatchArosDOS(d, ADOS_CMD_SEEK, 1, 0, 99); res2 != ADOS_ERROR_BAD_NUMBER {
		t.Fatalf("SEEK bad mode res2=%d, want BAD_NUMBER", res2)
	}
	if _, res2 := dispatchArosDOS(d, ADOS_CMD_SET_FILESIZE, 1, 1, 99); res2 != ADOS_ERROR_BAD_NUMBER {
		t.Fatalf("SET_FILESIZE bad mode res2=%d, want BAD_NUMBER", res2)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	d.fillFIB(0x2000, info, path)
	prot := uint32(bus.Read8(0x2000+ADOS_FIB_PROTECTION))<<24 |
		uint32(bus.Read8(0x2000+ADOS_FIB_PROTECTION+1))<<16 |
		uint32(bus.Read8(0x2000+ADOS_FIB_PROTECTION+2))<<8 |
		uint32(bus.Read8(0x2000+ADOS_FIB_PROTECTION+3))
	if prot&ADOS_FIBF_WRITE == 0 {
		t.Fatalf("read-only file protection=0x%X, want FIBF_WRITE set", prot)
	}
}

func TestArosDOS_SpecialNodeCommandMapping(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX special-node semantics")
	}
	bus, d, root := newTestArosDOSDevice(t)
	fifo := filepath.Join(root, "fifo")
	if err := makeTestFIFO(fifo); err != nil {
		t.Skipf("FIFO not available: %v", err)
	}
	writeArosDOSString(t, bus, 0x1000, "fifo")

	if _, res2 := dispatchArosDOS(d, ADOS_CMD_LOCK, 0x1000, 0); res2 != ADOS_ERR_NONE {
		t.Fatalf("LOCK FIFO res2=%d, want OK", res2)
	}
	if _, res2 := dispatchArosDOS(d, ADOS_CMD_FINDINPUT, 0x1000, 0); res2 != ADOS_ERROR_OBJECT_WRONG_TYPE {
		t.Fatalf("FINDINPUT FIFO res2=%d, want OBJECT_WRONG_TYPE", res2)
	}
	if _, res2 := dispatchArosDOS(d, ADOS_CMD_FINDUPDATE, 0x1000, 0); res2 != ADOS_ERROR_OBJECT_WRONG_TYPE {
		t.Fatalf("FINDUPDATE FIFO res2=%d, want OBJECT_WRONG_TYPE", res2)
	}
	if _, res2 := dispatchArosDOS(d, ADOS_CMD_FINDOUTPUT, 0x1000, 0); res2 != ADOS_ERROR_OBJECT_WRONG_TYPE {
		t.Fatalf("FINDOUTPUT FIFO res2=%d, want OBJECT_WRONG_TYPE", res2)
	}
	if _, res2 := dispatchArosDOS(d, ADOS_CMD_CREATEDIR, 0, 0x1000); res2 != ADOS_ERROR_OBJECT_EXISTS {
		t.Fatalf("CREATEDIR FIFO res2=%d, want OBJECT_EXISTS", res2)
	}
	if _, res2 := dispatchArosDOS(d, ADOS_CMD_DELETE, 0, 0x1000); res2 != ADOS_ERROR_OBJECT_WRONG_TYPE {
		t.Fatalf("DELETE FIFO res2=%d, want OBJECT_WRONG_TYPE", res2)
	}

	writeArosDOSString(t, bus, 0x1100, "dst")
	if _, res2 := dispatchArosDOS(d, ADOS_CMD_RENAME, 0, 0x1000, 0, 0x1100); res2 != ADOS_ERROR_OBJECT_WRONG_TYPE {
		t.Fatalf("RENAME FIFO src res2=%d, want OBJECT_WRONG_TYPE", res2)
	}
	if err := os.WriteFile(filepath.Join(root, "src"), []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile src: %v", err)
	}
	writeArosDOSString(t, bus, 0x1200, "src")
	if _, res2 := dispatchArosDOS(d, ADOS_CMD_RENAME, 0, 0x1200, 0, 0x1000); res2 != ADOS_ERROR_OBJECT_EXISTS {
		t.Fatalf("RENAME FIFO dst res2=%d, want OBJECT_EXISTS", res2)
	}
}

func TestArosDOS_ErrnoMapping(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want uint32
		fn   func(error) uint32
	}{
		{"write ENOSPC", syscall.ENOSPC, ADOS_ERROR_DISK_FULL, mapWriteErr},
		{"write EROFS", syscall.EROFS, ADOS_ERROR_DISK_WRITE_PROTECTED, mapWriteErr},
		{"write EBUSY", syscall.EBUSY, ADOS_ERROR_OBJECT_IN_USE, mapWriteErr},
		{"delete EBUSY", syscall.EBUSY, ADOS_ERROR_OBJECT_IN_USE, mapDeleteErr},
		{"rename ENOSPC", syscall.ENOSPC, ADOS_ERROR_DISK_FULL, mapRenameErr},
		{"rename EROFS", syscall.EROFS, ADOS_ERROR_DISK_WRITE_PROTECTED, mapRenameErr},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := &os.PathError{Op: "test", Path: "x", Err: tc.err}
			if !errors.Is(err, tc.err) {
				t.Fatalf("test setup PathError does not wrap errno")
			}
			if got := tc.fn(err); got != tc.want {
				t.Fatalf("mapped %v to %d, want %d", tc.err, got, tc.want)
			}
		})
	}
}
