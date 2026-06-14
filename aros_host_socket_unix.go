//go:build linux || darwin

package main

import (
	"encoding/binary"
	"sync"
	"syscall"
	"unsafe"
)

type unixArosHostSocketBackend struct {
	mu   sync.Mutex
	next int
	fds  map[int]int
}

func NewUnixArosHostSocketBackend() arosHostSocketBackend {
	return &unixArosHostSocketBackend{next: 3, fds: make(map[int]int)}
}

func (b *unixArosHostSocketBackend) Socket(domain, typ, protocol int) (int, uint32) {
	if errno := validateArosHostSocketCreate(domain, typ, protocol); errno != 0 {
		return -1, errno
	}

	fd, err := syscall.Socket(domain, typ, protocol)
	if err != nil {
		return -1, errnoToAros(err)
	}
	_ = syscall.SetNonblock(fd, true)
	guest, errno := b.allocGuest(fd)
	if errno != 0 {
		_ = syscall.Close(fd)
		return -1, errno
	}
	return guest, 0
}

func validateArosHostSocketCreate(domain, typ, protocol int) uint32 {
	baseType := typ & ^arosHostSocketCreateFlagMask

	switch domain {
	case syscall.AF_INET:
	default:
		return arosSockErrOpNotSupp
	}

	switch baseType {
	case syscall.SOCK_STREAM:
		if protocol == 0 || protocol == syscall.IPPROTO_TCP {
			return 0
		}
	case syscall.SOCK_DGRAM:
		if protocol == 0 || protocol == syscall.IPPROTO_UDP {
			return 0
		}
	}

	return arosSockErrOpNotSupp
}

func (b *unixArosHostSocketBackend) Bind(s int, name []byte) uint32 {
	fd, ok := b.hostFD(s)
	if !ok {
		return arosSockErrBadf
	}
	sa, errno := sockaddrFromGuest(name)
	if errno != 0 {
		return errno
	}
	return errnoToAros(syscall.Bind(fd, sa))
}

func (b *unixArosHostSocketBackend) Listen(s int, backlog int) uint32 {
	fd, ok := b.hostFD(s)
	if !ok {
		return arosSockErrBadf
	}
	return errnoToAros(syscall.Listen(fd, backlog))
}

func (b *unixArosHostSocketBackend) Accept(s int, addrCap int) (int, []byte, uint32) {
	fd, ok := b.hostFD(s)
	if !ok {
		return -1, nil, arosSockErrBadf
	}
	accepted, sa, err := syscall.Accept(fd)
	if err != nil {
		return -1, nil, errnoToAros(err)
	}
	_ = syscall.SetNonblock(accepted, true)
	guest, errno := b.allocGuest(accepted)
	if errno != 0 {
		_ = syscall.Close(accepted)
		return -1, nil, errno
	}
	return guest, sockaddrToGuest(sa, addrCap), 0
}

func (b *unixArosHostSocketBackend) Connect(s int, name []byte) uint32 {
	fd, ok := b.hostFD(s)
	if !ok {
		return arosSockErrBadf
	}
	sa, errno := sockaddrFromGuest(name)
	if errno != 0 {
		return errno
	}
	err := syscall.Connect(fd, sa)
	return connectErrToAros(err)
}

func (b *unixArosHostSocketBackend) SendTo(s int, data []byte, flags int, to []byte) (int, uint32) {
	fd, ok := b.hostFD(s)
	if !ok {
		return -1, arosSockErrBadf
	}
	if len(to) != 0 {
		sa, errno := sockaddrFromGuest(to)
		if errno != 0 {
			return -1, errno
		}
		if err := syscall.Sendto(fd, data, flags, sa); err != nil {
			return -1, errnoToAros(err)
		}
		return len(data), 0
	}
	n, err := syscall.Write(fd, data)
	return n, errnoToAros(err)
}

func (b *unixArosHostSocketBackend) RecvFrom(s int, length int, flags int, fromCap int) ([]byte, []byte, uint32) {
	fd, ok := b.hostFD(s)
	if !ok {
		return nil, nil, arosSockErrBadf
	}
	buf := make([]byte, length)
	n, sa, err := syscall.Recvfrom(fd, buf, flags)
	if err != nil {
		return nil, nil, errnoToAros(err)
	}
	return buf[:n], sockaddrToGuest(sa, fromCap), 0
}

func (b *unixArosHostSocketBackend) Shutdown(s int, how int) uint32 {
	fd, ok := b.hostFD(s)
	if !ok {
		return arosSockErrBadf
	}
	return errnoToAros(syscall.Shutdown(fd, how))
}

func (b *unixArosHostSocketBackend) SetSockOpt(s int, level int, optname int, opt []byte) uint32 {
	fd, ok := b.hostFD(s)
	if !ok {
		return arosSockErrBadf
	}
	if len(opt) == 4 {
		return errnoToAros(syscall.SetsockoptInt(fd, level, optname, int(int32(binaryBE32(opt)))))
	}
	return arosSockErrOpNotSupp
}

