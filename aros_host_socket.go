package main

import (
	"encoding/binary"
	"net"
	"strings"
)

type arosHostSocketBackend interface {
	Socket(domain, typ, protocol int) (int, uint32)
	Bind(s int, name []byte) uint32
	Listen(s int, backlog int) uint32
	Accept(s int, addrCap int) (int, []byte, uint32)
	Connect(s int, name []byte) uint32
	SendTo(s int, data []byte, flags int, to []byte) (int, uint32)
	RecvFrom(s int, length int, flags int, fromCap int) ([]byte, []byte, uint32)
	Shutdown(s int, how int) uint32
	SetSockOpt(s int, level int, optname int, opt []byte) uint32
	GetSockOpt(s int, level int, optname int, optCap int) ([]byte, uint32)
	GetSockName(s int, cap int) ([]byte, uint32)
	GetPeerName(s int, cap int) ([]byte, uint32)
	Ioctl(s int, request uint32, argp uint32) uint32
	Close(s int) uint32
	WaitSelect(nfds int, readfds, writefds, exceptfds []byte, timeout *arosHostSocketTimeval, sigmaskPtr uint32) (int, []byte, []byte, []byte, uint32)
	Dup2(fd1, fd2 int) (int, uint32)
	GetEvents() uint32
	Release(s int, id int) (int, uint32)
	ReleaseCopy(s int, id int) (int, uint32)
	Obtain(id int, domain int, typ int, protocol int) (int, uint32)
}

type ArosHostSocketDevice struct {
	bus     *MachineBus
	backend arosHostSocketBackend
	enabled bool

	reqPtr uint32
	reqLen uint32
	res1   uint32
	res2   uint32
	errno  uint32
	herrno uint32
	status uint32
	events uint32
}

func NewArosHostSocketDevice(bus *MachineBus, backend arosHostSocketBackend, enabled bool) *ArosHostSocketDevice {
	if backend == nil {
		backend = &disabledArosHostSocketBackend{}
	}
	return &ArosHostSocketDevice{bus: bus, backend: backend, enabled: enabled}
}

func (d *ArosHostSocketDevice) HandleRead(addr uint32) uint32 {
	switch addr {
	case AROS_HOST_SOCKET_REQ_PTR:
		return d.reqPtr
	case AROS_HOST_SOCKET_REQ_LEN:
		return d.reqLen
	case AROS_HOST_SOCKET_RES1:
		return d.res1
	case AROS_HOST_SOCKET_RES2:
		return d.res2
	case AROS_HOST_SOCKET_ERRNO:
		return d.errno
	case AROS_HOST_SOCKET_HERRNO:
		return d.herrno
	case AROS_HOST_SOCKET_STATUS:
		return d.status
	case AROS_HOST_SOCKET_EVENTS:
		return d.events
	default:
		return 0
	}
}

func (d *ArosHostSocketDevice) HandleWrite(addr uint32, val uint32) {
	switch addr {
	case AROS_HOST_SOCKET_REQ_PTR:
		d.reqPtr = val
	case AROS_HOST_SOCKET_REQ_LEN:
		d.reqLen = val
	case AROS_HOST_SOCKET_CMD:
		d.dispatch(val)
	}
}

