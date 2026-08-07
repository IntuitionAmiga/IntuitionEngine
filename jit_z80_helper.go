package main

// z80CanonicalHelperPayloadFromFetch freezes one instruction image using the
// architectural Z80 fetch view. This is required when an instruction crosses
// from direct RAM into a mapped logical page.
func z80CanonicalHelperPayloadFromFetch(fetch func(uint16) byte, pc uint16) z80CanonicalHelperPayload {
	var bytes [4]byte
	for i := range bytes {
		bytes[i] = fetch(pc + uint16(i))
	}
	payload := z80CanonicalPayloadFromBytes(pc, bytes)
	if bytes[0] != z80JITPrefixDD && bytes[0] != z80JITPrefixFD {
		return payload
	}
	// CPU_Z80 accepts a train of ignored DD/FD prefixes. The fixed four-byte
	// ABI cannot always include its final opcode and operands, so retain the
	// exact fetch image for the canonical helper rather than re-entering the
	// mutable interpreter stream.
	count := 0
	for count < 0xFFFF {
		if b := fetch(pc + uint16(count)); b != z80JITPrefixDD && b != z80JITPrefixFD {
			break
		}
		count++
	}
	if count < 2 || count == 0xFFFF {
		return payload
	}
	var tail [4]byte
	for i := range tail {
		tail[i] = fetch(pc + uint16(count+i))
	}
	tailPayload := z80CanonicalPayloadFromBytes(pc+uint16(count), tail)
	if tailPayload.Length == 0 || count+int(tailPayload.Length) > 0x10000 {
		return payload
	}
	payload.FrozenFetch = make([]byte, count+int(tailPayload.Length))
	for i := range payload.FrozenFetch {
		payload.FrozenFetch[i] = fetch(pc + uint16(i))
	}
	return payload
}

// z80CanonicalHelperPayloadAt decodes one bounded instruction image from an
// already-admitted direct backing slice. It is retained for slice-backed
// callers and tests; dispatchers must capture through the Z80 bus instead.
func z80CanonicalHelperPayloadAt(mem []byte, pc uint16) (z80CanonicalHelperPayload, bool) {
	if len(mem) == 0 || int(pc) >= len(mem) {
		return z80CanonicalHelperPayload{}, false
	}
	read := func(offset uint16) (byte, bool) {
		addr := uint16(uint32(pc) + uint32(offset))
		if int(addr) >= len(mem) {
			return 0, false
		}
		return mem[addr], true
	}
	if _, ok := read(0); !ok {
		return z80CanonicalHelperPayload{}, false
	}
	payload := z80CanonicalHelperPayloadFromFetch(func(addr uint16) byte {
		value, _ := read(uint16(addr - pc))
		return value
	}, pc)
	for i := uint16(0); i < uint16(payload.Length); i++ {
		if _, ok := read(i); !ok {
			return z80CanonicalHelperPayload{}, false
		}
	}
	return payload, true
}
