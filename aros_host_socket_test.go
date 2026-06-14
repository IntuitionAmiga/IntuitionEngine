package main

import (
	"encoding/binary"
	"testing"
)

type fakeArosHostSocketBackend struct {
	socketDomain    int
	socketType      int
	socketProto     int
	sendCalls       int
	sendData        []byte
	recvData        []byte
	events          uint32
	waitSelectCalls int
}

func (f *fakeArosHostSocketBackend) Socket(domain, typ, protocol int) (int, uint32) {
	f.socketDomain, f.socketType, f.socketProto = domain, typ, protocol
	return 7, 0
}
func (f *fakeArosHostSocketBackend) Bind(int, []byte) uint32 { return 0 }
func (f *fakeArosHostSocketBackend) Listen(int, int) uint32  { return 0 }
func (f *fakeArosHostSocketBackend) Accept(int, int) (int, []byte, uint32) {
	return 8, []byte{16, 2, 1, 2}, 0
}
func (f *fakeArosHostSocketBackend) Connect(int, []byte) uint32 { return 0 }
func (f *fakeArosHostSocketBackend) SendTo(s int, data []byte, flags int, to []byte) (int, uint32) {
	f.sendCalls++
	f.sendData = append([]byte(nil), data...)
	return len(data), 0
}
func (f *fakeArosHostSocketBackend) RecvFrom(s int, length int, flags int, fromCap int) ([]byte, []byte, uint32) {
	if len(f.recvData) > length {
		return f.recvData[:length], nil, 0
	}
	return f.recvData, nil, 0
}
func (f *fakeArosHostSocketBackend) Shutdown(int, int) uint32                { return 0 }
func (f *fakeArosHostSocketBackend) SetSockOpt(int, int, int, []byte) uint32 { return 0 }
func (f *fakeArosHostSocketBackend) GetSockOpt(int, int, int, int) ([]byte, uint32) {
	return []byte{0, 0, 0, 1}, 0
}
func (f *fakeArosHostSocketBackend) GetSockName(int, int) ([]byte, uint32) { return nil, 0 }
func (f *fakeArosHostSocketBackend) GetPeerName(int, int) ([]byte, uint32) { return nil, 0 }
func (f *fakeArosHostSocketBackend) Ioctl(int, uint32, uint32) uint32      { return 0 }
func (f *fakeArosHostSocketBackend) Close(int) uint32                      { return 0 }
func (f *fakeArosHostSocketBackend) WaitSelect(int, []byte, []byte, []byte, *arosHostSocketTimeval, uint32) (int, []byte, []byte, []byte, uint32) {
	f.waitSelectCalls++
	return 0, nil, nil, nil, 0
}
func (f *fakeArosHostSocketBackend) Dup2(fd1, fd2 int) (int, uint32) { return fd2, 0 }
func (f *fakeArosHostSocketBackend) GetEvents() uint32               { return f.events }

func writeArosHostSocketReq(t *testing.T, bus *MachineBus, ptr uint32, words ...uint32) {
	t.Helper()
	raw := make([]byte, arosHostSocketReqSize)
	for i, v := range words {
		binary.BigEndian.PutUint32(raw[i*4:], v)
	}
	if err := WriteGuestBytes(bus, ptr, 0, raw); err != nil {
		t.Fatalf("write request: %v", err)
	}
}

func TestArosHostSocketMMIORangeDoesNotOverlapSysInfo(t *testing.T) {
	if arosHostSocketRangesOverlap(AROS_HOST_SOCKET_REGION_BASE, AROS_HOST_SOCKET_REGION_END, SYSINFO_REGION_BASE, SYSINFO_REGION_END) {
		t.Fatalf("host socket range %#x-%#x overlaps SYSINFO %#x-%#x", AROS_HOST_SOCKET_REGION_BASE, AROS_HOST_SOCKET_REGION_END, SYSINFO_REGION_BASE, SYSINFO_REGION_END)
	}
}

func TestArosHostSocketRejectsShortDescriptor(t *testing.T) {
	bus := NewMachineBus()
	dev := NewArosHostSocketDevice(bus, &fakeArosHostSocketBackend{}, true)
	bus.MapIO(AROS_HOST_SOCKET_REGION_BASE, AROS_HOST_SOCKET_REGION_END, dev.HandleRead, dev.HandleWrite)
	bus.Write32(AROS_HOST_SOCKET_REQ_PTR, 0x1000)
	bus.Write32(AROS_HOST_SOCKET_REQ_LEN, arosHostSocketReqSize-4)
	bus.Write32(AROS_HOST_SOCKET_CMD, AROS_HOST_SOCKET_CMD_SOCKET)
	if got := bus.Read32(AROS_HOST_SOCKET_ERRNO); got != arosSockErrInval {
		t.Fatalf("errno=%d, want EINVAL", got)
	}
}