func (d *ArosHostSocketDevice) dispatch(cmd uint32) {
	d.status = arosHostSocketStatusReady
	d.res1 = ^uint32(0)
	d.res2 = 0
	d.errno = 0
	d.herrno = 0

	if !d.enabled {
		d.fail(arosSockErrNoSys)
		return
	}

	req, ok := d.readReq()
	if !ok {
		d.fail(arosSockErrInval)
		return
	}

	switch cmd {
	case AROS_HOST_SOCKET_CMD_SOCKET:
		fd, errno := d.backend.Socket(int(req[0]), int(req[1]), int(req[2]))
		d.finishInt(fd, errno)
	case AROS_HOST_SOCKET_CMD_BIND:
		d.finishErr(d.backend.Bind(int(req[0]), d.readBounded(req[3], req[4], arosHostSocketMaxAddr)))
	case AROS_HOST_SOCKET_CMD_LISTEN:
		d.finishErr(d.backend.Listen(int(req[0]), int(req[12])))
	case AROS_HOST_SOCKET_CMD_ACCEPT:
		fd, addr, errno := d.backend.Accept(int(req[0]), int(req[4]))
		if errno == 0 && req[3] != 0 && len(addr) != 0 {
			if !d.writeBytes(req[3], addr) {
				errno = arosSockErrInval
			}
		}
		if errno == 0 && req[5] != 0 {
			d.bus.Write32(req[5], uint32(len(addr)))
		}
		d.finishInt(fd, errno)
	case AROS_HOST_SOCKET_CMD_CONNECT:
		d.finishErr(d.backend.Connect(int(req[0]), d.readBounded(req[3], req[4], arosHostSocketMaxAddr)))
	case AROS_HOST_SOCKET_CMD_SENDTO:
		data := d.readBounded(req[3], req[4], arosHostSocketMaxIO)
		if req[4] > 0 && data == nil {
			d.fail(arosSockErrInval)
			return
		}
		var to []byte
		if req[5] != 0 || req[6] != 0 {
			to = d.readBounded(req[5], req[6], arosHostSocketMaxAddr)
			if to == nil {
				d.fail(arosSockErrInval)
				return
			}
		}
		n, errno := d.backend.SendTo(int(req[0]), data, int(req[7]), to)
		d.finishInt(n, errno)
	case AROS_HOST_SOCKET_CMD_RECVFROM:
		if req[4] > arosHostSocketMaxIO {
			d.fail(arosSockErrMsgSize)
			return
		}
		data, from, errno := d.backend.RecvFrom(int(req[0]), int(req[4]), int(req[7]), int(req[6]))
		if errno == 0 && len(data) != 0 && !d.writeBytes(req[3], data) {
			errno = arosSockErrInval
		}
		if errno == 0 && req[5] != 0 && len(from) != 0 && !d.writeBytes(req[5], from) {
			errno = arosSockErrInval
		}
		if errno == 0 && req[12] != 0 {
			d.bus.Write32(req[12], uint32(len(from)))
		}
		d.finishInt(len(data), errno)
	case AROS_HOST_SOCKET_CMD_SHUTDOWN:
		d.finishErr(d.backend.Shutdown(int(req[0]), int(req[12])))
	case AROS_HOST_SOCKET_CMD_SETSOCKOPT:
		d.finishErr(d.backend.SetSockOpt(int(req[0]), int(req[8]), int(req[9]), d.readBounded(req[3], req[4], arosHostSocketMaxAddr)))
	case AROS_HOST_SOCKET_CMD_GETSOCKOPT:
		opt, errno := d.backend.GetSockOpt(int(req[0]), int(req[8]), int(req[9]), int(req[4]))
		if errno == 0 && len(opt) != 0 && !d.writeBytes(req[3], opt) {
			errno = arosSockErrInval
		}
		if errno == 0 && req[5] != 0 {
			d.bus.Write32(req[5], uint32(len(opt)))
		}
		d.finishErr(errno)
	case AROS_HOST_SOCKET_CMD_GETSOCKNAME, AROS_HOST_SOCKET_CMD_GETPEERNAME:
		var addr []byte
		var errno uint32
		if cmd == AROS_HOST_SOCKET_CMD_GETSOCKNAME {
			addr, errno = d.backend.GetSockName(int(req[0]), int(req[4]))
		} else {
			addr, errno = d.backend.GetPeerName(int(req[0]), int(req[4]))
		}
		if errno == 0 && len(addr) != 0 && !d.writeBytes(req[3], addr) {
			errno = arosSockErrInval
		}
		if errno == 0 && req[5] != 0 {
			d.bus.Write32(req[5], uint32(len(addr)))
		}
		d.finishErr(errno)
	case AROS_HOST_SOCKET_CMD_IOCTL:
		d.finishErr(d.backend.Ioctl(int(req[0]), req[12], req[3]))
	case AROS_HOST_SOCKET_CMD_CLOSE:
		d.finishErr(d.backend.Close(int(req[0])))
	case AROS_HOST_SOCKET_CMD_WAITSELECT:
		rfIn, rfOK := d.readFDSet(req[3])
		wfIn, wfOK := d.readFDSet(req[5])
		efIn, efOK := d.readFDSet(req[14])
		tv, tvOK := d.readTimeval(req[10])
		if !rfOK || !wfOK || !efOK || !tvOK {
			d.finishErr(arosSockErrInval)
			break
		}
		n, rf, wf, ef, errno := d.backend.WaitSelect(int(req[12]), rfIn, wfIn, efIn, tv, req[11])
		if errno == 0 {
			d.writeOptional(req[3], rf)
			d.writeOptional(req[5], wf)
			d.writeOptional(req[14], ef)
		}
		d.finishInt(n, errno)
	case AROS_HOST_SOCKET_CMD_GETHOSTBYNAME:
		d.resolveHostByName(req)
	case AROS_HOST_SOCKET_CMD_GETHOSTBYADDR:
		d.resolveHostByAddr(req)
	case AROS_HOST_SOCKET_CMD_GETHOSTNAME:
		d.getHostName(req)
	case AROS_HOST_SOCKET_CMD_DUP2:
		fd, errno := d.backend.Dup2(arosHostSocketSigned(req[12]), arosHostSocketSigned(req[13]))
		d.finishInt(fd, errno)
	case AROS_HOST_SOCKET_CMD_GETEVENTS:
		d.events = d.backend.GetEvents()
		if req[3] != 0 {
			d.bus.Write32(req[3], d.events)
		}
		d.res1 = 0
	case AROS_HOST_SOCKET_CMD_RELEASE:
		id, errno := d.backend.Release(int(req[0]), arosHostSocketSigned(req[12]))
		d.finishInt(id, errno)
	case AROS_HOST_SOCKET_CMD_RELEASECOPY:
		id, errno := d.backend.ReleaseCopy(int(req[0]), arosHostSocketSigned(req[12]))
		d.finishInt(id, errno)
	case AROS_HOST_SOCKET_CMD_OBTAIN:
		fd, errno := d.backend.Obtain(arosHostSocketSigned(req[12]), int(req[0]), int(req[1]), int(req[2]))
		d.finishInt(fd, errno)
	default:
		d.fail(arosSockErrOpNotSupp)
	}
}

