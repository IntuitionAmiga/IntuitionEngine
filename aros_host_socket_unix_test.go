//go:build linux || darwin

package main

import (
	"encoding/binary"
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

func setGuestFDForTest(set []byte, fd int) {
	word := fd / 32
	v := binary.BigEndian.Uint32(set[word*4 : word*4+4])
	v |= uint32(1) << uint(fd%32)
	binary.BigEndian.PutUint32(set[word*4:word*4+4], v)
}
