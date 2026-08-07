package main

// z80CanonicalHelperPayload is an immutable, backend-neutral instruction
// image. A helper executes from Bytes, never by fetching the mutable guest
// instruction stream again.
type z80CanonicalHelperPayload struct {
	ExitReason   uint32
	StartPC      uint16
	Prefix       byte
	Opcode       byte
	Operand      uint16
	Displacement int8
	Length       byte
	ResumePC     uint16
	Retired      uint32
	Cycles       uint64
	RIncrements  byte
	Bytes        [4]byte
	// FrozenFetch contains the complete instruction-fetch image when the
	// instruction begins with repeated DD/FD prefixes. Bytes remains the
	// fixed ABI image for normal instructions; the extended image prevents a
	// helper from falling back to mutable guest code for a valid prefix train.
	FrozenFetch []byte
}

// z80CanonicalHelperPayloadComplete reports whether the fixed four-byte image
// contains every byte CPU_Z80.Step can fetch for this instruction. Repeated
// index prefixes and an index prefix followed by ED can consume more bytes;
// execute those through the ordinary interpreter instead of a partial helper.
func z80CanonicalHelperPayloadComplete(payload z80CanonicalHelperPayload) bool {
	if len(payload.FrozenFetch) != 0 {
		return true
	}
	if payload.Prefix != 0xDD && payload.Prefix != 0xFD {
		return true
	}
	switch payload.Bytes[1] {
	case 0xDD, 0xFD, 0xED:
		return false
	default:
		return true
	}
}

// z80FrontendScanBlock is the untagged decoder/admission frontend shared by
// browser and native region formation. It owns bounded fetch, prefix length,
// direct-page rejection and terminator policy; a backend supplies only its
// lowering-admission predicate.
func z80FrontendScanBlock(fetch func(uint16) byte, direct func(uint16) bool, admit func(z80CanonicalHelperPayload) bool, startPC uint16) []z80CanonicalHelperPayload {
	if fetch == nil || direct == nil || admit == nil || !direct(startPC) {
		return nil
	}
	result := make([]z80CanonicalHelperPayload, 0, 32)
	pc := startPC
	for len(result) < 128 {
		var bytes [4]byte
		for i := range bytes {
			bytes[i] = fetch(pc + uint16(i))
		}
		payload := z80CanonicalPayloadFromBytes(pc, bytes)
		if payload.Length == 0 || !admit(payload) {
			break
		}
		for i := uint16(0); i < uint16(payload.Length); i++ {
			if !direct(pc + i) {
				return result
			}
		}
		result = append(result, payload)
		if _, _, terminates := z80FrontendStaticTarget(payload); terminates {
			break
		}
		pc = payload.ResumePC
	}
	return result
}

// z80FrontendRegionPlan is the backend-neutral static-chain scanner. The
// caller supplies architectural byte fetch, direct-page admission and backend
// lowering admission; all prefix length, static-target and four-block/128
// instruction policy remains common to native and wasm dispatchers.
//
// A returned final block may end at a non-static observation boundary. Earlier
// members are connected only by unconditional JP/JR edges. Returning nil means
// that the start is not a promotable region.
func z80FrontendRegionPlan(fetch func(uint16) byte, direct func(uint16) bool, admit func(z80CanonicalHelperPayload) bool, startPC uint16) []uint16 {
	if fetch == nil || direct == nil || admit == nil || !direct(startPC) {
		return nil
	}
	pcs := make([]uint16, 0, 4)
	seen := map[uint16]struct{}{}
	pc := startPC
	total := 0
	for len(pcs) < 4 && total < 128 {
		if _, duplicate := seen[pc]; duplicate || !direct(pc) {
			break
		}
		blockPC := pc
		blockInstructions := 0
		var next uint16
		staticNext := false
		for total < 128 {
			var bytes [4]byte
			for i := range bytes {
				bytes[i] = fetch(pc + uint16(i))
			}
			payload := z80CanonicalPayloadFromBytes(pc, bytes)
			if payload.Length == 0 || !admit(payload) {
				break
			}
			for i := uint16(0); i < uint16(payload.Length); i++ {
				if !direct(pc + i) {
					return nil
				}
			}
			total++
			blockInstructions++
			if target, ok, terminates := z80FrontendStaticTarget(payload); terminates {
				next, staticNext = target, ok
				break
			}
			pc = payload.ResumePC
		}
		if blockInstructions == 0 {
			break
		}
		seen[blockPC] = struct{}{}
		pcs = append(pcs, blockPC)
		if !staticNext {
			break
		}
		pc = next
	}
	if len(pcs) < 2 {
		return nil
	}
	return pcs
}