func TestArosHostSocketSocketDispatch(t *testing.T) {
	bus := NewMachineBus()
	fake := &fakeArosHostSocketBackend{}
	dev := NewArosHostSocketDevice(bus, fake, true)
	bus.MapIO(AROS_HOST_SOCKET_REGION_BASE, AROS_HOST_SOCKET_REGION_END, dev.HandleRead, dev.HandleWrite)
	writeArosHostSocketReq(t, bus, 0x1000, 2, 1, 6)
	bus.Write32(AROS_HOST_SOCKET_REQ_PTR, 0x1000)
	bus.Write32(AROS_HOST_SOCKET_REQ_LEN, arosHostSocketReqSize)
	bus.Write32(AROS_HOST_SOCKET_CMD, AROS_HOST_SOCKET_CMD_SOCKET)
	if got := bus.Read32(AROS_HOST_SOCKET_RES1); got != 7 {
		t.Fatalf("socket result=%d, want 7", got)
	}
	if fake.socketDomain != 2 || fake.socketType != 1 || fake.socketProto != 6 {
		t.Fatalf("backend args=%d,%d,%d", fake.socketDomain, fake.socketType, fake.socketProto)
	}
}

func TestArosHostSocketSendUsesSingleBulkRead(t *testing.T) {
	bus := NewMachineBus()
	fake := &fakeArosHostSocketBackend{}
	dev := NewArosHostSocketDevice(bus, fake, true)
	bus.MapIO(AROS_HOST_SOCKET_REGION_BASE, AROS_HOST_SOCKET_REGION_END, dev.HandleRead, dev.HandleWrite)
	payload := []byte("hello host socket")
	if err := WriteGuestBytes(bus, 0x2000, 0, payload); err != nil {
		t.Fatal(err)
	}
	words := make([]uint32, arosHostSocketReqWords)
	words[0] = 7
	words[3] = 0x2000
	words[4] = uint32(len(payload))
	writeArosHostSocketReq(t, bus, 0x1000, words...)
	bus.Write32(AROS_HOST_SOCKET_REQ_PTR, 0x1000)
	bus.Write32(AROS_HOST_SOCKET_REQ_LEN, arosHostSocketReqSize)
	bus.Write32(AROS_HOST_SOCKET_CMD, AROS_HOST_SOCKET_CMD_SENDTO)
	if fake.sendCalls != 1 {
		t.Fatalf("send calls=%d, want 1", fake.sendCalls)
	}
	if string(fake.sendData) != string(payload) {
		t.Fatalf("payload=%q, want %q", fake.sendData, payload)
	}
}

func TestArosHostSocketSendToRejectsInvalidDestination(t *testing.T) {
	bus := NewMachineBus()
	fake := &fakeArosHostSocketBackend{}
	dev := NewArosHostSocketDevice(bus, fake, true)
	bus.MapIO(AROS_HOST_SOCKET_REGION_BASE, AROS_HOST_SOCKET_REGION_END, dev.HandleRead, dev.HandleWrite)
	payload := []byte("hello host socket")
	if err := WriteGuestBytes(bus, 0x2000, 0, payload); err != nil {
		t.Fatal(err)
	}
	words := make([]uint32, arosHostSocketReqWords)
	words[0] = 7
	words[3] = 0x2000
	words[4] = uint32(len(payload))
	words[5] = 0
	words[6] = 16
	writeArosHostSocketReq(t, bus, 0x1000, words...)
	bus.Write32(AROS_HOST_SOCKET_REQ_PTR, 0x1000)
	bus.Write32(AROS_HOST_SOCKET_REQ_LEN, arosHostSocketReqSize)
	bus.Write32(AROS_HOST_SOCKET_CMD, AROS_HOST_SOCKET_CMD_SENDTO)
	if got := bus.Read32(AROS_HOST_SOCKET_ERRNO); got != arosSockErrInval {
		t.Fatalf("errno=%d, want EINVAL", got)
	}
	if fake.sendCalls != 0 {
		t.Fatalf("send calls=%d, want 0", fake.sendCalls)
	}
}

