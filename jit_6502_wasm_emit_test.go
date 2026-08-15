package main

import (
	"context"
	"testing"

	"github.com/tetratelabs/wazero"
)

const (
	p65WasmTestCtx = 0x100
	p65WasmTestCPU = 0x200
)

func TestP65WasmMutatesRAMInventory(t *testing.T) {
	for _, opcode := range []byte{0x00, 0x08, 0x20, 0x48, 0x81, 0x8D, 0x9D, 0x1E, 0xDE, 0xFE} {
		if !p65WasmMutatesRAM(opcode) {
			t.Errorf("opcode %02X must end a wasm module prefix", opcode)
		}
	}
	for _, opcode := range []byte{0xA9, 0x68, 0x60, 0xEA} {
		if p65WasmMutatesRAM(opcode) {
			t.Errorf("non-writing opcode %02X ended a wasm module prefix", opcode)
		}
	}
}

func TestP65WasmCompileBlockRejectsUnsupportedOpcode(t *testing.T) {
	if _, err := p65WasmCompileBlock([]JIT6502Instr{{opcode: 0x03, length: 2}}, 0x0600); err == nil {
		t.Fatal("unsupported undocumented opcode compiled")
	}
}

// TestP65WasmManifestNativeAdmission keeps the wasm encoder aligned with the
// shared frontend contract: every official NMOS form admitted as direct must
// produce a wasm module, not silently become an interpreter-only prefix.
func TestP65WasmManifestNativeAdmission(t *testing.T) {
	for _, entry := range P65OpcodeManifest {
		if entry.Decision != p65OpcodeDirect {
			continue
		}
		instr := JIT6502Instr{opcode: entry.Opcode, length: entry.Length}
		if entry.Length >= 2 {
			instr.operand = uint16(entry.Representative[1])
		}
		if entry.Length == 3 {
			instr.operand |= uint16(entry.Representative[2]) << 8
		}
		if _, err := p65WasmCompileBlock([]JIT6502Instr{instr}, 0x0600); err != nil {
			t.Errorf("direct opcode %02X is not admitted by wasm: %v", entry.Opcode, err)
		}
	}
}

