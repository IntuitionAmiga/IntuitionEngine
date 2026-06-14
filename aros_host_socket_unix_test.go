//go:build linux || darwin

package main

import (
	"encoding/binary"
	"os"
	"syscall"
	"testing"
)

func TestUnixArosHostSocketRejectsUnownedDescriptors(t *testing.T) {
	backend := NewUnixArosHostSocketBackend()
	if errno := backend.Close(1); errno != arosSockErrBadf {
		t.Fatalf("Close(1) errno=%d, want EBADF", errno)
	}
	if _, errno := backend.Dup2(1, 2); errno != arosSockErrBadf {
		t.Fatalf("Dup2(1, 2) errno=%d, want EBADF", errno)
	}
}

func TestUnixArosHostSocketCreatePolicy(t *testing.T) {
	tests := []struct {
		name     string
		domain   int
		typ      int
		protocol int
		want     uint32
	}{
		{
			name:     "IPv4 stream default protocol",
			domain:   syscall.AF_INET,
			typ:      syscall.SOCK_STREAM,
			protocol: 0,
			want:     0,
		},
		{
			name:     "IPv6 rejected",
			domain:   syscall.AF_INET6,
			typ:      syscall.SOCK_DGRAM,
			protocol: syscall.IPPROTO_UDP,
			want:     arosSockErrOpNotSupp,
		},
		{
			name:     "creation flags ignored for base type",
			domain:   syscall.AF_INET,
			typ:      syscall.SOCK_STREAM | arosHostSocketCreateFlagMask,
			protocol: syscall.IPPROTO_TCP,
			want:     0,
		},
		{
			name:     "Unix sockets blocked",
			domain:   syscall.AF_UNIX,
			typ:      syscall.SOCK_STREAM,
			protocol: 0,
			want:     arosSockErrOpNotSupp,
		},
		{
			name:     "raw sockets blocked",
			domain:   syscall.AF_INET,
			typ:      syscall.SOCK_RAW,
			protocol: syscall.IPPROTO_ICMP,
			want:     arosSockErrOpNotSupp,
		},
		{
			name:     "stream UDP protocol blocked",
			domain:   syscall.AF_INET,
			typ:      syscall.SOCK_STREAM,
			protocol: syscall.IPPROTO_UDP,
			want:     arosSockErrOpNotSupp,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := validateArosHostSocketCreate(tt.domain, tt.typ, tt.protocol); got != tt.want {
				t.Fatalf("validateArosHostSocketCreate()=%d, want %d", got, tt.want)
			}
		})
	}
}

func TestUnixArosHostSocketSocketRejectsDisallowedFamily(t *testing.T) {
	backend := NewUnixArosHostSocketBackend()
	fd, errno := backend.Socket(syscall.AF_UNIX, syscall.SOCK_STREAM, 0)
	if fd != -1 || errno != arosSockErrOpNotSupp {
		t.Fatalf("Socket(AF_UNIX)=(%d, %d), want (-1, %d)", fd, errno, arosSockErrOpNotSupp)
	}
	fd, errno = backend.Socket(syscall.AF_INET6, syscall.SOCK_STREAM, syscall.IPPROTO_TCP)
	if fd != -1 || errno != arosSockErrOpNotSupp {
		t.Fatalf("Socket(AF_INET6)=(%d, %d), want (-1, %d)", fd, errno, arosSockErrOpNotSupp)
	}
}

func TestUnixArosHostSocketConnectPendingErrorsWouldBlock(t *testing.T) {
	for _, err := range []error{syscall.EINPROGRESS, syscall.EALREADY} {
		if got := connectErrToAros(err); got != arosSockErrWouldBlock {
			t.Fatalf("connectErrToAros(%v)=%d, want EWOULDBLOCK", err, got)
		}
	}
}

func TestUnixArosHostSocketWaitSelectReportsReadiness(t *testing.T) {
	fds, err := syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_STREAM, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer syscall.Close(fds[0])
	defer syscall.Close(fds[1])

	backend := &unixArosHostSocketBackend{
		next: 5,
		fds: map[int]int{
			3: fds[0],
			4: fds[1],
		},
		released: make(map[int]int),
	}
	if _, err := syscall.Write(fds[1], []byte{0x7f}); err != nil {
		t.Fatal(err)
	}

	readSet := make([]byte, arosHostSocketFDSetLen)
	setGuestFDForTest(readSet, 3)
	n, ready, _, _, errno := backend.WaitSelect(5, readSet, nil, nil, &arosHostSocketTimeval{}, 0)
	if errno != 0 {
		t.Fatalf("WaitSelect errno=%d", errno)
	}
	if n != 1 {
		t.Fatalf("WaitSelect ready count=%d, want 1", n)
	}
	if !guestFDSetIsSet(ready, 3) {
		t.Fatalf("guest fd 3 was not marked ready in % x", ready)
	}
}

func TestUnixArosHostSocketDup2MinusOneAllocatesDescriptor(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	defer r.Close()

	backend := &unixArosHostSocketBackend{
		next:     3,
		fds:      map[int]int{7: int(r.Fd())},
		released: make(map[int]int),
	}

	guest, errno := backend.Dup2(7, -1)
	if errno != 0 {
		t.Fatalf("Dup2(fd, -1) errno=%d, want 0", errno)
	}
	if guest == -1 || guest == 7 {
		t.Fatalf("Dup2(fd, -1) guest=%d, want new allocated descriptor", guest)
	}
	if _, ok := backend.fds[guest]; !ok {
		t.Fatalf("Dup2(fd, -1) did not allocate returned guest descriptor")
	}
	if errno := backend.Close(guest); errno != 0 {
		t.Fatalf("Close duplicated descriptor errno=%d", errno)
	}
	delete(backend.fds, 7)
}