func arosHostSocketSigned(v uint32) int {
	return int(int32(v))
}

func (d *ArosHostSocketDevice) readReq() ([arosHostSocketReqWords]uint32, bool) {
	var out [arosHostSocketReqWords]uint32
	if d.reqLen < arosHostSocketReqSize {
		return out, false
	}
	raw := make([]byte, arosHostSocketReqSize)
	if err := ReadGuestBytes(d.bus, d.reqPtr, 0, raw); err != nil {
		return out, false
	}
	for i := range out {
		out[i] = binary.BigEndian.Uint32(raw[i*4:])
	}
	return out, true
}

func (d *ArosHostSocketDevice) readBounded(ptr, length, max uint32) []byte {
	if length == 0 {
		return nil
	}
	if ptr == 0 || length > max {
		return nil
	}
	b := make([]byte, length)
	if err := ReadGuestBytes(d.bus, ptr, 0, b); err != nil {
		return nil
	}
	return b
}

func (d *ArosHostSocketDevice) writeBytes(ptr uint32, b []byte) bool {
	if ptr == 0 && len(b) != 0 {
		return false
	}
	return WriteGuestBytes(d.bus, ptr, 0, b) == nil
}

func (d *ArosHostSocketDevice) writeOptional(ptr uint32, b []byte) {
	if ptr != 0 && b != nil {
		_ = WriteGuestBytes(d.bus, ptr, 0, b)
	}
}

func (d *ArosHostSocketDevice) readFDSet(ptr uint32) ([]byte, bool) {
	if ptr == 0 {
		return nil, true
	}
	b := make([]byte, arosHostSocketFDSetLen)
	if err := ReadGuestBytes(d.bus, ptr, 0, b); err != nil {
		return nil, false
	}
	return b, true
}

func (d *ArosHostSocketDevice) readTimeval(ptr uint32) (*arosHostSocketTimeval, bool) {
	if ptr == 0 {
		return nil, true
	}
	b := make([]byte, 8)
	if err := ReadGuestBytes(d.bus, ptr, 0, b); err != nil {
		return nil, false
	}
	return &arosHostSocketTimeval{
		Sec:  int32(binary.BigEndian.Uint32(b[0:4])),
		Usec: int32(binary.BigEndian.Uint32(b[4:8])),
	}, true
}

func (d *ArosHostSocketDevice) finishErr(errno uint32) {
	if errno != 0 {
		d.fail(errno)
		return
	}
	d.res1 = 0
	d.errno = 0
}

func (d *ArosHostSocketDevice) finishInt(v int, errno uint32) {
	if errno != 0 {
		d.fail(errno)
		return
	}
	d.res1 = uint32(int32(v))
	d.errno = 0
}

func (d *ArosHostSocketDevice) fail(errno uint32) {
	d.res1 = ^uint32(0)
	d.errno = errno
	d.status = arosHostSocketStatusError
}

func (d *ArosHostSocketDevice) readCString(ptr uint32, limit int) (string, bool) {
	if ptr == 0 || limit <= 0 {
		return "", false
	}
	b := make([]byte, 0, limit)
	for i := 0; i < limit; i++ {
		v := d.bus.Read8(ptr + uint32(i))
		if v == 0 {
			return string(b), true
		}
		b = append(b, v)
	}
	return "", false
}

func (d *ArosHostSocketDevice) writeCString(ptr, cap uint32, s string) bool {
	if ptr == 0 || cap == 0 {
		return false
	}
	if uint32(len(s)+1) > cap {
		s = s[:cap-1]
	}
	b := append([]byte(s), 0)
	return d.writeBytes(ptr, b)
}

func (d *ArosHostSocketDevice) resolveHostByName(req [arosHostSocketReqWords]uint32) {
	name, ok := d.readCString(req[3], 255)
	if !ok {
		d.herrno = arosSockHHostNotFound
		d.fail(arosSockErrInval)
		return
	}
	ips, err := net.LookupIP(name)
	if err != nil {
		d.herrno = arosSockHHostNotFound
		d.fail(arosSockErrNone)
		return
	}
	d.writeHostent(req, name, ips)
}