func TestP65WasmCompileBlockImmediateLoads(t *testing.T) {
	instrs := []JIT6502Instr{
		{opcode: 0xA9, length: 2, operand: 0x80}, // LDA #$80
		{opcode: 0xAA, length: 1},                // TAX
		{opcode: 0xE8, length: 1},                // INX
		{opcode: 0xCA, length: 1},                // DEX
		{opcode: 0x38, length: 1},                // SEC
	}
	module, err := p65WasmCompileBlock(instrs, 0x0600)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	r := wazero.NewRuntime(ctx)
	defer r.Close(ctx)
	envB := newWasmModuleBuilder()
	envB.defineMemory(1)
	envB.exportMemory("mem")
	env, err := r.InstantiateWithConfig(ctx, envB.build(), wazero.NewModuleConfig().WithName("env"))
	if err != nil {
		t.Fatal(err)
	}
	mem := env.ExportedMemory("mem")
	if !mem.WriteUint32Le(p65WasmTestCtx+p65WasmCtxOffCpuPtr, p65WasmTestCPU) {
		t.Fatal("write CPU pointer")
	}
	const nzTable = 0x300
	if !mem.WriteUint32Le(p65WasmTestCtx+p65WasmCtxOffNZTable, nzTable) {
		t.Fatal("write N/Z table pointer")
	}
	for value := 0; value < 256; value++ {
		flags := byte(0)
		if value == 0 {
			flags |= ZERO_FLAG
		}
		if value&0x80 != 0 {
			flags |= NEGATIVE_FLAG
		}
		if !mem.WriteByte(nzTable+uint32(value), flags) {
			t.Fatal("write N/Z table")
		}
	}
	if !mem.WriteByte(p65WasmTestCPU+cpu6502OffSR, UNUSED_FLAG|CARRY_FLAG) {
		t.Fatal("write SR")
	}
	mod, err := r.Instantiate(ctx, module)
	if err != nil {
		t.Fatalf("instantiate block: %v", err)
	}
	if _, err := mod.ExportedFunction("block").Call(ctx, p65WasmTestCtx); err != nil {
		t.Fatalf("run block: %v", err)
	}
	if got, ok := mem.ReadByte(p65WasmTestCPU + cpu6502OffA); !ok || got != 0x80 {
		t.Fatalf("A=%02X ok=%v, want 80", got, ok)
	}
	if got, ok := mem.ReadByte(p65WasmTestCPU + cpu6502OffX); !ok || got != 0x80 {
		t.Fatalf("X=%02X ok=%v, want 80", got, ok)
	}
	if got, ok := mem.ReadByte(p65WasmTestCPU + cpu6502OffY); !ok || got != 0 {
		t.Fatalf("Y=%02X ok=%v, want 00", got, ok)
	}
	if got, ok := mem.ReadByte(p65WasmTestCPU + cpu6502OffSR); !ok || got != UNUSED_FLAG|NEGATIVE_FLAG|CARRY_FLAG {
		t.Fatalf("SR=%02X ok=%v, want %02X", got, ok, UNUSED_FLAG|NEGATIVE_FLAG|CARRY_FLAG)
	}
	if got, ok := mem.ReadUint32Le(p65WasmTestCtx + p65WasmCtxOffRetPC); !ok || got != 0x0606 {
		t.Fatalf("RetPC=%04X ok=%v, want 0606", got, ok)
	}
	if got, ok := mem.ReadUint32Le(p65WasmTestCtx + p65WasmCtxOffRetCount); !ok || got != uint32(len(instrs)) {
		t.Fatalf("RetCount=%d ok=%v, want %d", got, ok, len(instrs))
	}
	if got, ok := mem.ReadUint64Le(p65WasmTestCtx + p65WasmCtxOffRetCycles); !ok || got != 10 {
		t.Fatalf("RetCycles=%d ok=%v, want 10", got, ok)
	}
}

func TestP65WasmCompileBlockAbsoluteJump(t *testing.T) {
	module, err := p65WasmCompileBlock([]JIT6502Instr{{opcode: 0x4C, length: 3, operand: 0x1234}}, 0x0600)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	r := wazero.NewRuntime(ctx)
	defer r.Close(ctx)
	envB := newWasmModuleBuilder()
	envB.defineMemory(1)
	envB.exportMemory("mem")
	env, err := r.InstantiateWithConfig(ctx, envB.build(), wazero.NewModuleConfig().WithName("env"))
	if err != nil {
		t.Fatal(err)
	}
	mem := env.ExportedMemory("mem")
	mem.WriteUint32Le(p65WasmTestCtx+p65WasmCtxOffCpuPtr, p65WasmTestCPU)
	mod, err := r.Instantiate(ctx, module)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mod.ExportedFunction("block").Call(ctx, p65WasmTestCtx); err != nil {
		t.Fatal(err)
	}
	if got, ok := mem.ReadUint32Le(p65WasmTestCtx + p65WasmCtxOffRetPC); !ok || got != 0x1234 {
		t.Fatalf("RetPC=%04X ok=%v, want 1234", got, ok)
	}
}