// z80FrontendStaticTarget classifies every block boundary, returning a target
// only for the unconditional JP/JR edges that may join a promoted region.
func z80FrontendStaticTarget(payload z80CanonicalHelperPayload) (target uint16, static, terminates bool) {
	if payload.Prefix != 0 {
		switch payload.Prefix {
		case 0xDD, 0xFD:
			return 0, false, payload.Opcode == 0xE9 // JP (IX/IY)
		case 0xED:
			switch payload.Opcode {
			case 0x4D, 0x45, 0x55, 0x5D, 0x65, 0x6D, 0x75, 0x7D, 0xB0, 0xB8, 0xB1, 0xB9:
				return 0, false, true
			}
		}
		return 0, false, false
	}
	switch op := payload.Opcode; op {
	case 0xC3:
		return payload.Operand, true, true
	case 0x18:
		return payload.ResumePC + uint16(int16(int8(payload.Operand))), true, true
	case 0xC9, 0xE9, 0x76, 0xCD, 0xFB, 0xF3:
		return 0, false, true
	}
	if payload.Opcode&0xC7 == 0xC2 || payload.Opcode&0xC7 == 0xC4 || payload.Opcode&0xC7 == 0xC0 || payload.Opcode&0xC7 == 0xC7 {
		return 0, false, true
	}
	switch payload.Opcode {
	case 0x10, 0x20, 0x28, 0x30, 0x38:
		return 0, false, true
	}
	return 0, false, false
}

// z80CanonicalPayloadFromBytes decodes the fixed four-byte image used by a
// helper exit. It is independent of the native scanner so wasm has the same
// ABI metadata without consulting mutable guest memory.
func z80CanonicalPayloadFromBytes(pc uint16, bytes [4]byte) z80CanonicalHelperPayload {
	payload := z80CanonicalHelperPayload{StartPC: pc, Opcode: bytes[0], Length: 1, ResumePC: pc + 1, RIncrements: 1, Bytes: bytes}
	setLength := func(length byte) { payload.Length, payload.ResumePC = length, pc+uint16(length) }
	baseImm8 := func(op byte) bool {
		switch op {
		case 0x06, 0x0E, 0x16, 0x1E, 0x26, 0x2E, 0x36, 0x3E, 0xC6, 0xCE, 0xD6, 0xDE, 0xE6, 0xEE, 0xF6, 0xFE, 0xDB, 0xD3:
			return true
		}
		return false
	}
	baseImm16 := func(op byte) bool {
		switch op {
		case 0x01, 0x11, 0x21, 0x31, 0xC3, 0xCD, 0x22, 0x2A, 0x32, 0x3A:
			return true
		}
		return op&0xC7 == 0xC2 || op&0xC7 == 0xC4
	}
	baseRelative := func(op byte) bool {
		return op == 0x10 || op == 0x18 || op == 0x20 || op == 0x28 || op == 0x30 || op == 0x38
	}
	edImm16 := func(op byte) bool {
		return op == 0x43 || op == 0x4B || op == 0x53 || op == 0x5B || op == 0x63 || op == 0x6B || op == 0x73 || op == 0x7B
	}
	ddfdDisplacement := func(op byte) bool {
		return (op&0xC7 == 0x46 && op != 0x76) || (op >= 0x70 && op <= 0x77 && op != 0x76) || op == 0x36 || op&0xC7 == 0x86 || op == 0x34 || op == 0x35
	}
	ddfdImm16 := func(op byte) bool { return op == 0x21 || op == 0x22 || op == 0x2A }
	ddfdImm8 := func(op byte) bool { return op == 0x26 || op == 0x2E }

	switch bytes[0] {
	case 0xCB:
		payload.Prefix, payload.Opcode, payload.RIncrements = 0xCB, bytes[1], 2
		setLength(2)
	case 0xED:
		payload.Prefix, payload.Opcode, payload.RIncrements = 0xED, bytes[1], 2
		setLength(2)
		if edImm16(payload.Opcode) {
			setLength(4)
			payload.Operand = uint16(bytes[2]) | uint16(bytes[3])<<8
		}
	case 0xDD, 0xFD:
		payload.Prefix, payload.Opcode, payload.RIncrements = bytes[0], bytes[1], 2
		switch {
		case payload.Opcode == 0xCB:
			setLength(4)
			payload.Displacement, payload.Opcode, payload.RIncrements = int8(bytes[2]), bytes[3], 3
		case ddfdDisplacement(payload.Opcode):
			setLength(3)
			payload.Displacement = int8(bytes[2])
			if payload.Opcode == 0x36 {
				setLength(4)
				payload.Operand = uint16(bytes[3])
			}
		case ddfdImm16(payload.Opcode):
			setLength(4)
			payload.Operand = uint16(bytes[2]) | uint16(bytes[3])<<8
		case ddfdImm8(payload.Opcode):
			setLength(3)
			payload.Operand = uint16(bytes[2])
		case baseImm8(payload.Opcode) || baseRelative(payload.Opcode):
			// DD/FD ignores these base forms, but opDD/FDUnimplemented
			// delegates to the base handler, which still fetches its operand.
			setLength(3)
			payload.Operand = uint16(bytes[2])
		case baseImm16(payload.Opcode):
			setLength(4)
			payload.Operand = uint16(bytes[2]) | uint16(bytes[3])<<8
		default:
			setLength(2)
		}
	default:
		if baseImm8(bytes[0]) || baseRelative(bytes[0]) {
			setLength(2)
			payload.Operand = uint16(bytes[1])
		} else if baseImm16(bytes[0]) {
			setLength(3)
			payload.Operand = uint16(bytes[1]) | uint16(bytes[2])<<8
		}
	}
	return payload
}

