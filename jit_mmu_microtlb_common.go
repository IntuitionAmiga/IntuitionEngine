// jit_mmu_microtlb_common.go - IE64 JIT helper micro-TLB state.

//go:build (amd64 && (linux || windows || darwin)) || (arm64 && (linux || windows || darwin))

package main

const (
	ie64MicroTLBValid       uint64 = 1 << 63
	ie64MicroTLBAccessRead  uint64 = 1 << 60
	ie64MicroTLBAccessWrite uint64 = 2 << 60
	ie64MicroTLBModeShift   uint   = 56
	ie64MicroTLBModeMask    uint64 = 0x0F << ie64MicroTLBModeShift
	ie64MicroTLBVPNMask     uint64 = PTE_PPN_MASK
)

func ie64MicroTLBModeBits(cpu *CPU64) uint64 {
	var mode uint64
	if cpu != nil && cpu.supervisorMode {
		mode |= 1
	}
	if cpu != nil && cpu.skac {
		mode |= 2
	}
	if cpu != nil && cpu.suaLatch {
		mode |= 4
	}
	if cpu != nil && cpu.skef {
		mode |= 8
	}
	return mode << ie64MicroTLBModeShift
}

func ie64MicroTLBPrefix(cpu *CPU64, access byte) uint64 {
	prefix := ie64MicroTLBValid | ie64MicroTLBModeBits(cpu)
	if access == ACCESS_WRITE {
		return prefix | ie64MicroTLBAccessWrite
	}
	return prefix | ie64MicroTLBAccessRead
}

func ie64MicroTLBKey(cpu *CPU64, vaddr uint64, access byte) uint64 {
	vpn := (vaddr >> MMU_PAGE_SHIFT) & ie64MicroTLBVPNMask
	return ie64MicroTLBPrefix(cpu, access) | vpn
}

func (ctx *JITContext) refreshMicroTLBPrefixes(cpu *CPU64) {
	if ctx == nil {
		return
	}
	ctx.MicroTLBReadPrefix = ie64MicroTLBPrefix(cpu, ACCESS_READ)
	ctx.MicroTLBWritePrefix = ie64MicroTLBPrefix(cpu, ACCESS_WRITE)
}

func (ctx *JITContext) flushMicroTLB() {
	if ctx == nil {
		return
	}
	for i := range ctx.MicroTLBKeys {
		ctx.MicroTLBKeys[i] = 0
		ctx.MicroTLBPhys[i] = 0
	}
}

func (ctx *JITContext) invalidateMicroTLBVPN(vpn uint64) {
	if ctx == nil {
		return
	}
	vpn &= ie64MicroTLBVPNMask
	for i, key := range ctx.MicroTLBKeys {
		if key&ie64MicroTLBVPNMask == vpn {
			ctx.MicroTLBKeys[i] = 0
			ctx.MicroTLBPhys[i] = 0
		}
	}
}

func (cpu *CPU64) fillJITMicroTLB(vaddr uint64, access byte) {
	if cpu == nil || cpu.jitCtx == nil || !cpu.mmuEnabled {
		return
	}
	phys, fault, _ := cpu.translateAddr(vaddr, access)
	if fault {
		return
	}
	if phys >= uint64(len(cpu.memory)) || phys >= uint64(IO_REGION_START) {
		return
	}
	vpn := (vaddr >> MMU_PAGE_SHIFT) & ie64MicroTLBVPNMask
	idx := vpn & (jitCtxMicroTLBEntries - 1)
	cpu.jitCtx.MicroTLBKeys[idx] = ie64MicroTLBKey(cpu, vaddr, access)
	cpu.jitCtx.MicroTLBPhys[idx] = phys & ^uint64(MMU_PAGE_MASK)
}
