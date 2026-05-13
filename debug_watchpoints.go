package main

type extendedWatchpointSetter interface {
	SetWatchpointEx(addr uint64, typ WatchpointType, width uint8) bool
}

func normalizeWatchWidth(width uint8) uint8 {
	switch width {
	case 1, 2, 4, 8:
		return width
	default:
		return 1
	}
}

func setWatchpointExLocked(cpu DebuggableCPU, watchpoints map[uint64]*Watchpoint, addr uint64, typ WatchpointType, width uint8) {
	width = normalizeWatchWidth(width)
	bytes := watchpointSnapshot(cpu, addr, width)
	var first byte
	if len(bytes) > 0 {
		first = bytes[0]
	}
	watchpoints[addr] = &Watchpoint{
		Type:      typ,
		Address:   addr,
		Width:     width,
		LastValue: first,
		LastBytes: bytes,
	}
}

func (d *Debug6502) readByte(addr uint64) byte { return d.cpu.memory.Read(uint16(addr)) }
func (d *DebugZ80) readByte(addr uint64) byte  { return d.cpu.bus.Read(uint16(addr)) }
func (d *DebugIE32) readByte(addr uint64) byte {
	if int(uint32(addr)) < len(d.cpu.memory) {
		return d.cpu.memory[uint32(addr)]
	}
	return 0
}
func (d *DebugX86) readByte(addr uint64) byte { return d.cpu.bus.Read(uint32(addr)) }

func (d *Debug6502) SetWatchpointEx(addr uint64, typ WatchpointType, width uint8) bool {
	d.bpMu.Lock()
	defer d.bpMu.Unlock()
	setWatchpointExLocked(d, d.watchpoints, addr, typ, width)
	return true
}

func (d *DebugZ80) SetWatchpointEx(addr uint64, typ WatchpointType, width uint8) bool {
	d.bpMu.Lock()
	defer d.bpMu.Unlock()
	setWatchpointExLocked(d, d.watchpoints, addr, typ, width)
	return true
}

func watchpointSnapshot(cpu DebuggableCPU, addr uint64, width uint8) []byte {
	width = normalizeWatchWidth(width)
	if cpu == nil {
		return nil
	}
	data := cpu.ReadMemory(addr, int(width))
	if len(data) < int(width) {
		return append([]byte(nil), data...)
	}
	return append([]byte(nil), data[:width]...)
}

func watchpointChanged(cpu DebuggableCPU, wp *Watchpoint) (bool, byte, byte, []byte) {
	if wp == nil {
		return false, 0, 0, nil
	}
	if wp.Type != WatchWrite {
		return false, 0, 0, nil
	}
	width := normalizeWatchWidth(wp.Width)
	cur := watchpointSnapshot(cpu, wp.Address, width)
	if len(cur) < int(width) {
		return false, 0, 0, nil
	}
	old := wp.LastBytes
	if len(old) != int(width) {
		old = make([]byte, width)
		old[0] = wp.LastValue
	}
	for i := 0; i < int(width); i++ {
		if old[i] != cur[i] {
			return true, old[i], cur[i], cur
		}
	}
	return false, 0, 0, nil
}

func (d *DebugM68K) SetWatchpointEx(addr uint64, typ WatchpointType, width uint8) bool {
	d.bpMu.Lock()
	defer d.bpMu.Unlock()
	setWatchpointExLocked(d, d.watchpoints, addr, typ, width)
	return true
}

func (d *DebugIE32) SetWatchpointEx(addr uint64, typ WatchpointType, width uint8) bool {
	d.bpMu.Lock()
	defer d.bpMu.Unlock()
	setWatchpointExLocked(d, d.watchpoints, addr, typ, width)
	return true
}

func (d *DebugIE64) SetWatchpointEx(addr uint64, typ WatchpointType, width uint8) bool {
	d.bpMu.Lock()
	defer d.bpMu.Unlock()
	setWatchpointExLocked(d, d.watchpoints, addr, typ, width)
	return true
}

func (d *DebugX86) SetWatchpointEx(addr uint64, typ WatchpointType, width uint8) bool {
	d.bpMu.Lock()
	defer d.bpMu.Unlock()
	setWatchpointExLocked(d, d.watchpoints, addr, typ, width)
	return true
}

func watchpointTypeString(typ WatchpointType) string {
	switch typ {
	case WatchRead:
		return "R"
	case WatchReadWrite:
		return "RW"
	default:
		return "W"
	}
}
