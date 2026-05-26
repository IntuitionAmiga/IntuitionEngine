// debug_ioview_read.go - native-width reads for IEMon I/O register views

package main

func readIORegisterFromBus(bus *MachineBus, addr uint64, width int) (uint32, bool) {
	if bus == nil || addr > 0xFFFFFFFF {
		return 0, false
	}
	a := uint32(addr)
	switch width {
	case 1:
		return uint32(bus.Read8(a)), true
	case 2:
		return uint32(bus.Read16(a)), true
	case 4:
		return bus.Read32(a), true
	default:
		return 0, false
	}
}

func (d *DebugIE64) ReadIORegister(addr uint64, width int) (uint32, bool) {
	if d == nil || d.cpu == nil {
		return 0, false
	}
	return readIORegisterFromBus(d.cpu.bus, addr, width)
}

func (d *DebugIE32) ReadIORegister(addr uint64, width int) (uint32, bool) {
	if d == nil || d.cpu == nil {
		return 0, false
	}
	bus, _ := d.cpu.bus.(*MachineBus)
	return readIORegisterFromBus(bus, addr, width)
}

func (d *DebugM68K) ReadIORegister(addr uint64, width int) (uint32, bool) {
	if d == nil || d.cpu == nil {
		return 0, false
	}
	bus, _ := d.cpu.bus.(*MachineBus)
	return readIORegisterFromBus(bus, addr, width)
}

func (d *DebugX86) ReadIORegister(addr uint64, width int) (uint32, bool) {
	if d == nil || d.cpu == nil {
		return 0, false
	}
	provider, _ := d.cpu.bus.(interface{ GetMachineBus() *MachineBus })
	if provider == nil {
		return 0, false
	}
	return readIORegisterFromBus(provider.GetMachineBus(), addr, width)
}

func (d *DebugZ80) ReadIORegister(addr uint64, width int) (uint32, bool) {
	if d == nil || d.cpu == nil {
		return 0, false
	}
	adapter, _ := d.cpu.bus.(*Z80BusAdapter)
	if adapter == nil {
		return 0, false
	}
	return readIORegisterFromBus(adapter.bus, addr, width)
}

func (d *Debug6502) ReadIORegister(addr uint64, width int) (uint32, bool) {
	if d == nil {
		return 0, false
	}
	if d.cpu != nil && d.cpu.fastAdapter != nil {
		if val, ok := readIORegisterFromBus(d.cpu.fastAdapter.machineBus, addr, width); ok {
			return val, true
		}
	}
	if d.runner != nil {
		return readIORegisterFromBus(d.runner.bus, addr, width)
	}
	return 0, false
}
