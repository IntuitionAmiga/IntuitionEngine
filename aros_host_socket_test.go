package main

import (
	"encoding/binary"
	"testing"
)

type fakeArosHostSocketBackend struct {
	socketDomain      int
	socketType        int
	socketProto       int
	sendCalls         int
	sendData          []byte
	recvData          []byte
	events            uint32
	waitSelectCalls   int
	releaseCalls      int
	releaseFD         int
	releaseID         int
	releaseCopyCalls  int
	releaseCopyFD     int
	releaseCopyID     int
	releaseResult     int
	releaseErr        uint32
	releaseCopyResult int
	releaseCopyErr    uint32
	obtainID          int
	obtainDomain      int
	obtainType        int
	obtainProto       int
	obtainFD          int
	obtainErr         uint32
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
func (f *fakeArosHostSocketBackend) Release(s int, id int) (int, uint32) {
	f.releaseCalls++
	f.releaseFD = s
	f.releaseID = id
	if f.releaseResult != 0 || f.releaseErr != 0 {
		return f.releaseResult, f.releaseErr
	}
	return id, 0
}
func (f *fakeArosHostSocketBackend) ReleaseCopy(s int, id int) (int, uint32) {
	f.releaseCopyCalls++
	f.releaseCopyFD = s
	f.releaseCopyID = id
	if f.releaseCopyResult != 0 || f.releaseCopyErr != 0 {
		return f.releaseCopyResult, f.releaseCopyErr
	}
	return id, 0
}
func (f *fakeArosHostSocketBackend) Obtain(id int, domain int, typ int, protocol int) (int, uint32) {
	f.obtainID = id
	f.obtainDomain = domain
	f.obtainType = typ
	f.obtainProto = protocol
	if f.obtainFD != 0 || f.obtainErr != 0 {
		return f.obtainFD, f.obtainErr
	}
	return 11, 0
}

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

func TestArosHostSocketGetSockOptWritesOptionLength(t *testing.T) {
	bus := NewMachineBus()
	dev := NewArosHostSocketDevice(bus, &fakeArosHostSocketBackend{}, true)
	bus.MapIO(AROS_HOST_SOCKET_REGION_BASE, AROS_HOST_SOCKET_REGION_END, dev.HandleRead, dev.HandleWrite)

	words := make([]uint32, arosHostSocketReqWords)
	words[0] = 7
	words[3] = 0x3000
	words[4] = 16
	words[5] = 0x3010
	writeArosHostSocketReq(t, bus, 0x1000, words...)
	bus.Write32(AROS_HOST_SOCKET_REQ_PTR, 0x1000)
	bus.Write32(AROS_HOST_SOCKET_REQ_LEN, arosHostSocketReqSize)
	bus.Write32(AROS_HOST_SOCKET_CMD, AROS_HOST_SOCKET_CMD_GETSOCKOPT)
	if got := bus.Read32(0x3010); got != 4 {
		t.Fatalf("option length=%d, want 4", got)
	}
}

func TestArosHostSocketGetHostNameWritesCString(t *testing.T) {
	bus := NewMachineBus()
	dev := NewArosHostSocketDevice(bus, &fakeArosHostSocketBackend{}, true)
	bus.MapIO(AROS_HOST_SOCKET_REGION_BASE, AROS_HOST_SOCKET_REGION_END, dev.HandleRead, dev.HandleWrite)

	words := make([]uint32, arosHostSocketReqWords)
	words[3] = 0x3000
	words[4] = 32
	writeArosHostSocketReq(t, bus, 0x1000, words...)
	bus.Write32(AROS_HOST_SOCKET_REQ_PTR, 0x1000)
	bus.Write32(AROS_HOST_SOCKET_REQ_LEN, arosHostSocketReqSize)
	bus.Write32(AROS_HOST_SOCKET_CMD, AROS_HOST_SOCKET_CMD_GETHOSTNAME)

	out := make([]byte, len("intuitionengine")+1)
	if err := ReadGuestBytes(bus, 0x3000, 0, out); err != nil {
		t.Fatal(err)
	}
	if string(out[:len(out)-1]) != "intuitionengine" || out[len(out)-1] != 0 {
		t.Fatalf("hostname bytes=%q", out)
	}
}

func TestArosHostSocketGetEventsWritesGuestPointer(t *testing.T) {
	bus := NewMachineBus()
	fake := &fakeArosHostSocketBackend{events: 0xa5}
	dev := NewArosHostSocketDevice(bus, fake, true)
	bus.MapIO(AROS_HOST_SOCKET_REGION_BASE, AROS_HOST_SOCKET_REGION_END, dev.HandleRead, dev.HandleWrite)

	words := make([]uint32, arosHostSocketReqWords)
	words[3] = 0x3000
	writeArosHostSocketReq(t, bus, 0x1000, words...)
	bus.Write32(AROS_HOST_SOCKET_REQ_PTR, 0x1000)
	bus.Write32(AROS_HOST_SOCKET_REQ_LEN, arosHostSocketReqSize)
	bus.Write32(AROS_HOST_SOCKET_CMD, AROS_HOST_SOCKET_CMD_GETEVENTS)
	if got := bus.Read32(AROS_HOST_SOCKET_EVENTS); got != 0xa5 {
		t.Fatalf("event register=%#x, want 0xa5", got)
	}
	if got := bus.Read32(0x3000); got != 0xa5 {
		t.Fatalf("event pointer=%#x, want 0xa5", got)
	}
}

func TestArosHostSocketReleasePassesDescriptorAndKey(t *testing.T) {
	bus := NewMachineBus()
	fake := &fakeArosHostSocketBackend{}
	dev := NewArosHostSocketDevice(bus, fake, true)
	bus.MapIO(AROS_HOST_SOCKET_REGION_BASE, AROS_HOST_SOCKET_REGION_END, dev.HandleRead, dev.HandleWrite)

	words := make([]uint32, arosHostSocketReqWords)
	words[0] = 7
	words[12] = 0x12345678
	writeArosHostSocketReq(t, bus, 0x1000, words...)
	bus.Write32(AROS_HOST_SOCKET_REQ_PTR, 0x1000)
	bus.Write32(AROS_HOST_SOCKET_REQ_LEN, arosHostSocketReqSize)
	bus.Write32(AROS_HOST_SOCKET_CMD, AROS_HOST_SOCKET_CMD_RELEASE)
	if fake.releaseCalls != 1 || fake.releaseFD != 7 || fake.releaseID != 0x12345678 {
		t.Fatalf("release calls=%d fd=%d id=%#x", fake.releaseCalls, fake.releaseFD, fake.releaseID)
	}
	if got := bus.Read32(AROS_HOST_SOCKET_RES1); got != 0x12345678 {
		t.Fatalf("release result=%#x, want release key", got)
	}
	if got := bus.Read32(AROS_HOST_SOCKET_ERRNO); got != 0 {
		t.Fatalf("errno=%d, want 0", got)
	}
}

func TestArosHostSocketReleaseCopyPassesDescriptorAndKey(t *testing.T) {
	bus := NewMachineBus()
	fake := &fakeArosHostSocketBackend{}
	dev := NewArosHostSocketDevice(bus, fake, true)
	bus.MapIO(AROS_HOST_SOCKET_REGION_BASE, AROS_HOST_SOCKET_REGION_END, dev.HandleRead, dev.HandleWrite)

	words := make([]uint32, arosHostSocketReqWords)
	words[0] = 8
	words[12] = 0x07654321
	writeArosHostSocketReq(t, bus, 0x1000, words...)
	bus.Write32(AROS_HOST_SOCKET_REQ_PTR, 0x1000)
	bus.Write32(AROS_HOST_SOCKET_REQ_LEN, arosHostSocketReqSize)
	bus.Write32(AROS_HOST_SOCKET_CMD, AROS_HOST_SOCKET_CMD_RELEASECOPY)
	if fake.releaseCopyCalls != 1 || fake.releaseCopyFD != 8 || fake.releaseCopyID != 0x07654321 {
		t.Fatalf("releasecopy calls=%d fd=%d id=%#x", fake.releaseCopyCalls, fake.releaseCopyFD, fake.releaseCopyID)
	}
	if got := bus.Read32(AROS_HOST_SOCKET_RES1); got != 0x07654321 {
		t.Fatalf("releasecopy result=%#x, want release key", got)
	}
	if got := bus.Read32(AROS_HOST_SOCKET_ERRNO); got != 0 {
		t.Fatalf("errno=%d, want 0", got)
	}
}

func TestArosHostSocketReleaseReturnsGeneratedKey(t *testing.T) {
	bus := NewMachineBus()
	fake := &fakeArosHostSocketBackend{releaseResult: 0x345678}
	dev := NewArosHostSocketDevice(bus, fake, true)
	bus.MapIO(AROS_HOST_SOCKET_REGION_BASE, AROS_HOST_SOCKET_REGION_END, dev.HandleRead, dev.HandleWrite)

	words := make([]uint32, arosHostSocketReqWords)
	words[0] = 7
	words[12] = ^uint32(0)
	writeArosHostSocketReq(t, bus, 0x1000, words...)
	bus.Write32(AROS_HOST_SOCKET_REQ_PTR, 0x1000)
	bus.Write32(AROS_HOST_SOCKET_REQ_LEN, arosHostSocketReqSize)
	bus.Write32(AROS_HOST_SOCKET_CMD, AROS_HOST_SOCKET_CMD_RELEASE)
	if fake.releaseID != -1 {
		t.Fatalf("release id=%d, want UNIQUE_ID -1", fake.releaseID)
	}
	if got := bus.Read32(AROS_HOST_SOCKET_RES1); got != 0x345678 {
		t.Fatalf("generated release key=%#x, want 0x345678", got)
	}
}

func TestArosHostSocketObtainUsesReleaseKey(t *testing.T) {
	bus := NewMachineBus()
	fake := &fakeArosHostSocketBackend{obtainFD: 42}
	dev := NewArosHostSocketDevice(bus, fake, true)
	bus.MapIO(AROS_HOST_SOCKET_REGION_BASE, AROS_HOST_SOCKET_REGION_END, dev.HandleRead, dev.HandleWrite)

	words := make([]uint32, arosHostSocketReqWords)
	words[0] = 2
	words[1] = 1
	words[2] = 6
	words[12] = 0x12345678
	writeArosHostSocketReq(t, bus, 0x1000, words...)
	bus.Write32(AROS_HOST_SOCKET_REQ_PTR, 0x1000)
	bus.Write32(AROS_HOST_SOCKET_REQ_LEN, arosHostSocketReqSize)
	bus.Write32(AROS_HOST_SOCKET_CMD, AROS_HOST_SOCKET_CMD_OBTAIN)
	if got := bus.Read32(AROS_HOST_SOCKET_RES1); got != 42 {
		t.Fatalf("obtain result=%d, want 42", got)
	}
	if fake.obtainID != 0x12345678 || fake.obtainDomain != 2 || fake.obtainType != 1 || fake.obtainProto != 6 {
		t.Fatalf("obtain args id=%#x domain=%d type=%d proto=%d", fake.obtainID, fake.obtainDomain, fake.obtainType, fake.obtainProto)
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