func (b *unixArosHostSocketBackend) GetSockOpt(s int, level int, optname int, optCap int) ([]byte, uint32) {
	fd, ok := b.hostFD(s)
	if !ok {
		return nil, arosSockErrBadf
	}
	if optCap < 4 {
		return nil, arosSockErrInval
	}
	v, err := syscall.GetsockoptInt(fd, level, optname)
	if err != nil {
		return nil, errnoToAros(err)
	}
	out := make([]byte, 4)
	out[0] = byte(uint32(v) >> 24)
	out[1] = byte(uint32(v) >> 16)
	out[2] = byte(uint32(v) >> 8)
	out[3] = byte(uint32(v))
	return out, 0
}

func (b *unixArosHostSocketBackend) GetSockName(s int, cap int) ([]byte, uint32) {
	fd, ok := b.hostFD(s)
	if !ok {
		return nil, arosSockErrBadf
	}
	sa, err := syscall.Getsockname(fd)
	return sockaddrToGuest(sa, cap), errnoToAros(err)
}

func (b *unixArosHostSocketBackend) GetPeerName(s int, cap int) ([]byte, uint32) {
	fd, ok := b.hostFD(s)
	if !ok {
		return nil, arosSockErrBadf
	}
	sa, err := syscall.Getpeername(fd)
	return sockaddrToGuest(sa, cap), errnoToAros(err)
}

func (b *unixArosHostSocketBackend) Ioctl(int, uint32, uint32) uint32 { return arosSockErrOpNotSupp }

func (b *unixArosHostSocketBackend) Close(s int) uint32 {
	fd, ok := b.takeHostFD(s)
	if !ok {
		return arosSockErrBadf
	}
	return errnoToAros(syscall.Close(fd))
}

func (b *unixArosHostSocketBackend) WaitSelect(nfds int, readfds, writefds, exceptfds []byte, timeout *arosHostSocketTimeval, sigmaskPtr uint32) (int, []byte, []byte, []byte, uint32) {
	if nfds < 0 || nfds > arosHostSocketDTable {
		return -1, nil, nil, nil, arosSockErrInval
	}
	rmap, errno := b.hostFDsForSet(readfds, nfds)
	if errno != 0 {
		return -1, nil, nil, nil, errno
	}
	wmap, errno := b.hostFDsForSet(writefds, nfds)
	if errno != 0 {
		return -1, nil, nil, nil, errno
	}
	emap, errno := b.hostFDsForSet(exceptfds, nfds)
	if errno != 0 {
		return -1, nil, nil, nil, errno
	}

	var rfds, wfds, efds syscall.FdSet
	maxHost := -1
	for _, host := range rmap {
		if !fdSetCanHold(host) {
			return -1, nil, nil, nil, arosSockErrNoBufs
		}
		fdSet(&rfds, host)
		if host > maxHost {
			maxHost = host
		}
	}
	for _, host := range wmap {
		if !fdSetCanHold(host) {
			return -1, nil, nil, nil, arosSockErrNoBufs
		}
		fdSet(&wfds, host)
		if host > maxHost {
			maxHost = host
		}
	}
	for _, host := range emap {
		if !fdSetCanHold(host) {
			return -1, nil, nil, nil, arosSockErrNoBufs
		}
		fdSet(&efds, host)
		if host > maxHost {
			maxHost = host
		}
	}

	if timeout != nil {
		if timeout.Sec < 0 || timeout.Usec < 0 || timeout.Usec >= 1000000 {
			return -1, nil, nil, nil, arosSockErrInval
		}
	}

	var rp, wp, ep *syscall.FdSet
	if readfds != nil {
		rp = &rfds
	}
	if writefds != nil {
		wp = &wfds
	}
	if exceptfds != nil {
		ep = &efds
	}
	n, err := arosHostSocketSelect(maxHost+1, rp, wp, ep, timeout)
	if err != nil {
		return -1, nil, nil, nil, errnoToAros(err)
	}
	return n, guestFDSetFromReady(readfds, rmap, &rfds), guestFDSetFromReady(writefds, wmap, &wfds), guestFDSetFromReady(exceptfds, emap, &efds), 0
}

func (b *unixArosHostSocketBackend) Dup2(fd1, fd2 int) (int, uint32) {
	if fd2 < 0 || fd2 >= arosHostSocketDTable {
		return -1, arosSockErrBadf
	}
	fd, ok := b.hostFD(fd1)
	if !ok {
		return -1, arosSockErrBadf
	}
	if fd1 == fd2 {
		return fd2, 0
	}
	dup, err := syscall.Dup(fd)
	if err != nil {
		return -1, errnoToAros(err)
	}
	b.mu.Lock()
	if old, ok := b.fds[fd2]; ok {
		_ = syscall.Close(old)
	}
	b.fds[fd2] = dup
	b.mu.Unlock()
	return fd2, 0
}

