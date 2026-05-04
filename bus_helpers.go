package main

import "errors"

var (
	ErrSeamCrossing     = errors.New("guest read crosses low/high memory seam")
	ErrAddrOutOfRange   = errors.New("guest read address out of range")
	ErrHIPtrUnsupported = errors.New("high pointer unsupported by 32-bit bus")
)

type backingProvider interface {
	Backing() Backing
}

type activeRAMReporter interface {
	ActiveVisibleRAM() uint64
}

// ReadGuestBytes is a strict bulk-copy helper for media loaders. It reads only
// regular guest RAM, rejects seam-crossing spans, and never accepts Backing's
// out-of-range zero-fill behavior as valid media bytes.
func ReadGuestBytes(bus Bus32, ptrLo, ptrHi uint32, dst []byte) error {
	if bus == nil {
		return ErrAddrOutOfRange
	}
	addr := uint64(ptrHi)<<32 | uint64(ptrLo)
	length := uint64(len(dst))
	end := addr + length
	if end < addr {
		return ErrAddrOutOfRange
	}
	mem := bus.GetMemory()
	lowEnd := uint64(len(mem))

	if bp, ok := bus.(backingProvider); ok && bp.Backing() != nil {
		backing := bp.Backing()
		effectiveTop := backing.Size()
		if ar, ok := bus.(activeRAMReporter); ok && ar.ActiveVisibleRAM() != 0 && ar.ActiveVisibleRAM() < effectiveTop {
			effectiveTop = ar.ActiveVisibleRAM()
		}
		if end <= lowEnd {
			if end > effectiveTop {
				return ErrAddrOutOfRange
			}
			copy(dst, mem[addr:end])
			return nil
		}
		if addr < lowEnd && end > lowEnd {
			return ErrSeamCrossing
		}
		if addr >= lowEnd && end <= effectiveTop {
			backing.ReadBytes(addr, dst)
			return nil
		}
		return ErrAddrOutOfRange
	}

	if ptrHi != 0 {
		return ErrHIPtrUnsupported
	}
	if end > lowEnd {
		return ErrAddrOutOfRange
	}
	copy(dst, mem[addr:end])
	return nil
}

func WriteGuestBytes(bus Bus32, ptrLo, ptrHi uint32, src []byte) error {
	if bus == nil {
		return ErrAddrOutOfRange
	}
	addr := uint64(ptrHi)<<32 | uint64(ptrLo)
	length := uint64(len(src))
	end := addr + length
	if end < addr {
		return ErrAddrOutOfRange
	}
	mem := bus.GetMemory()
	lowEnd := uint64(len(mem))

	if bp, ok := bus.(backingProvider); ok && bp.Backing() != nil {
		backing := bp.Backing()
		effectiveTop := backing.Size()
		if ar, ok := bus.(activeRAMReporter); ok && ar.ActiveVisibleRAM() != 0 && ar.ActiveVisibleRAM() < effectiveTop {
			effectiveTop = ar.ActiveVisibleRAM()
		}
		if end <= lowEnd {
			if end > effectiveTop {
				return ErrAddrOutOfRange
			}
			copy(mem[addr:end], src)
			return nil
		}
		if addr < lowEnd && end > lowEnd {
			return ErrSeamCrossing
		}
		if addr >= lowEnd && end <= effectiveTop {
			backing.WriteBytes(addr, src)
			return nil
		}
		return ErrAddrOutOfRange
	}

	if ptrHi != 0 {
		return ErrHIPtrUnsupported
	}
	if end > lowEnd {
		return ErrAddrOutOfRange
	}
	copy(mem[addr:end], src)
	return nil
}