func TestP65WasmCompileBlockJSR(t *testing.T) {
	module, err := p65WasmCompileBlock([]JIT6502Instr{{opcode: 0x20, length: 3, operand: 0x0700}}, 0x0600)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	r := wazero.NewRuntime(ctx)
	defer r.Close(ctx)
	envB := newWasmModuleBuilder()
	envB.defineMemory(1)
	envB.exportMemory("mem")
	env, err := r.InstantiateWithConfig(ctx, envB.build(), wazero.NewModuleConfig().WithName("env"))
	if err != nil {
		t.Fatal(err)
	}
	mem := env.ExportedMemory("mem")
	const guest, pages = 0x1000, 0x3000
	if !mem.WriteUint32Le(p65WasmTestCtx+p65WasmCtxOffCpuPtr, p65WasmTestCPU) ||
		!mem.WriteUint32Le(p65WasmTestCtx+p65WasmCtxOffMemPtr, guest) ||
		!mem.WriteUint32Le(p65WasmTestCtx+p65WasmCtxOffDirectPages, pages) ||
		!mem.WriteByte(p65WasmTestCPU+cpu6502OffSP, 0x00) {
		t.Fatal("seed JSR context")
	}
	mod, err := r.Instantiate(ctx, module)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mod.ExportedFunction("block").Call(ctx, p65WasmTestCtx); err != nil {
		t.Fatal(err)
	}
	if got, ok := mem.ReadByte(p65WasmTestCPU + cpu6502OffSP); !ok || got != 0xFE {
		t.Fatalf("SP=%02X ok=%v, want FE", got, ok)
	}
	if high, ok := mem.ReadByte(guest + 0x0100); !ok || high != 0x06 {
		t.Fatalf("stack high=%02X ok=%v, want 06", high, ok)
	}
	if low, ok := mem.ReadByte(guest + 0x01FF); !ok || low != 0x02 {
		t.Fatalf("stack low=%02X ok=%v, want 02", low, ok)
	}
	if got, ok := mem.ReadUint32Le(p65WasmTestCtx + p65WasmCtxOffRetPC); !ok || got != 0x0700 {
		t.Fatalf("RetPC=%04X ok=%v, want 0700", got, ok)
	}
}

func TestP65WasmCompileBlockConditionalBranch(t *testing.T) {
	module, err := p65WasmCompileBlock([]JIT6502Instr{{opcode: 0xD0, length: 2, operand: 0xFE}}, 0x0600)
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name   string
		sr     byte
		wantPC uint32
		cycles uint64
	}{
		{name: "taken", sr: UNUSED_FLAG, wantPC: 0x0600, cycles: 1},
		{name: "fallthrough", sr: UNUSED_FLAG | ZERO_FLAG, wantPC: 0x0602, cycles: 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			r := wazero.NewRuntime(ctx)
			defer r.Close(ctx)
			envB := newWasmModuleBuilder()
			envB.defineMemory(1)
			envB.exportMemory("mem")
			env, err := r.InstantiateWithConfig(ctx, envB.build(), wazero.NewModuleConfig().WithName("env"))
			if err != nil {
				t.Fatal(err)
			}
			mem := env.ExportedMemory("mem")
			if !mem.WriteUint32Le(p65WasmTestCtx+p65WasmCtxOffCpuPtr, p65WasmTestCPU) || !mem.WriteByte(p65WasmTestCPU+cpu6502OffSR, tt.sr) {
				t.Fatal("seed branch context")
			}
			mod, err := r.Instantiate(ctx, module)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := mod.ExportedFunction("block").Call(ctx, p65WasmTestCtx); err != nil {
				t.Fatal(err)
			}
			if got, ok := mem.ReadUint32Le(p65WasmTestCtx + p65WasmCtxOffRetPC); !ok || got != tt.wantPC {
				t.Fatalf("RetPC=%04X ok=%v, want %04X", got, ok, tt.wantPC)
			}
			if got, ok := mem.ReadUint64Le(p65WasmTestCtx + p65WasmCtxOffRetCycles); !ok || got != tt.cycles {
				t.Fatalf("RetCycles=%d ok=%v, want %d", got, ok, tt.cycles)
			}
		})
	}
}