func TestUnixArosHostSocketDup2MinusOneMarksDescriptor(t *testing.T) {
	backend := &unixArosHostSocketBackend{
		next:     3,
		fds:      make(map[int]int),
		released: make(map[int]int),
	}

	guest, errno := backend.Dup2(-1, 9)
	if errno != 0 {
		t.Fatalf("Dup2(-1, fd) errno=%d, want 0", errno)
	}
	if guest != 9 {
		t.Fatalf("Dup2(-1, fd) guest=%d, want 9", guest)
	}
	if got := backend.fds[9]; got != arosHostSocketReservedFD {
		t.Fatalf("reserved descriptor marker=%d, want %d", got, arosHostSocketReservedFD)
	}
	if _, ok := backend.hostFD(9); ok {
		t.Fatalf("reserved descriptor must not expose a host fd")
	}
	if errno := backend.Close(9); errno != 0 {
		t.Fatalf("Close reserved descriptor errno=%d, want 0", errno)
	}
	if _, ok := backend.fds[9]; ok {
		t.Fatalf("Close reserved descriptor left reservation behind")
	}
}

func TestUnixArosHostSocketBackendReleaseObtainPreservesHostFD(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()

	backend := &unixArosHostSocketBackend{
		next:     3,
		fds:      map[int]int{7: int(r.Fd())},
		released: make(map[int]int),
	}
	r = nil

	id, errno := backend.Release(7, 0x12345678)
	if errno != 0 {
		t.Fatalf("Release errno=%d, want 0", errno)
	}
	if id != 0x12345678 {
		t.Fatalf("Release id=%#x, want explicit release key", id)
	}
	if _, ok := backend.fds[7]; ok {
		t.Fatalf("Release left caller descriptor allocated")
	}
	if _, ok := backend.released[0x12345678]; !ok {
		t.Fatalf("Release did not preserve host fd under release key")
	}

	guest, errno := backend.Obtain(0x12345678, 2, 1, 6)
	if errno != 0 {
		t.Fatalf("Obtain errno=%d, want 0", errno)
	}
	if guest == 0x12345678 {
		t.Fatalf("Obtain returned release key as descriptor")
	}
	if _, ok := backend.released[0x12345678]; ok {
		t.Fatalf("Obtain left release key allocated")
	}
	if _, ok := backend.fds[guest]; !ok {
		t.Fatalf("Obtain did not allocate returned guest descriptor")
	}
	if errno := backend.Close(guest); errno != 0 {
		t.Fatalf("Close obtained descriptor errno=%d", errno)
	}
}

func TestUnixArosHostSocketBackendReleaseCopyPreservesCallerDescriptor(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	defer r.Close()

	backend := &unixArosHostSocketBackend{
		next:     3,
		fds:      map[int]int{7: int(r.Fd())},
		released: make(map[int]int),
	}

	id, errno := backend.ReleaseCopy(7, 0x87654321)
	if errno != 0 {
		t.Fatalf("ReleaseCopy errno=%d, want 0", errno)
	}
	if id != 0x87654321 {
		t.Fatalf("ReleaseCopy id=%#x, want explicit release key", id)
	}
	if _, ok := backend.fds[7]; !ok {
		t.Fatalf("ReleaseCopy deallocated caller descriptor")
	}

	guest, errno := backend.Obtain(0x87654321, 2, 1, 6)
	if errno != 0 {
		t.Fatalf("Obtain copied socket errno=%d, want 0", errno)
	}
	if guest == 7 {
		t.Fatalf("Obtain reused caller descriptor for copied release")
	}
	if _, ok := backend.fds[7]; !ok {
		t.Fatalf("Obtain of copied release disturbed caller descriptor")
	}
	if errno := backend.Close(guest); errno != 0 {
		t.Fatalf("Close copied descriptor errno=%d", errno)
	}
	delete(backend.fds, 7)
}

func TestUnixArosHostSocketBackendReleaseGeneratesKey(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()

	backend := &unixArosHostSocketBackend{
		next:     3,
		fds:      map[int]int{7: int(r.Fd())},
		released: make(map[int]int),
	}
	r = nil

	id, errno := backend.Release(7, -1)
	if errno != 0 {
		t.Fatalf("Release UNIQUE_ID errno=%d, want 0", errno)
	}
	if id < 0 {
		t.Fatalf("Release UNIQUE_ID id=%d, want generated nonnegative key", id)
	}
	if _, ok := backend.released[id]; !ok {
		t.Fatalf("Release UNIQUE_ID did not preserve socket under generated key")
	}
	guest, errno := backend.Obtain(id, 2, 1, 6)
	if errno != 0 {
		t.Fatalf("Obtain generated key errno=%d, want 0", errno)
	}
	if errno := backend.Close(guest); errno != 0 {
		t.Fatalf("Close obtained generated-key descriptor errno=%d", errno)
	}
}

func setGuestFDForTest(set []byte, fd int) {
	word := fd / 32
	v := binary.BigEndian.Uint32(set[word*4 : word*4+4])
	v |= uint32(1) << uint(fd%32)
	binary.BigEndian.PutUint32(set[word*4:word*4+4], v)
}