// z80CanonicalHelperBus delegates architectural memory and I/O accesses to
// the original bus while serving instruction fetches from a frozen payload.
type z80CanonicalHelperBus struct {
	bus     Z80Bus
	startPC uint16
	length  byte
	bytes   [4]byte
	frozen  []byte
}

// Read remains an architectural data read. In particular, LD A,(nn) must
// observe memory at nn even when nn happens to overlap the instruction's
// address. Only fetchRead below is allowed to substitute frozen code bytes.
func (b *z80CanonicalHelperBus) Read(addr uint16) byte         { return b.bus.Read(addr) }
func (b *z80CanonicalHelperBus) Write(addr uint16, value byte) { b.bus.Write(addr, value) }
func (b *z80CanonicalHelperBus) In(port uint16) byte           { return b.bus.In(port) }
func (b *z80CanonicalHelperBus) Out(port uint16, value byte)   { b.bus.Out(port, value) }
func (b *z80CanonicalHelperBus) Tick(cycles int)               { b.bus.Tick(cycles) }

func (b *z80CanonicalHelperBus) fetchRead(addr uint16) byte {
	offset := uint16(addr - b.startPC)
	if len(b.frozen) != 0 && int(offset) < len(b.frozen) {
		return b.frozen[offset]
	}
	if int(offset) < int(b.length) {
		return b.bytes[offset]
	}
	if fetcher, ok := b.bus.(interface{ fetchRead(uint16) byte }); ok {
		return fetcher.fetchRead(addr)
	}
	return b.bus.Read(addr)
}

func (b *z80CanonicalHelperBus) debugOnFetch(addr uint16, width int) {
	if debug, ok := b.bus.(interface{ debugOnFetch(uint16, int) }); ok {
		debug.debugOnFetch(addr, width)
	}
}

// executeZ80CanonicalHelper uses CPU_Z80's interpreter operation table for
// semantics, but all opcode, prefix, displacement and immediate fetches are
// served from payload.Bytes. It is shared by native and wasm dispatchers.
func (cpu *CPU_Z80) executeZ80CanonicalHelper(payload z80CanonicalHelperPayload) {
	originalBus := cpu.bus
	cpu.bus = &z80CanonicalHelperBus{
		bus:     originalBus,
		startPC: payload.StartPC,
		length:  payload.Length,
		bytes:   payload.Bytes,
		frozen:  payload.FrozenFetch,
	}
	defer func() { cpu.bus = originalBus }()
	cpu.PC = payload.StartPC
	cpu.Step()
}