func TestP65WasmCompileBlockStackWrap(t *testing.T) {
	instrs := []JIT6502Instr{{opcode: 0x48, length: 1}, {opcode: 0x68, length: 1}}
	module, err := p65WasmCompileBlock(instrs, 0x0600)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	r := wazero.NewRuntime(ctx)
	defer r.Close(ctx)
	envB := newWasmModuleBuilder()
	envB.defineMemory(1)
	envB.exportMemory("mem")
	env, err := r.InstantiateWithConfig(ctx, envB.build(), wazero.NewModuleConfig().WithName("env"))
	if err != nil {
		t.Fatal(err)
	}
	mem := env.ExportedMemory("mem")
	const guest, pages, nz = 0x1000, 0x2000, 0x2100
	if !mem.WriteUint32Le(p65WasmTestCtx+p65WasmCtxOffCpuPtr, p65WasmTestCPU) ||
		!mem.WriteUint32Le(p65WasmTestCtx+p65WasmCtxOffMemPtr, guest) ||
		!mem.WriteUint32Le(p65WasmTestCtx+p65WasmCtxOffDirectPages, pages) ||
		!mem.WriteUint32Le(p65WasmTestCtx+p65WasmCtxOffNZTable, nz) ||
		!mem.WriteByte(p65WasmTestCPU+cpu6502OffA, 0x80) ||
		!mem.WriteByte(p65WasmTestCPU+cpu6502OffSP, 0) {
		t.Fatal("seed stack context")
	}
	for value := 0; value < 256; value++ {
		flags := byte(0)
		if value == 0 {
			flags |= ZERO_FLAG
		}
		if value&0x80 != 0 {
			flags |= NEGATIVE_FLAG
		}
		if !mem.WriteByte(nz+uint32(value), flags) {
			t.Fatal("seed N/Z table")
		}
	}
	mod, err := r.Instantiate(ctx, module)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mod.ExportedFunction("block").Call(ctx, p65WasmTestCtx); err != nil {
		t.Fatal(err)
	}
	if got, ok := mem.ReadByte(guest + 0x100); !ok || got != 0x80 {
		t.Fatalf("stack[$0100]=%02X ok=%v, want 80", got, ok)
	}
	if got, ok := mem.ReadByte(p65WasmTestCPU + cpu6502OffSP); !ok || got != 0 {
		t.Fatalf("SP=%02X ok=%v, want 00", got, ok)
	}
	if got, ok := mem.ReadByte(p65WasmTestCPU + cpu6502OffSR); !ok || got != NEGATIVE_FLAG {
		t.Fatalf("SR=%02X ok=%v, want %02X", got, ok, NEGATIVE_FLAG)
	}
	if got, ok := mem.ReadUint64Le(p65WasmTestCtx + p65WasmCtxOffRetCycles); !ok || got != 7 {
		t.Fatalf("RetCycles=%d ok=%v, want 7", got, ok)
	}
}