func (b *unixArosHostSocketBackend) GetEvents() uint32 { return 0 }

func (b *unixArosHostSocketBackend) allocGuest(host int) (int, uint32) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for i := 0; i < arosHostSocketDTable; i++ {
		guest := (b.next + i) % arosHostSocketDTable
		if _, exists := b.fds[guest]; !exists {
			b.fds[guest] = host
			b.next = (guest + 1) % arosHostSocketDTable
			return guest, 0
		}
	}
	return -1, arosSockErrNoBufs
}

func (b *unixArosHostSocketBackend) hostFD(guest int) (int, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	fd, ok := b.fds[guest]
	return fd, ok
}

func (b *unixArosHostSocketBackend) takeHostFD(guest int) (int, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	fd, ok := b.fds[guest]
	if ok {
		delete(b.fds, guest)
	}
	return fd, ok
}

func (b *unixArosHostSocketBackend) hostFDsForSet(set []byte, nfds int) (map[int]int, uint32) {
	if set == nil {
		return nil, 0
	}
	out := make(map[int]int)
	b.mu.Lock()
	defer b.mu.Unlock()
	for guest := 0; guest < nfds; guest++ {
		if !guestFDSetIsSet(set, guest) {
			continue
		}
		host, ok := b.fds[guest]
		if !ok {
			return nil, arosSockErrBadf
		}
		out[guest] = host
	}
	return out, 0
}

func guestFDSetIsSet(set []byte, fd int) bool {
	if fd < 0 || fd >= arosHostSocketDTable || len(set) < arosHostSocketFDSetLen {
		return false
	}
	word := fd / 32
	mask := uint32(1) << uint(fd%32)
	return binary.BigEndian.Uint32(set[word*4:word*4+4])&mask != 0
}

func guestFDSetFromReady(original []byte, guestToHost map[int]int, hostSet *syscall.FdSet) []byte {
	if original == nil {
		return nil
	}
	out := make([]byte, arosHostSocketFDSetLen)
	for guest, host := range guestToHost {
		if fdIsSet(hostSet, host) {
			word := guest / 32
			v := binary.BigEndian.Uint32(out[word*4 : word*4+4])
			v |= uint32(1) << uint(guest%32)
			binary.BigEndian.PutUint32(out[word*4:word*4+4], v)
		}
	}
	return out
}

func fdSet(set *syscall.FdSet, fd int) {
	wordBits := fdSetWordBits()
	set.Bits[fd/wordBits] |= 1 << uint(fd%wordBits)
}

func fdIsSet(set *syscall.FdSet, fd int) bool {
	wordBits := fdSetWordBits()
	return set.Bits[fd/wordBits]&(1<<uint(fd%wordBits)) != 0
}

func fdSetCanHold(fd int) bool {
	var set syscall.FdSet
	wordBits := fdSetWordBits()
	return fd >= 0 && wordBits > 0 && fd/wordBits < len(set.Bits)
}

func fdSetWordBits() int {
	var set syscall.FdSet
	if len(set.Bits) == 0 {
		return 0
	}
	return int(unsafe.Sizeof(set.Bits[0]) * 8)
}

func sockaddrFromGuest(b []byte) (syscall.Sockaddr, uint32) {
	if len(b) < 8 {
		return nil, arosSockErrInval
	}
	family := int(b[1])
	if family != syscall.AF_INET {
		return nil, arosSockErrOpNotSupp
	}
	sa := &syscall.SockaddrInet4{}
	sa.Port = int(uint16(b[2])<<8 | uint16(b[3]))
	copy(sa.Addr[:], b[4:8])
	return sa, 0
}

func sockaddrToGuest(sa syscall.Sockaddr, cap int) []byte {
	if cap < 16 {
		return nil
	}
	out := make([]byte, 16)
	switch v := sa.(type) {
	case *syscall.SockaddrInet4:
		out[0] = 16
		out[1] = syscall.AF_INET
		out[2] = byte(v.Port >> 8)
		out[3] = byte(v.Port)
		copy(out[4:8], v.Addr[:])
	}
	return out
}

func binaryBE32(b []byte) uint32 {
	return uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])
}

func errnoToAros(err error) uint32 {
	if err == nil {
		return 0
	}
	switch err {
	case syscall.EBADF:
		return arosSockErrBadf
	case syscall.EINVAL:
		return arosSockErrInval
	case syscall.EWOULDBLOCK:
		return arosSockErrWouldBlock
	case syscall.EMSGSIZE:
		return arosSockErrMsgSize
	case syscall.ENOBUFS:
		return arosSockErrNoBufs
	default:
		return arosSockErrOpNotSupp
	}
}

func connectErrToAros(err error) uint32 {
	if err == syscall.EINPROGRESS || err == syscall.EALREADY {
		return arosSockErrWouldBlock
	}
	return errnoToAros(err)
}