func TestArosHostSocketWaitSelectRejectsInvalidTimeoutPointer(t *testing.T) {
	bus := NewMachineBus()
	fake := &fakeArosHostSocketBackend{}
	dev := NewArosHostSocketDevice(bus, fake, true)
	bus.MapIO(AROS_HOST_SOCKET_REGION_BASE, AROS_HOST_SOCKET_REGION_END, dev.HandleRead, dev.HandleWrite)

	words := make([]uint32, arosHostSocketReqWords)
	words[10] = 0xfffffff0
	writeArosHostSocketReq(t, bus, 0x1000, words...)
	bus.Write32(AROS_HOST_SOCKET_REQ_PTR, 0x1000)
	bus.Write32(AROS_HOST_SOCKET_REQ_LEN, arosHostSocketReqSize)
	bus.Write32(AROS_HOST_SOCKET_CMD, AROS_HOST_SOCKET_CMD_WAITSELECT)
	if got := bus.Read32(AROS_HOST_SOCKET_ERRNO); got != arosSockErrInval {
		t.Fatalf("errno=%d, want EINVAL", got)
	}
	if fake.waitSelectCalls != 0 {
		t.Fatalf("waitselect calls=%d, want 0", fake.waitSelectCalls)
	}
}

func TestArosHostSocketWaitSelectRejectsInvalidFDSetPointers(t *testing.T) {
	for _, tc := range []struct {
		name string
		word int
	}{
		{name: "read", word: 3},
		{name: "write", word: 5},
		{name: "except", word: 14},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bus := NewMachineBus()
			fake := &fakeArosHostSocketBackend{}
			dev := NewArosHostSocketDevice(bus, fake, true)
			bus.MapIO(AROS_HOST_SOCKET_REGION_BASE, AROS_HOST_SOCKET_REGION_END, dev.HandleRead, dev.HandleWrite)

			words := make([]uint32, arosHostSocketReqWords)
			words[tc.word] = 0xfffffff0
			writeArosHostSocketReq(t, bus, 0x1000, words...)
			bus.Write32(AROS_HOST_SOCKET_REQ_PTR, 0x1000)
			bus.Write32(AROS_HOST_SOCKET_REQ_LEN, arosHostSocketReqSize)
			bus.Write32(AROS_HOST_SOCKET_CMD, AROS_HOST_SOCKET_CMD_WAITSELECT)
			if got := bus.Read32(AROS_HOST_SOCKET_ERRNO); got != arosSockErrInval {
				t.Fatalf("errno=%d, want EINVAL", got)
			}
			if fake.waitSelectCalls != 0 {
				t.Fatalf("waitselect calls=%d, want 0", fake.waitSelectCalls)
			}
		})
	}
}

func TestArosHostSocketRecvWritesGuestBuffer(t *testing.T) {
	bus := NewMachineBus()
	fake := &fakeArosHostSocketBackend{recvData: []byte("reply")}
	dev := NewArosHostSocketDevice(bus, fake, true)
	bus.MapIO(AROS_HOST_SOCKET_REGION_BASE, AROS_HOST_SOCKET_REGION_END, dev.HandleRead, dev.HandleWrite)
	words := make([]uint32, arosHostSocketReqWords)
	words[0] = 7
	words[3] = 0x3000
	words[4] = 16
	writeArosHostSocketReq(t, bus, 0x1000, words...)
	bus.Write32(AROS_HOST_SOCKET_REQ_PTR, 0x1000)
	bus.Write32(AROS_HOST_SOCKET_REQ_LEN, arosHostSocketReqSize)
	bus.Write32(AROS_HOST_SOCKET_CMD, AROS_HOST_SOCKET_CMD_RECVFROM)
	out := make([]byte, 5)
	if err := ReadGuestBytes(bus, 0x3000, 0, out); err != nil {
		t.Fatal(err)
	}
	if string(out) != "reply" {
		t.Fatalf("recv buffer=%q", out)
	}
}

func TestArosHostSocketDisabledReturnsStableError(t *testing.T) {
	bus := NewMachineBus()
	dev := NewArosHostSocketDevice(bus, &fakeArosHostSocketBackend{}, false)
	bus.MapIO(AROS_HOST_SOCKET_REGION_BASE, AROS_HOST_SOCKET_REGION_END, dev.HandleRead, dev.HandleWrite)
	writeArosHostSocketReq(t, bus, 0x1000, 2, 1, 6)
	bus.Write32(AROS_HOST_SOCKET_REQ_PTR, 0x1000)
	bus.Write32(AROS_HOST_SOCKET_REQ_LEN, arosHostSocketReqSize)
	bus.Write32(AROS_HOST_SOCKET_CMD, AROS_HOST_SOCKET_CMD_SOCKET)
	if got := bus.Read32(AROS_HOST_SOCKET_ERRNO); got != arosSockErrNoSys {
		t.Fatalf("errno=%d, want ENOSYS", got)
	}
}

func arosHostSocketRangesOverlap(a0, a1, b0, b1 uint32) bool {
	return a0 <= b1 && b0 <= a1
}