func TestP65WasmCompileBlockDecimalImmediate(t *testing.T) {
	tests := []struct {
		name   string
		opcode byte
		table  *[p65DecimalTableSize]p65DecimalResult
	}{
		{name: "adc", opcode: 0x69, table: &p65DecimalADC},
		{name: "sbc", opcode: 0xE9, table: &p65DecimalSBC},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			const a, operand, carry = byte(0x45), byte(0x55), byte(1)
			index := int(a) | int(operand)<<8 | int(carry)<<16
			want := tt.table[index]
			module, err := p65WasmCompileBlock([]JIT6502Instr{{opcode: tt.opcode, length: 2, operand: uint16(operand)}}, 0x0600)
			if err != nil {
				t.Fatal(err)
			}
			ctx := context.Background()
			r := wazero.NewRuntime(ctx)
			defer r.Close(ctx)
			envB := newWasmModuleBuilder()
			envB.defineMemory(2)
			envB.exportMemory("mem")
			env, err := r.InstantiateWithConfig(ctx, envB.build(), wazero.NewModuleConfig().WithName("env"))
			if err != nil {
				t.Fatal(err)
			}
			mem := env.ExportedMemory("mem")
			const tableOff = 0x1000
			if !mem.WriteUint32Le(p65WasmTestCtx+p65WasmCtxOffCpuPtr, p65WasmTestCPU) ||
				!mem.WriteUint32Le(p65WasmTestCtx+map[bool]uint32{true: p65WasmCtxOffDecimalADC, false: p65WasmCtxOffDecimalSBC}[tt.opcode == 0x69], tableOff) ||
				!mem.WriteByte(p65WasmTestCPU+cpu6502OffA, a) ||
				!mem.WriteByte(p65WasmTestCPU+cpu6502OffSR, UNUSED_FLAG|DECIMAL_FLAG|CARRY_FLAG) ||
				!mem.WriteByte(tableOff+uint32(index*2), want.A) ||
				!mem.WriteByte(tableOff+uint32(index*2+1), want.Flags) {
				t.Fatal("seed wasm decimal context")
			}
			mod, err := r.Instantiate(ctx, module)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := mod.ExportedFunction("block").Call(ctx, p65WasmTestCtx); err != nil {
				t.Fatal(err)
			}
			if got, ok := mem.ReadByte(p65WasmTestCPU + cpu6502OffA); !ok || got != want.A {
				t.Fatalf("A=%02X ok=%v, want %02X", got, ok, want.A)
			}
			wantSR := byte(UNUSED_FLAG | DECIMAL_FLAG | (want.Flags & (CARRY_FLAG | OVERFLOW_FLAG | NEGATIVE_FLAG | ZERO_FLAG)))
			if got, ok := mem.ReadByte(p65WasmTestCPU + cpu6502OffSR); !ok || got != wantSR {
				t.Fatalf("SR=%02X ok=%v, want %02X", got, ok, wantSR)
			}
		})
	}
}

func TestP65WasmCompileBlockBinaryImmediate(t *testing.T) {
	const a, operand, carry = byte(0x45), byte(0x55), byte(1)
	index := int(a) | int(operand)<<8 | int(carry)<<16
	want := p65BinaryADC[index]
	module, err := p65WasmCompileBlock([]JIT6502Instr{{opcode: 0x69, length: 2, operand: uint16(operand)}}, 0x0600)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	r := wazero.NewRuntime(ctx)
	defer r.Close(ctx)
	envB := newWasmModuleBuilder()
	envB.defineMemory(2)
	envB.exportMemory("mem")
	env, err := r.InstantiateWithConfig(ctx, envB.build(), wazero.NewModuleConfig().WithName("env"))
	if err != nil {
		t.Fatal(err)
	}
	mem := env.ExportedMemory("mem")
	const tableOff = 0x1000
	if !mem.WriteUint32Le(p65WasmTestCtx+p65WasmCtxOffCpuPtr, p65WasmTestCPU) ||
		!mem.WriteUint32Le(p65WasmTestCtx+p65WasmCtxOffBinaryADC, tableOff) ||
		!mem.WriteByte(p65WasmTestCPU+cpu6502OffA, a) ||
		!mem.WriteByte(p65WasmTestCPU+cpu6502OffSR, UNUSED_FLAG|CARRY_FLAG) ||
		!mem.WriteByte(tableOff+uint32(index*2), want.A) ||
		!mem.WriteByte(tableOff+uint32(index*2+1), want.Flags) {
		t.Fatal("seed wasm binary context")
	}
	mod, err := r.Instantiate(ctx, module)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mod.ExportedFunction("block").Call(ctx, p65WasmTestCtx); err != nil {
		t.Fatal(err)
	}
	if got, ok := mem.ReadByte(p65WasmTestCPU + cpu6502OffA); !ok || got != want.A {
		t.Fatalf("A=%02X ok=%v, want %02X", got, ok, want.A)
	}
	wantSR := byte(UNUSED_FLAG | (want.Flags & (CARRY_FLAG | OVERFLOW_FLAG | NEGATIVE_FLAG | ZERO_FLAG)))
	if got, ok := mem.ReadByte(p65WasmTestCPU + cpu6502OffSR); !ok || got != wantSR {
		t.Fatalf("SR=%02X ok=%v, want %02X", got, ok, wantSR)
	}
}

