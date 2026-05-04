package main

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"
)

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
	d.handles[1] = f

	if _, res2 := dispatchArosDOS(d, ADOS_CMD_READ, 1, 0x1000, 0xFFFFFFFF); res2 != ADOS_ERROR_OBJECT_TOO_LARGE {
		t.Fatalf("READ huge res2=%d, want OBJECT_TOO_LARGE", res2)
	}
	if _, err := f.Seek(0, 0); err != nil {
		t.Fatalf("Seek: %v", err)
	}
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
	d.handles[1] = f

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