func (d *ArosHostSocketDevice) resolveHostByAddr(req [arosHostSocketReqWords]uint32) {
	addr := d.readBounded(req[3], req[4], 4)
	if len(addr) != 4 {
		d.herrno = arosSockHHostNotFound
		d.fail(arosSockErrInval)
		return
	}
	names, err := net.LookupAddr(net.IPv4(addr[0], addr[1], addr[2], addr[3]).String())
	if err != nil || len(names) == 0 {
		d.herrno = arosSockHHostNotFound
		d.fail(arosSockErrNone)
		return
	}
	d.writeHostent(req, strings.TrimSuffix(names[0], "."), []net.IP{net.IPv4(addr[0], addr[1], addr[2], addr[3])})
}

func (d *ArosHostSocketDevice) writeHostent(req [arosHostSocketReqWords]uint32, name string, ips []net.IP) {
	if !d.writeCString(req[16], req[17], name) {
		d.fail(arosSockErrInval)
		return
	}
	max := int(req[19])
	if max > len(ips) {
		max = len(ips)
	}
	if max > 0 {
		out := make([]byte, 0, max*4)
		for _, ip := range ips {
			v4 := ip.To4()
			if v4 == nil {
				continue
			}
			out = append(out, v4...)
			if len(out)/4 == max {
				break
			}
		}
		if len(out) == 0 || !d.writeBytes(req[18], out) {
			d.herrno = arosSockHHostNotFound
			d.fail(arosSockErrNone)
			return
		}
	}
	d.res1 = 0
	d.errno = 0
	d.herrno = 0
}

func (d *ArosHostSocketDevice) getHostName(req [arosHostSocketReqWords]uint32) {
	name := "intuitionengine"
	if !d.writeCString(req[3], req[4], name) {
		d.fail(arosSockErrInval)
		return
	}
	d.res1 = 0
}

type disabledArosHostSocketBackend struct{}

func (disabledArosHostSocketBackend) Socket(int, int, int) (int, uint32) { return -1, arosSockErrNoSys }
func (disabledArosHostSocketBackend) Bind(int, []byte) uint32            { return arosSockErrNoSys }
func (disabledArosHostSocketBackend) Listen(int, int) uint32             { return arosSockErrNoSys }
func (disabledArosHostSocketBackend) Accept(int, int) (int, []byte, uint32) {
	return -1, nil, arosSockErrNoSys
}
func (disabledArosHostSocketBackend) Connect(int, []byte) uint32 { return arosSockErrNoSys }
func (disabledArosHostSocketBackend) SendTo(int, []byte, int, []byte) (int, uint32) {
	return -1, arosSockErrNoSys
}
func (disabledArosHostSocketBackend) RecvFrom(int, int, int, int) ([]byte, []byte, uint32) {
	return nil, nil, arosSockErrNoSys
}
func (disabledArosHostSocketBackend) Shutdown(int, int) uint32 { return arosSockErrNoSys }
func (disabledArosHostSocketBackend) SetSockOpt(int, int, int, []byte) uint32 {
	return arosSockErrNoSys
}
func (disabledArosHostSocketBackend) GetSockOpt(int, int, int, int) ([]byte, uint32) {
	return nil, arosSockErrNoSys
}
func (disabledArosHostSocketBackend) GetSockName(int, int) ([]byte, uint32) {
	return nil, arosSockErrNoSys
}
func (disabledArosHostSocketBackend) GetPeerName(int, int) ([]byte, uint32) {
	return nil, arosSockErrNoSys
}
func (disabledArosHostSocketBackend) Ioctl(int, uint32, uint32) uint32 { return arosSockErrNoSys }
func (disabledArosHostSocketBackend) Close(int) uint32                 { return arosSockErrNoSys }
func (disabledArosHostSocketBackend) WaitSelect(int, []byte, []byte, []byte, *arosHostSocketTimeval, uint32) (int, []byte, []byte, []byte, uint32) {
	return -1, nil, nil, nil, arosSockErrNoSys
}
func (disabledArosHostSocketBackend) Dup2(int, int) (int, uint32) { return -1, arosSockErrNoSys }
func (disabledArosHostSocketBackend) GetEvents() uint32           { return 0 }
func (disabledArosHostSocketBackend) Release(int, int) (int, uint32) {
	return -1, arosSockErrNoSys
}
func (disabledArosHostSocketBackend) ReleaseCopy(int, int) (int, uint32) {
	return -1, arosSockErrNoSys
}
func (disabledArosHostSocketBackend) Obtain(int, int, int, int) (int, uint32) {
	return -1, arosSockErrNoSys
}