func TestP65WasmCompileBlockDirectRAMLoadStore(t *testing.T) {
	instrs := []JIT6502Instr{
		{opcode: 0xA9, length: 2, operand: 0x42}, // LDA #$42
		{opcode: 0x85, length: 2, operand: 0x10}, // STA $10
		{opcode: 0xA5, length: 2, operand: 0x10}, // LDA $10
		{opcode: 0x24, length: 2, operand: 0x10}, // BIT $10
		{opcode: 0xA2, length: 2, operand: 0x02}, // LDX #2
		{opcode: 0x95, length: 2, operand: 0xFF}, // STA $FF,X -> $01
	}
	module, err := p65WasmCompileBlock(instrs, 0x0600)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	r := wazero.NewRuntime(ctx)
	defer r.Close(ctx)
	envB := newWasmModuleBuilder()
	envB.defineMemory(1)
	envB.exportMemory("mem")
	env, err := r.InstantiateWithConfig(ctx, envB.build(), wazero.NewModuleConfig().WithName("env"))
	if err != nil {
		t.Fatal(err)
	}
	mem := env.ExportedMemory("mem")
	const guest, pages, nz = 0x1000, 0x2000, 0x2100
	if !mem.WriteUint32Le(p65WasmTestCtx+p65WasmCtxOffCpuPtr, p65WasmTestCPU) ||
		!mem.WriteUint32Le(p65WasmTestCtx+p65WasmCtxOffMemPtr, guest) ||
		!mem.WriteUint32Le(p65WasmTestCtx+p65WasmCtxOffDirectPages, pages) ||
		!mem.WriteUint32Le(p65WasmTestCtx+p65WasmCtxOffNZTable, nz) {
		t.Fatal("seed wasm direct-RAM context")
	}
	for value := 0; value < 256; value++ {
		flags := byte(0)
		if value == 0 {
			flags |= ZERO_FLAG
		}
		if value&0x80 != 0 {
			flags |= NEGATIVE_FLAG
		}
		if !mem.WriteByte(nz+uint32(value), flags) {
			t.Fatal("seed N/Z table")
		}
	}
	mod, err := r.Instantiate(ctx, module)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mod.ExportedFunction("block").Call(ctx, p65WasmTestCtx); err != nil {
		t.Fatal(err)
	}
	if got, ok := mem.ReadByte(guest + 0x10); !ok || got != 0x42 {
		t.Fatalf("guest[$10]=%02X ok=%v, want 42", got, ok)
	}
	if got, ok := mem.ReadByte(guest + 0x01); !ok || got != 0x42 {
		t.Fatalf("guest[$01]=%02X ok=%v, want 42", got, ok)
	}
	if got, ok := mem.ReadByte(p65WasmTestCPU + cpu6502OffA); !ok || got != 0x42 {
		t.Fatalf("A=%02X ok=%v, want 42", got, ok)
	}
	if got, ok := mem.ReadByte(p65WasmTestCPU + cpu6502OffSR); !ok || got != OVERFLOW_FLAG {
		t.Fatalf("SR=%02X ok=%v, want %02X", got, ok, OVERFLOW_FLAG)
	}
	if got, ok := mem.ReadUint32Le(p65WasmTestCtx + p65WasmCtxOffRetPC); !ok || got != 0x060C {
		t.Fatalf("RetPC=%04X ok=%v, want 060C", got, ok)
	}
	if got, ok := mem.ReadUint64Le(p65WasmTestCtx + p65WasmCtxOffRetCycles); !ok || got != 17 {
		t.Fatalf("RetCycles=%d ok=%v, want 17", got, ok)
	}
}
