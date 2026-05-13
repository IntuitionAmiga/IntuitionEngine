package main

func (d *DebugIE64) accessDebugActive() bool {
	return d != nil && d.cpu != nil && d.cpu.debugAccess != nil && d.cpu.debugAccess.AnyActive(d.cpu.debugCPUID)
}

func (d *DebugIE32) accessDebugActive() bool {
	return d != nil && d.cpu != nil && d.cpu.debugAccess != nil && d.cpu.debugAccess.AnyActive(d.cpu.debugCPUID)
}

func (d *DebugM68K) accessDebugActive() bool {
	return d != nil && d.cpu != nil && d.cpu.debugAccess != nil && d.cpu.debugAccess.AnyActive(d.cpu.debugCPUID)
}

func (d *Debug6502) accessDebugActive() bool {
	if d == nil || d.cpu == nil || d.cpu.fastAdapter == nil || d.cpu.fastAdapter.machineBus == nil {
		return false
	}
	access := d.cpu.fastAdapter.machineBus.debugAccess
	return access != nil && access.AnyActive(d.cpu.fastAdapter.debugCPUID)
}

func (d *DebugZ80) accessDebugActive() bool {
	if d == nil || d.cpu == nil {
		return false
	}
	adapter, ok := d.cpu.bus.(*Z80BusAdapter)
	return ok && adapter != nil && adapter.debugAccess != nil && adapter.debugAccess.AnyActive(adapter.debugCPUID)
}

func (d *DebugX86) accessDebugActive() bool {
	if d == nil || d.cpu == nil {
		return false
	}
	adapter, ok := d.cpu.bus.(*X86BusAdapter)
	return ok && adapter != nil && adapter.debugAccess != nil && adapter.debugAccess.AnyActive(adapter.debugCPUID)
}
