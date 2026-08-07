//go:build !js

package main

import (
	"context"
	"testing"
	"unsafe"

	"github.com/tetratelabs/wazero"
)

func TestZ80WasmCPURegisterLayout(t *testing.T) {
	if unsafe.Offsetof(CPU_Z80{}.A) != z80WasmCPUOffA || unsafe.Offsetof(CPU_Z80{}.F) != z80WasmCPUOffF ||
		unsafe.Offsetof(CPU_Z80{}.B) != z80WasmCPUOffB || unsafe.Offsetof(CPU_Z80{}.C) != z80WasmCPUOffC ||
		unsafe.Offsetof(CPU_Z80{}.D) != z80WasmCPUOffD || unsafe.Offsetof(CPU_Z80{}.E) != z80WasmCPUOffE ||
		unsafe.Offsetof(CPU_Z80{}.H) != z80WasmCPUOffH || unsafe.Offsetof(CPU_Z80{}.L) != z80WasmCPUOffL ||
		unsafe.Offsetof(CPU_Z80{}.SP) != z80WasmCPUOffSP || unsafe.Offsetof(CPU_Z80{}.I) != z80WasmCPUOffI || unsafe.Offsetof(CPU_Z80{}.IM) != z80WasmCPUOffIM || unsafe.Offsetof(CPU_Z80{}.IFF2) != z80WasmCPUOffIFF2 {
		t.Fatal("wasm register ABI diverged from CPU_Z80")
	}
}

func TestZ80WasmContextLayout(t *testing.T) {
	if z80WasmCtxOffCPUPtr != 0 || z80WasmCtxOffRetPC != 4 || z80WasmCtxOffRetCycles != 8 ||
		z80WasmCtxOffRetCount != 16 || z80WasmCtxOffRIncrements != 20 ||
		z80WasmCtxOffMemPtr != 24 || z80WasmCtxOffDirectPageBitmap != 28 ||
		z80WasmCtxOffCodePageBitmap != 32 || z80WasmCtxOffNeedBail != 36 ||
		z80WasmCtxOffNeedInval != 40 || z80WasmCtxOffInvalPage != 44 ||
		z80WasmCtxOffIFFDelay != 48 || z80WasmCtxImageSize != 52 {
		t.Fatal("wasm Z80 context ABI changed without updating its layout guard")
	}
}

func TestZ80WasmManifestAdmitsEveryNonObservationForm(t *testing.T) {
	var missing []string
	for _, row := range z80JITOpcodeManifest() {
		if row.WasmOutcome != z80JITOutcomeDirect {
			continue
		}
		instr := row.Instr
		wasmInstr := z80WasmInstr{prefix: instr.prefix, opcode: instr.opcode, operand: byte(instr.operand), operandHi: byte(instr.operand >> 8)}
		if instr.opcode == z80JITPrefixCB && (instr.prefix == z80JITPrefixDD || instr.prefix == z80JITPrefixFD) {
			wasmInstr.opcode = instr.cbSubOp
			wasmInstr.displacement = instr.displacement
			wasmInstr.indexedCB = true
		}
		if _, _, _, err := z80WasmInstructionMeta(wasmInstr); err != nil {
			missing = append(missing, row.Name)
		}
	}
	if len(missing) != 0 {
		t.Fatalf("wasm JIT has %d non-observation forms without a native lowering: %v", len(missing), missing)
	}
}

func TestZ80WasmNewFamiliesInstantiate(t *testing.T) {
	for _, instr := range []z80WasmInstr{
		{opcode: 0x27},
		{prefix: z80JITPrefixDD, opcode: 0x09},
		{opcode: 0xE3},
	} {
		module, err := z80WasmCompileBlock([]z80WasmInstr{instr}, 0x0100)
		if err != nil {
			t.Fatalf("%02X:%02X compile: %v", instr.prefix, instr.opcode, err)
		}
		ctx := context.Background()
		r := wazero.NewRuntime(ctx)
		if _, err := r.CompileModule(ctx, module); err != nil {
			r.Close(ctx)
			t.Fatalf("%02X:%02X instantiate: %v", instr.prefix, instr.opcode, err)
		}
		r.Close(ctx)
	}
}

func TestZ80WasmCBSetRegisterKeepsSequentialPC(t *testing.T) {
	for _, instr := range []z80WasmInstr{
		{prefix: z80JITPrefixCB, opcode: 0xC3},
		{prefix: z80JITPrefixDD, opcode: 0xC3, indexedCB: true},
		{prefix: z80JITPrefixFD, opcode: 0xC3, indexedCB: true},
	} {
		module, err := z80WasmCompileBlock([]z80WasmInstr{instr}, 0x0100)
		if err != nil {
			t.Fatal(err)
		}
		ctx := context.Background()
		r := wazero.NewRuntime(ctx)
		defer r.Close(ctx)
		envBuilder := newWasmModuleBuilder()
		envBuilder.defineMemory(1)
		envBuilder.exportMemory("mem")
		env, err := r.InstantiateWithConfig(ctx, envBuilder.build(), wazero.NewModuleConfig().WithName("env"))
		if err != nil {
			t.Fatal(err)
		}
		mod, err := r.Instantiate(ctx, module)
		if err != nil {
			t.Fatal(err)
		}
		const contextAddress, cpuAddress = 0x100, 0x200
		mem := env.ExportedMemory("mem")
		if !mem.WriteUint32Le(contextAddress+z80WasmCtxOffCPUPtr, cpuAddress) {
			t.Fatal("write CPU pointer")
		}
		if _, err = mod.ExportedFunction("block").Call(ctx, contextAddress); err != nil {
			t.Fatal(err)
		}
		if got, _ := mem.ReadUint32Le(contextAddress + z80WasmCtxOffRetPC); got != 0x0102 {
			want := uint32(0x0102)
			if instr.indexedCB {
				want = 0x0104
			}
			if got != want {
				t.Fatalf("prefix %02X indexed=%v PC = %04X, want %04X", instr.prefix, instr.indexedCB, got, want)
			}
		}
	}
}

func TestZ80WasmIgnoredIndexPrefixJPStillJumps(t *testing.T) {
	for _, prefix := range []byte{z80JITPrefixDD, z80JITPrefixFD} {
		module, err := z80WasmCompileBlock([]z80WasmInstr{{prefix: prefix, opcode: 0xC3, operand: 0x34, operandHi: 0x12}}, 0x0100)
		if err != nil {
			t.Fatal(err)
		}
		ctx := context.Background()
		r := wazero.NewRuntime(ctx)
		envBuilder := newWasmModuleBuilder()
		envBuilder.defineMemory(1)
		envBuilder.exportMemory("mem")
		env, err := r.InstantiateWithConfig(ctx, envBuilder.build(), wazero.NewModuleConfig().WithName("env"))
		if err != nil {
			r.Close(ctx)
			t.Fatal(err)
		}
		mod, err := r.Instantiate(ctx, module)
		if err != nil {
			r.Close(ctx)
			t.Fatal(err)
		}
		const contextAddress = 0x100
		if _, err = mod.ExportedFunction("block").Call(ctx, contextAddress); err != nil {
			r.Close(ctx)
			t.Fatal(err)
		}
		if got, _ := env.ExportedMemory("mem").ReadUint32Le(contextAddress + z80WasmCtxOffRetPC); got != 0x1234 {
			r.Close(ctx)
			t.Fatalf("prefix %02X PC = %04X, want 1234", prefix, got)
		}
		r.Close(ctx)
	}
}

func TestZ80WasmEIFinishesInstructionDelay(t *testing.T) {
	module, err := z80WasmCompileBlock([]z80WasmInstr{{opcode: 0xFB}}, 0x0100)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	r := wazero.NewRuntime(ctx)
	defer r.Close(ctx)
	envBuilder := newWasmModuleBuilder()
	envBuilder.defineMemory(1)
	envBuilder.exportMemory("mem")
	env, err := r.InstantiateWithConfig(ctx, envBuilder.build(), wazero.NewModuleConfig().WithName("env"))
	if err != nil {
		t.Fatal(err)
	}
	mod, err := r.Instantiate(ctx, module)
	if err != nil {
		t.Fatal(err)
	}
	const contextAddress = 0x100
	if _, err = mod.ExportedFunction("block").Call(ctx, contextAddress); err != nil {
		t.Fatal(err)
	}
	if got, _ := env.ExportedMemory("mem").ReadUint32Le(contextAddress + z80WasmCtxOffIFFDelay); got != 1 {
		t.Fatalf("IFF delay = %d, want 1 after EI completes", got)
	}
}

func TestZ80WasmCompileBlockDirectRegisterForms(t *testing.T) {
	module, err := z80WasmCompileBlock([]z80WasmInstr{
		{opcode: 0x00},                // NOP
		{opcode: 0x3E, operand: 0x81}, // LD A,$81
		{opcode: 0x07},                // RLCA
		{opcode: 0x0F},                // RRCA
		{opcode: 0x17},                // RLA
		{opcode: 0x1F},                // RRA
		{opcode: 0x47},                // LD B,A
	}, 0x1200)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	r := wazero.NewRuntime(ctx)
	defer r.Close(ctx)
	envBuilder := newWasmModuleBuilder()
	envBuilder.defineMemory(1)
	envBuilder.exportMemory("mem")
	env, err := r.InstantiateWithConfig(ctx, envBuilder.build(), wazero.NewModuleConfig().WithName("env"))
	if err != nil {
		t.Fatal(err)
	}
	mem := env.ExportedMemory("mem")
	const contextAddress, cpuAddress = 0x100, 0x200
	if !mem.WriteUint32Le(contextAddress+z80WasmCtxOffCPUPtr, cpuAddress) {
		t.Fatal("write CPU pointer")
	}
	mod, err := r.Instantiate(ctx, module)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = mod.ExportedFunction("block").Call(ctx, contextAddress); err != nil {
		t.Fatal(err)
	}
	if got, ok := mem.ReadByte(cpuAddress + z80WasmCPUOffA); !ok || got != 0x81 {
		t.Fatalf("A = %02X, ok=%v", got, ok)
	}
	if got, ok := mem.ReadByte(cpuAddress + z80WasmCPUOffB); !ok || got != 0x81 {
		t.Fatalf("B = %02X, ok=%v", got, ok)
	}
	if got, ok := mem.ReadUint32Le(contextAddress + z80WasmCtxOffRetPC); !ok || got != 0x1208 {
		t.Fatalf("PC = %04X, ok=%v", got, ok)
	}
	if got, ok := mem.ReadUint64Le(contextAddress + z80WasmCtxOffRetCycles); !ok || got != 31 {
		t.Fatalf("cycles = %d, ok=%v", got, ok)
	}
	if got, ok := mem.ReadUint32Le(contextAddress + z80WasmCtxOffRetCount); !ok || got != 7 {
		t.Fatalf("count = %d, ok=%v", got, ok)
	}
	if got, ok := mem.ReadUint32Le(contextAddress + z80WasmCtxOffRIncrements); !ok || got != 7 {
		t.Fatalf("R increments = %d, ok=%v", got, ok)
	}
}

func TestZ80WasmCompileBlockDirectPairForms(t *testing.T) {
	module, err := z80WasmCompileBlock([]z80WasmInstr{
		{opcode: 0x21, operand: 0x34, operandHi: 0x12}, // LD HL,$1234
		{opcode: 0x23}, // INC HL
		{opcode: 0x2B}, // DEC HL
		{opcode: 0xF9}, // LD SP,HL
	}, 0x1200)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	r := wazero.NewRuntime(ctx)
	defer r.Close(ctx)
	envBuilder := newWasmModuleBuilder()
	envBuilder.defineMemory(1)
	envBuilder.exportMemory("mem")
	env, err := r.InstantiateWithConfig(ctx, envBuilder.build(), wazero.NewModuleConfig().WithName("env"))
	if err != nil {
		t.Fatal(err)
	}
	mem := env.ExportedMemory("mem")
	const contextAddress, cpuAddress = 0x100, 0x200
	if !mem.WriteUint32Le(contextAddress+z80WasmCtxOffCPUPtr, cpuAddress) {
		t.Fatal("write CPU pointer")
	}
	mod, err := r.Instantiate(ctx, module)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = mod.ExportedFunction("block").Call(ctx, contextAddress); err != nil {
		t.Fatal(err)
	}
	if got, ok := mem.ReadUint16Le(cpuAddress + z80WasmCPUOffSP); !ok || got != 0x1234 {
		t.Fatalf("SP = %04X, ok=%v", got, ok)
	}
}

func TestZ80WasmCompileBlockDirectAddHLPair(t *testing.T) {
	module, err := z80WasmCompileBlock([]z80WasmInstr{{opcode: 0x09}}, 0x1200) // ADD HL,BC
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	r := wazero.NewRuntime(ctx)
	defer r.Close(ctx)
	envBuilder := newWasmModuleBuilder()
	envBuilder.defineMemory(1)
	envBuilder.exportMemory("mem")
	env, err := r.InstantiateWithConfig(ctx, envBuilder.build(), wazero.NewModuleConfig().WithName("env"))
	if err != nil {
		t.Fatal(err)
	}
	mem := env.ExportedMemory("mem")
	const contextAddress, cpuAddress = 0x100, 0x200
	if !mem.WriteUint32Le(contextAddress+z80WasmCtxOffCPUPtr, cpuAddress) ||
		!mem.WriteByte(cpuAddress+z80WasmCPUOffH, 0x8F) || !mem.WriteByte(cpuAddress+z80WasmCPUOffL, 0xFF) ||
		!mem.WriteByte(cpuAddress+z80WasmCPUOffB, 0x70) || !mem.WriteByte(cpuAddress+z80WasmCPUOffC, 0x01) ||
		!mem.WriteByte(cpuAddress+z80WasmCPUOffF, z80FlagZ|z80FlagPV) {
		t.Fatal("set ADD HL state")
	}
	mod, err := r.Instantiate(ctx, module)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = mod.ExportedFunction("block").Call(ctx, contextAddress); err != nil {
		t.Fatal(err)
	}
	if hi, _ := mem.ReadByte(cpuAddress + z80WasmCPUOffH); hi != 0x00 {
		t.Fatalf("H = %02X, want 00", hi)
	}
	if lo, _ := mem.ReadByte(cpuAddress + z80WasmCPUOffL); lo != 0x00 {
		t.Fatalf("L = %02X, want 00", lo)
	}
	if flags, _ := mem.ReadByte(cpuAddress + z80WasmCPUOffF); flags != z80FlagZ|z80FlagPV|z80FlagH|z80FlagC {
		t.Fatalf("F = %02X, want %02X", flags, z80FlagZ|z80FlagPV|z80FlagH|z80FlagC)
	}
}

func TestZ80WasmCompileBlockDirectRegisterExchange(t *testing.T) {
	module, err := z80WasmCompileBlock([]z80WasmInstr{{opcode: 0xEB}}, 0x1200) // EX DE,HL
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	r := wazero.NewRuntime(ctx)
	defer r.Close(ctx)
	envBuilder := newWasmModuleBuilder()
	envBuilder.defineMemory(1)
	envBuilder.exportMemory("mem")
	env, err := r.InstantiateWithConfig(ctx, envBuilder.build(), wazero.NewModuleConfig().WithName("env"))
	if err != nil {
		t.Fatal(err)
	}
	mem := env.ExportedMemory("mem")
	const contextAddress, cpuAddress = 0x100, 0x200
	if !mem.WriteUint32Le(contextAddress+z80WasmCtxOffCPUPtr, cpuAddress) ||
		!mem.WriteByte(cpuAddress+z80WasmCPUOffD, 0x56) || !mem.WriteByte(cpuAddress+z80WasmCPUOffE, 0x78) ||
		!mem.WriteByte(cpuAddress+z80WasmCPUOffH, 0x12) || !mem.WriteByte(cpuAddress+z80WasmCPUOffL, 0x34) {
		t.Fatal("set exchange state")
	}
	mod, err := r.Instantiate(ctx, module)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = mod.ExportedFunction("block").Call(ctx, contextAddress); err != nil {
		t.Fatal(err)
	}
	for offset, want := range map[uint32]byte{z80WasmCPUOffD: 0x12, z80WasmCPUOffE: 0x34, z80WasmCPUOffH: 0x56, z80WasmCPUOffL: 0x78} {
		if got, ok := mem.ReadByte(cpuAddress + offset); !ok || got != want {
			t.Fatalf("register offset %d = %02X, want %02X", offset, got, want)
		}
	}
}

func TestZ80WasmCompileBlockDirectFlagControls(t *testing.T) {
	module, err := z80WasmCompileBlock([]z80WasmInstr{{opcode: 0x2F}}, 0x1200) // CPL
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	r := wazero.NewRuntime(ctx)
	defer r.Close(ctx)
	envBuilder := newWasmModuleBuilder()
	envBuilder.defineMemory(1)
	envBuilder.exportMemory("mem")
	env, err := r.InstantiateWithConfig(ctx, envBuilder.build(), wazero.NewModuleConfig().WithName("env"))
	if err != nil {
		t.Fatal(err)
	}
	mem := env.ExportedMemory("mem")
	const contextAddress, cpuAddress = 0x100, 0x200
	if !mem.WriteUint32Le(contextAddress+z80WasmCtxOffCPUPtr, cpuAddress) ||
		!mem.WriteByte(cpuAddress+z80WasmCPUOffA, 0x28) || !mem.WriteByte(cpuAddress+z80WasmCPUOffF, 0xC5) {
		t.Fatal("set CPL state")
	}
	mod, err := r.Instantiate(ctx, module)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = mod.ExportedFunction("block").Call(ctx, contextAddress); err != nil {
		t.Fatal(err)
	}
	if got, _ := mem.ReadByte(cpuAddress + z80WasmCPUOffA); got != 0xD7 {
		t.Fatalf("CPL A = %02X, want D7", got)
	}
	if got, _ := mem.ReadByte(cpuAddress + z80WasmCPUOffF); got != 0xD7 {
		t.Fatalf("CPL F = %02X, want D7", got)
	}
}

func TestZ80WasmCompileBlockDirectIncDecFlags(t *testing.T) {
	for _, tc := range []struct {
		name       string
		opcode, in byte
		wantValue  byte
		wantFlags  byte
	}{
		{"inc overflow", 0x04, 0x7F, 0x80, 0x95},
		{"dec overflow", 0x05, 0x80, 0x7F, 0x3F},
	} {
		t.Run(tc.name, func(t *testing.T) {
			module, err := z80WasmCompileBlock([]z80WasmInstr{{opcode: tc.opcode}}, 0x1200)
			if err != nil {
				t.Fatal(err)
			}
			ctx := context.Background()
			r := wazero.NewRuntime(ctx)
			defer r.Close(ctx)
			envBuilder := newWasmModuleBuilder()
			envBuilder.defineMemory(1)
			envBuilder.exportMemory("mem")
			env, err := r.InstantiateWithConfig(ctx, envBuilder.build(), wazero.NewModuleConfig().WithName("env"))
			if err != nil {
				t.Fatal(err)
			}
			mem := env.ExportedMemory("mem")
			const contextAddress, cpuAddress = 0x100, 0x200
			if !mem.WriteUint32Le(contextAddress+z80WasmCtxOffCPUPtr, cpuAddress) || !mem.WriteByte(cpuAddress+z80WasmCPUOffB, tc.in) || !mem.WriteByte(cpuAddress+z80WasmCPUOffF, z80FlagC) {
				t.Fatal("set INC/DEC state")
			}
			mod, err := r.Instantiate(ctx, module)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = mod.ExportedFunction("block").Call(ctx, contextAddress); err != nil {
				t.Fatal(err)
			}
			if got, _ := mem.ReadByte(cpuAddress + z80WasmCPUOffB); got != tc.wantValue {
				t.Fatalf("B = %02X, want %02X", got, tc.wantValue)
			}
			if got, _ := mem.ReadByte(cpuAddress + z80WasmCPUOffF); got != tc.wantFlags {
				t.Fatalf("F = %02X, want %02X", got, tc.wantFlags)
			}
		})
	}
}

func TestZ80WasmCompileBlockDirectLogicalALU(t *testing.T) {
	for _, tc := range []struct {
		name       string
		instr      z80WasmInstr
		a, operand byte
		wantA      byte
		wantF      byte
	}{
		{"AND register", z80WasmInstr{opcode: 0xA0}, 0xD3, 0x6A, 0x42, 0x14},
		{"XOR immediate", z80WasmInstr{opcode: 0xEE, operand: 0xFF}, 0x55, 0, 0xAA, 0xAC},
		{"OR register", z80WasmInstr{opcode: 0xB0}, 0x40, 0x03, 0x43, 0x00},
	} {
		t.Run(tc.name, func(t *testing.T) {
			module, err := z80WasmCompileBlock([]z80WasmInstr{tc.instr}, 0x1200)
			if err != nil {
				t.Fatal(err)
			}
			ctx := context.Background()
			r := wazero.NewRuntime(ctx)
			defer r.Close(ctx)
			envBuilder := newWasmModuleBuilder()
			envBuilder.defineMemory(1)
			envBuilder.exportMemory("mem")
			env, err := r.InstantiateWithConfig(ctx, envBuilder.build(), wazero.NewModuleConfig().WithName("env"))
			if err != nil {
				t.Fatal(err)
			}
			mem := env.ExportedMemory("mem")
			const contextAddress, cpuAddress = 0x100, 0x200
			if !mem.WriteUint32Le(contextAddress+z80WasmCtxOffCPUPtr, cpuAddress) ||
				!mem.WriteByte(cpuAddress+z80WasmCPUOffA, tc.a) ||
				!mem.WriteByte(cpuAddress+z80WasmCPUOffB, tc.operand) {
				t.Fatal("set logical ALU state")
			}
			mod, err := r.Instantiate(ctx, module)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = mod.ExportedFunction("block").Call(ctx, contextAddress); err != nil {
				t.Fatal(err)
			}
			if got, _ := mem.ReadByte(cpuAddress + z80WasmCPUOffA); got != tc.wantA {
				t.Fatalf("A = %02X, want %02X", got, tc.wantA)
			}
			if got, _ := mem.ReadByte(cpuAddress + z80WasmCPUOffF); got != tc.wantF {
				t.Fatalf("F = %02X, want %02X", got, tc.wantF)
			}
		})
	}
}

func TestZ80WasmCompileBlockDirectAddALU(t *testing.T) {
	for _, tc := range []struct {
		name  string
		instr z80WasmInstr
		a, b  byte
		wantA byte
		wantF byte
	}{
		{"register overflow", z80WasmInstr{opcode: 0x80}, 0x7F, 0x01, 0x80, z80FlagS | z80FlagH | z80FlagPV},
		{"immediate carry", z80WasmInstr{opcode: 0xC6, operand: 0x01}, 0xFF, 0x00, 0x00, z80FlagZ | z80FlagH | z80FlagC},
	} {
		t.Run(tc.name, func(t *testing.T) {
			z80WasmCompileAddALUTest(t, tc.instr, tc.a, tc.b, 0, tc.wantA, tc.wantF)
		})
	}
}

func TestZ80WasmCompileBlockDirectSubALU(t *testing.T) {
	for _, tc := range []struct {
		name  string
		instr z80WasmInstr
		a, b  byte
		wantA byte
		wantF byte
	}{
		{"register overflow", z80WasmInstr{opcode: 0x90}, 0x80, 0x01, 0x7F, z80FlagX | z80FlagY | z80FlagH | z80FlagPV | z80FlagN},
		{"immediate borrow", z80WasmInstr{opcode: 0xD6, operand: 0x01}, 0x00, 0x00, 0xFF, z80FlagS | z80FlagX | z80FlagY | z80FlagH | z80FlagN | z80FlagC},
	} {
		t.Run(tc.name, func(t *testing.T) {
			z80WasmCompileSubALUTest(t, tc.instr, tc.a, tc.b, tc.wantA, tc.wantF)
		})
	}
}

func TestZ80WasmCompileBlockDirectCompareALU(t *testing.T) {
	for _, tc := range []struct {
		name  string
		instr z80WasmInstr
		a, b  byte
		wantF byte
	}{
		{"register overflow", z80WasmInstr{opcode: 0xB8}, 0x80, 0x01, z80FlagX | z80FlagY | z80FlagH | z80FlagPV | z80FlagN},
		{"immediate borrow", z80WasmInstr{opcode: 0xFE, operand: 0x01}, 0x00, 0, z80FlagS | z80FlagX | z80FlagY | z80FlagH | z80FlagN | z80FlagC},
	} {
		t.Run(tc.name, func(t *testing.T) {
			z80WasmCompileSubALUTest(t, tc.instr, tc.a, tc.b, tc.a, tc.wantF)
		})
	}
}

func z80WasmCompileSubALUTest(t *testing.T, instr z80WasmInstr, a, b, wantA, wantF byte) {
	z80WasmCompileSubALUTestWithFlags(t, instr, a, b, 0, wantA, wantF)
}

func z80WasmCompileSubALUTestWithFlags(t *testing.T, instr z80WasmInstr, a, b, flags, wantA, wantF byte) {
	t.Helper()
	module, err := z80WasmCompileBlock([]z80WasmInstr{instr}, 0x1200)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	r := wazero.NewRuntime(ctx)
	defer r.Close(ctx)
	envBuilder := newWasmModuleBuilder()
	envBuilder.defineMemory(1)
	envBuilder.exportMemory("mem")
	env, err := r.InstantiateWithConfig(ctx, envBuilder.build(), wazero.NewModuleConfig().WithName("env"))
	if err != nil {
		t.Fatal(err)
	}
	mem := env.ExportedMemory("mem")
	const contextAddress, cpuAddress = 0x100, 0x200
	if !mem.WriteUint32Le(contextAddress+z80WasmCtxOffCPUPtr, cpuAddress) ||
		!mem.WriteByte(cpuAddress+z80WasmCPUOffA, a) ||
		!mem.WriteByte(cpuAddress+z80WasmCPUOffB, b) ||
		!mem.WriteByte(cpuAddress+z80WasmCPUOffF, flags) {
		t.Fatal("set SUB state")
	}
	mod, err := r.Instantiate(ctx, module)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = mod.ExportedFunction("block").Call(ctx, contextAddress); err != nil {
		t.Fatal(err)
	}
	if got, _ := mem.ReadByte(cpuAddress + z80WasmCPUOffA); got != wantA {
		t.Fatalf("A = %02X, want %02X", got, wantA)
	}
	if got, _ := mem.ReadByte(cpuAddress + z80WasmCPUOffF); got != wantF {
		t.Fatalf("F = %02X, want %02X", got, wantF)
	}
}

func TestZ80WasmCompileBlockDirectADCALU(t *testing.T) {
	for _, tc := range []struct {
		name  string
		instr z80WasmInstr
		a, b  byte
		wantA byte
		wantF byte
	}{
		{"register carry", z80WasmInstr{opcode: 0x88}, 0xFF, 0x00, 0x00, z80FlagZ | z80FlagH | z80FlagC},
		{"immediate carry", z80WasmInstr{opcode: 0xCE, operand: 0x00}, 0xFF, 0x00, 0x00, z80FlagZ | z80FlagH | z80FlagC},
	} {
		t.Run(tc.name, func(t *testing.T) {
			z80WasmCompileAddALUTest(t, tc.instr, tc.a, tc.b, z80FlagC, tc.wantA, tc.wantF)
		})
	}
}

func TestZ80WasmCompileBlockDirectSBCALU(t *testing.T) {
	for _, tc := range []struct {
		name  string
		instr z80WasmInstr
		a, b  byte
		wantA byte
		wantF byte
	}{
		{"register borrow", z80WasmInstr{opcode: 0x98}, 0x00, 0x00, 0xFF, z80FlagS | z80FlagX | z80FlagY | z80FlagH | z80FlagN | z80FlagC},
		{"immediate overflow", z80WasmInstr{opcode: 0xDE, operand: 0x00}, 0x80, 0x00, 0x7F, z80FlagX | z80FlagY | z80FlagH | z80FlagPV | z80FlagN},
	} {
		t.Run(tc.name, func(t *testing.T) {
			z80WasmCompileSubALUTestWithFlags(t, tc.instr, tc.a, tc.b, z80FlagC, tc.wantA, tc.wantF)
		})
	}
}

// Direct-page (HL) access is not a host-observation boundary. The emitted
// module must admit it and use the wasm bailout field only when the page is
// mapped or otherwise non-direct.
func TestZ80WasmCompileBlockDirectAddHL(t *testing.T) {
	module, err := z80WasmCompileBlock([]z80WasmInstr{{opcode: 0x86}}, 0x1200)
	if err != nil {
		t.Fatalf("ADD A,(HL) is a direct-page form, got %v", err)
	}
	ctx := context.Background()
	r := wazero.NewRuntime(ctx)
	defer r.Close(ctx)
	envBuilder := newWasmModuleBuilder()
	envBuilder.defineMemory(1)
	envBuilder.exportMemory("mem")
	env, err := r.InstantiateWithConfig(ctx, envBuilder.build(), wazero.NewModuleConfig().WithName("env"))
	if err != nil {
		t.Fatal(err)
	}
	mem := env.ExportedMemory("mem")
	const contextAddress, cpuAddress, ramAddress, pagesAddress = 0x100, 0x200, 0x1000, 0x3000
	if !mem.WriteUint32Le(contextAddress+z80WasmCtxOffCPUPtr, cpuAddress) ||
		!mem.WriteUint32Le(contextAddress+z80WasmCtxOffMemPtr, ramAddress) ||
		!mem.WriteUint32Le(contextAddress+z80WasmCtxOffDirectPageBitmap, pagesAddress) ||
		!mem.WriteByte(cpuAddress+z80WasmCPUOffA, 0x12) ||
		!mem.WriteByte(cpuAddress+z80WasmCPUOffH, 0x40) ||
		!mem.WriteByte(cpuAddress+z80WasmCPUOffL, 0x34) ||
		!mem.WriteByte(ramAddress+0x4034, 0x22) {
		t.Fatal("set direct ADD A,(HL) state")
	}
	mod, err := r.Instantiate(ctx, module)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = mod.ExportedFunction("block").Call(ctx, contextAddress); err != nil {
		t.Fatal(err)
	}
	if got, _ := mem.ReadByte(cpuAddress + z80WasmCPUOffA); got != 0x34 {
		t.Fatalf("A = %02X, want 34", got)
	}
	if got, _ := mem.ReadUint32Le(contextAddress + z80WasmCtxOffNeedBail); got != 0 {
		t.Fatalf("NeedBail = %d, want 0", got)
	}
	if got, _ := mem.ReadUint64Le(contextAddress + z80WasmCtxOffRetCycles); got != 7 {
		t.Fatalf("cycles = %d, want 7", got)
	}
}

func TestZ80WasmCompileBlockDirectHLALUForms(t *testing.T) {
	for _, opcode := range []byte{0x86, 0x8E, 0x96, 0x9E, 0xA6, 0xAE, 0xB6, 0xBE} {
		if _, err := z80WasmCompileBlock([]z80WasmInstr{{opcode: opcode}}, 0x1200); err != nil {
			t.Fatalf("%02X direct (HL) ALU form: %v", opcode, err)
		}
	}
}

func TestZ80WasmCompileBlockDirectLoadFromHL(t *testing.T) {
	module, err := z80WasmCompileBlock([]z80WasmInstr{{opcode: 0x46}}, 0x1200) // LD B,(HL)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	r := wazero.NewRuntime(ctx)
	defer r.Close(ctx)
	envBuilder := newWasmModuleBuilder()
	envBuilder.defineMemory(1)
	envBuilder.exportMemory("mem")
	env, err := r.InstantiateWithConfig(ctx, envBuilder.build(), wazero.NewModuleConfig().WithName("env"))
	if err != nil {
		t.Fatal(err)
	}
	mem := env.ExportedMemory("mem")
	const contextAddress, cpuAddress, ramAddress, pagesAddress = 0x100, 0x200, 0x1000, 0x3000
	if !mem.WriteUint32Le(contextAddress+z80WasmCtxOffCPUPtr, cpuAddress) || !mem.WriteUint32Le(contextAddress+z80WasmCtxOffMemPtr, ramAddress) || !mem.WriteUint32Le(contextAddress+z80WasmCtxOffDirectPageBitmap, pagesAddress) || !mem.WriteByte(cpuAddress+z80WasmCPUOffH, 0x40) || !mem.WriteByte(cpuAddress+z80WasmCPUOffL, 0x34) || !mem.WriteByte(ramAddress+0x4034, 0x7C) {
		t.Fatal("set LD B,(HL) state")
	}
	mod, err := r.Instantiate(ctx, module)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = mod.ExportedFunction("block").Call(ctx, contextAddress); err != nil {
		t.Fatal(err)
	}
	if got, _ := mem.ReadByte(cpuAddress + z80WasmCPUOffB); got != 0x7C {
		t.Fatalf("B = %02X, want 7C", got)
	}
	if got, _ := mem.ReadUint32Le(contextAddress + z80WasmCtxOffNeedBail); got != 0 {
		t.Fatalf("NeedBail = %d, want 0", got)
	}
}

func TestZ80WasmCompileBlockDirectLoadFromPair(t *testing.T) {
	for _, opcode := range []byte{0x0A, 0x1A} { // LD A,(BC)/(DE)
		if _, err := z80WasmCompileBlock([]z80WasmInstr{{opcode: opcode}}, 0x1200); err != nil {
			t.Fatalf("%02X direct pair load: %v", opcode, err)
		}
	}
}

func TestZ80WasmCompileBlockDirectLoadAbsolute(t *testing.T) {
	if _, err := z80WasmCompileBlock([]z80WasmInstr{{opcode: 0x3A, operand: 0x34, operandHi: 0x40}}, 0x1200); err != nil {
		t.Fatalf("LD A,(nn) direct load: %v", err)
	}
}

func TestZ80WasmCompileBlockDirectLoadHLAbsolute(t *testing.T) {
	if _, err := z80WasmCompileBlock([]z80WasmInstr{{opcode: 0x2A, operand: 0x34, operandHi: 0x40}}, 0x1200); err != nil {
		t.Fatalf("LD HL,(nn) direct load: %v", err)
	}
}

func TestZ80WasmCompileBlockLoadHLAbsoluteBailsAtomicallyAtPageBoundary(t *testing.T) {
	module, err := z80WasmCompileBlock([]z80WasmInstr{{opcode: 0x2A, operand: 0xFF, operandHi: 0x1F}}, 0x1200)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	r := wazero.NewRuntime(ctx)
	defer r.Close(ctx)
	envBuilder := newWasmModuleBuilder()
	envBuilder.defineMemory(1)
	envBuilder.exportMemory("mem")
	env, err := r.InstantiateWithConfig(ctx, envBuilder.build(), wazero.NewModuleConfig().WithName("env"))
	if err != nil {
		t.Fatal(err)
	}
	mem := env.ExportedMemory("mem")
	const contextAddress, cpuAddress, ramAddress, pagesAddress = 0x100, 0x200, 0x1000, 0x3000
	if !mem.WriteUint32Le(contextAddress+z80WasmCtxOffCPUPtr, cpuAddress) || !mem.WriteUint32Le(contextAddress+z80WasmCtxOffMemPtr, ramAddress) || !mem.WriteUint32Le(contextAddress+z80WasmCtxOffDirectPageBitmap, pagesAddress) || !mem.WriteByte(cpuAddress+z80WasmCPUOffH, 0xAA) || !mem.WriteByte(cpuAddress+z80WasmCPUOffL, 0x55) || !mem.WriteByte(ramAddress+0x1FFF, 0x34) || !mem.WriteByte(pagesAddress+0x20, 1) {
		t.Fatal("set cross-page LD HL state")
	}
	mod, err := r.Instantiate(ctx, module)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = mod.ExportedFunction("block").Call(ctx, contextAddress); err != nil {
		t.Fatal(err)
	}
	if got, _ := mem.ReadUint32Le(contextAddress + z80WasmCtxOffNeedBail); got != 1 {
		t.Fatalf("NeedBail = %d, want 1", got)
	}
	if got, _ := mem.ReadByte(cpuAddress + z80WasmCPUOffH); got != 0xAA {
		t.Fatalf("H = %02X, want AA", got)
	}
	if got, _ := mem.ReadByte(cpuAddress + z80WasmCPUOffL); got != 0x55 {
		t.Fatalf("L = %02X, want 55", got)
	}
	if got, _ := mem.ReadUint32Le(contextAddress + z80WasmCtxOffRetPC); got != 0x1200 {
		t.Fatalf("RetPC = %04X, want 1200", got)
	}
}

func TestZ80WasmCompileBlockDirectStoreAbsolute(t *testing.T) {
	if _, err := z80WasmCompileBlock([]z80WasmInstr{{opcode: 0x32, operand: 0x34, operandHi: 0x40}}, 0x1200); err != nil {
		t.Fatalf("LD (nn),A direct store: %v", err)
	}
}

func TestZ80WasmCompileBlockDirectStoreToPair(t *testing.T) {
	for _, opcode := range []byte{0x02, 0x12} {
		if _, err := z80WasmCompileBlock([]z80WasmInstr{{opcode: opcode}}, 0x1200); err != nil {
			t.Fatalf("%02X direct pair store: %v", opcode, err)
		}
	}
}

func TestZ80WasmCompileBlockEDLoadIA(t *testing.T) {
	module, err := z80WasmCompileBlock([]z80WasmInstr{{prefix: z80JITPrefixED, opcode: 0x47}}, 0x1200)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	r := wazero.NewRuntime(ctx)
	defer r.Close(ctx)
	envBuilder := newWasmModuleBuilder()
	envBuilder.defineMemory(1)
	envBuilder.exportMemory("mem")
	env, err := r.InstantiateWithConfig(ctx, envBuilder.build(), wazero.NewModuleConfig().WithName("env"))
	if err != nil {
		t.Fatal(err)
	}
	mem := env.ExportedMemory("mem")
	const contextAddress, cpuAddress = 0x100, 0x200
	if !mem.WriteUint32Le(contextAddress+z80WasmCtxOffCPUPtr, cpuAddress) || !mem.WriteByte(cpuAddress+z80WasmCPUOffA, 0x7C) {
		t.Fatal("set ED LD I,A state")
	}
	mod, err := r.Instantiate(ctx, module)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = mod.ExportedFunction("block").Call(ctx, contextAddress); err != nil {
		t.Fatal(err)
	}
	if got, _ := mem.ReadByte(cpuAddress + z80WasmCPUOffI); got != 0x7C {
		t.Fatalf("I = %02X, want 7C", got)
	}
	if got, _ := mem.ReadUint32Le(contextAddress + z80WasmCtxOffRetPC); got != 0x1202 {
		t.Fatalf("RetPC = %04X, want 1202", got)
	}
	if got, _ := mem.ReadUint64Le(contextAddress + z80WasmCtxOffRetCycles); got != 9 {
		t.Fatalf("cycles = %d, want 9", got)
	}
	if got, _ := mem.ReadUint32Le(contextAddress + z80WasmCtxOffRIncrements); got != 2 {
		t.Fatalf("R increments = %d, want 2", got)
	}
}

func TestZ80WasmCompileBlockEDInterruptModes(t *testing.T) {
	for _, tc := range []struct{ opcode, want byte }{{0x46, 0}, {0x56, 1}, {0x5E, 2}} {
		module, err := z80WasmCompileBlock([]z80WasmInstr{{prefix: z80JITPrefixED, opcode: tc.opcode}}, 0x1200)
		if err != nil {
			t.Fatal(err)
		}
		ctx := context.Background()
		r := wazero.NewRuntime(ctx)
		envBuilder := newWasmModuleBuilder()
		envBuilder.defineMemory(1)
		envBuilder.exportMemory("mem")
		env, err := r.InstantiateWithConfig(ctx, envBuilder.build(), wazero.NewModuleConfig().WithName("env"))
		if err != nil {
			t.Fatal(err)
		}
		mem := env.ExportedMemory("mem")
		const contextAddress, cpuAddress = 0x100, 0x200
		if !mem.WriteUint32Le(contextAddress+z80WasmCtxOffCPUPtr, cpuAddress) {
			t.Fatal("set CPU pointer")
		}
		mod, err := r.Instantiate(ctx, module)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = mod.ExportedFunction("block").Call(ctx, contextAddress); err != nil {
			t.Fatal(err)
		}
		if got, _ := mem.ReadByte(cpuAddress + z80WasmCPUOffIM); got != tc.want {
			t.Fatalf("%02X IM = %d, want %d", tc.opcode, got, tc.want)
		}
		r.Close(ctx)
	}
}

func TestZ80WasmCompileBlockEDInterruptModeAliases(t *testing.T) {
	for _, opcode := range []byte{0x46, 0x4E, 0x66, 0x6E, 0x56, 0x76, 0x5E, 0x7E} {
		if _, err := z80WasmCompileBlock([]z80WasmInstr{{prefix: z80JITPrefixED, opcode: opcode}}, 0x1200); err != nil {
			t.Fatalf("ED %02X IM alias: %v", opcode, err)
		}
	}
}

func TestZ80WasmCompileBlockEDNEG(t *testing.T) {
	module, err := z80WasmCompileBlock([]z80WasmInstr{{prefix: z80JITPrefixED, opcode: 0x44}}, 0x1200)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	r := wazero.NewRuntime(ctx)
	defer r.Close(ctx)
	envBuilder := newWasmModuleBuilder()
	envBuilder.defineMemory(1)
	envBuilder.exportMemory("mem")
	env, err := r.InstantiateWithConfig(ctx, envBuilder.build(), wazero.NewModuleConfig().WithName("env"))
	if err != nil {
		t.Fatal(err)
	}
	mem := env.ExportedMemory("mem")
	const contextAddress, cpuAddress = 0x100, 0x200
	if !mem.WriteUint32Le(contextAddress+z80WasmCtxOffCPUPtr, cpuAddress) || !mem.WriteByte(cpuAddress+z80WasmCPUOffA, 1) {
		t.Fatal("set NEG state")
	}
	mod, err := r.Instantiate(ctx, module)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = mod.ExportedFunction("block").Call(ctx, contextAddress); err != nil {
		t.Fatal(err)
	}
	if got, _ := mem.ReadByte(cpuAddress + z80WasmCPUOffA); got != 0xFF {
		t.Fatalf("NEG A = %02X, want FF", got)
	}
	if got, _ := mem.ReadByte(cpuAddress + z80WasmCPUOffF); got != z80FlagS|z80FlagX|z80FlagY|z80FlagH|z80FlagN|z80FlagC {
		t.Fatalf("NEG F = %02X", got)
	}
}

func TestZ80WasmCompileBlockEDNEGAliases(t *testing.T) {
	for _, opcode := range []byte{0x44, 0x4C, 0x54, 0x5C, 0x64, 0x6C, 0x74, 0x7C} {
		if _, err := z80WasmCompileBlock([]z80WasmInstr{{prefix: z80JITPrefixED, opcode: opcode}}, 0x1200); err != nil {
			t.Fatalf("ED %02X NEG alias: %v", opcode, err)
		}
	}
}

func TestZ80WasmCompileBlockEDLoadAI(t *testing.T) {
	module, err := z80WasmCompileBlock([]z80WasmInstr{{prefix: z80JITPrefixED, opcode: 0x57}}, 0x1200)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	r := wazero.NewRuntime(ctx)
	defer r.Close(ctx)
	envBuilder := newWasmModuleBuilder()
	envBuilder.defineMemory(1)
	envBuilder.exportMemory("mem")
	env, err := r.InstantiateWithConfig(ctx, envBuilder.build(), wazero.NewModuleConfig().WithName("env"))
	if err != nil {
		t.Fatal(err)
	}
	mem := env.ExportedMemory("mem")
	const contextAddress, cpuAddress = 0x100, 0x200
	if !mem.WriteUint32Le(contextAddress+z80WasmCtxOffCPUPtr, cpuAddress) || !mem.WriteByte(cpuAddress+z80WasmCPUOffI, 0x80) || !mem.WriteByte(cpuAddress+z80WasmCPUOffF, z80FlagC) || !mem.WriteByte(cpuAddress+z80WasmCPUOffIFF2, 1) {
		t.Fatal("set LD A,I state")
	}
	mod, err := r.Instantiate(ctx, module)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = mod.ExportedFunction("block").Call(ctx, contextAddress); err != nil {
		t.Fatal(err)
	}
	if got, _ := mem.ReadByte(cpuAddress + z80WasmCPUOffA); got != 0x80 {
		t.Fatalf("A = %02X, want 80", got)
	}
	if got, _ := mem.ReadByte(cpuAddress + z80WasmCPUOffF); got != z80FlagS|z80FlagPV|z80FlagC {
		t.Fatalf("F = %02X, want 85", got)
	}
}

func TestZ80WasmCompileBlockIgnoredIndexPrefixNOP(t *testing.T) {
	module, err := z80WasmCompileBlock([]z80WasmInstr{{prefix: z80JITPrefixDD, opcode: 0x00}}, 0x1200)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	r := wazero.NewRuntime(ctx)
	defer r.Close(ctx)
	envBuilder := newWasmModuleBuilder()
	envBuilder.defineMemory(1)
	envBuilder.exportMemory("mem")
	env, err := r.InstantiateWithConfig(ctx, envBuilder.build(), wazero.NewModuleConfig().WithName("env"))
	if err != nil {
		t.Fatal(err)
	}
	mem := env.ExportedMemory("mem")
	const contextAddress, cpuAddress = 0x100, 0x200
	if !mem.WriteUint32Le(contextAddress+z80WasmCtxOffCPUPtr, cpuAddress) {
		t.Fatal("set CPU pointer")
	}
	mod, err := r.Instantiate(ctx, module)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = mod.ExportedFunction("block").Call(ctx, contextAddress); err != nil {
		t.Fatal(err)
	}
	if got, _ := mem.ReadUint32Le(contextAddress + z80WasmCtxOffRetPC); got != 0x1202 {
		t.Fatalf("RetPC = %04X, want 1202", got)
	}
	if got, _ := mem.ReadUint64Le(contextAddress + z80WasmCtxOffRetCycles); got != 8 {
		t.Fatalf("cycles = %d, want 8", got)
	}
	if got, _ := mem.ReadUint32Le(contextAddress + z80WasmCtxOffRIncrements); got != 2 {
		t.Fatalf("R increments = %d, want 2", got)
	}
}

func TestZ80WasmCompileBlockIgnoredIndexPrefixRegisterForms(t *testing.T) {
	for _, opcode := range []byte{0x00, 0x07, 0x0F, 0x17, 0x1F, 0x2F, 0x37, 0x3F, 0x08, 0xD9, 0xEB} {
		if _, err := z80WasmCompileBlock([]z80WasmInstr{{prefix: z80JITPrefixFD, opcode: opcode}}, 0x1200); err != nil {
			t.Fatalf("FD %02X ignored form: %v", opcode, err)
		}
	}
}

func TestZ80WasmCompileBlockIgnoredIndexPrefixLDImm(t *testing.T) {
	if _, err := z80WasmCompileBlock([]z80WasmInstr{{prefix: z80JITPrefixDD, opcode: 0x3E, operand: 0x7C}}, 0x1200); err != nil {
		t.Fatal(err)
	}
	if _, err := z80WasmCompileBlock([]z80WasmInstr{{prefix: z80JITPrefixFD, opcode: 0x26, operand: 0x7C}}, 0x1200); err != nil {
		t.Fatalf("FD LD IYH,n: %v", err)
	}
}

func TestZ80WasmCompileBlockIgnoredIndexPrefixLDReg(t *testing.T) {
	if _, err := z80WasmCompileBlock([]z80WasmInstr{{prefix: z80JITPrefixFD, opcode: 0x78}}, 0x1200); err != nil {
		t.Fatal(err)
	} // LD A,B
	if _, err := z80WasmCompileBlock([]z80WasmInstr{{prefix: z80JITPrefixDD, opcode: 0x7C}}, 0x1200); err != nil {
		t.Fatalf("DD LD A,IXH: %v", err)
	}
}

func TestZ80WasmCompileBlockIgnoredIndexPrefixIncDec(t *testing.T) {
	if _, err := z80WasmCompileBlock([]z80WasmInstr{{prefix: z80JITPrefixDD, opcode: 0x04}}, 0x1200); err != nil {
		t.Fatal(err)
	}
	if _, err := z80WasmCompileBlock([]z80WasmInstr{{prefix: z80JITPrefixFD, opcode: 0x25}}, 0x1200); err != nil {
		t.Fatalf("FD DEC IYH: %v", err)
	}
}

func TestZ80WasmCompileBlockIgnoredIndexPrefixALUReg(t *testing.T) {
	if _, err := z80WasmCompileBlock([]z80WasmInstr{{prefix: z80JITPrefixDD, opcode: 0x80}}, 0x1200); err != nil { // ADD A,B
		t.Fatal(err)
	}
	if _, err := z80WasmCompileBlock([]z80WasmInstr{{prefix: z80JITPrefixFD, opcode: 0x84}}, 0x1200); err != nil { // ADD A,IYH
		t.Fatalf("FD ADD A,IYH: %v", err)
	}
}

func TestZ80WasmCompileBlockIgnoredIndexPrefixPairs(t *testing.T) {
	for _, opcode := range []byte{0x01, 0x13, 0x3B} { // LD BC,nn; INC DE; DEC SP
		if _, err := z80WasmCompileBlock([]z80WasmInstr{{prefix: z80JITPrefixDD, opcode: opcode, operand: 0x34, operandHi: 0x12}}, 0x1200); err != nil {
			t.Fatalf("DD %02X: %v", opcode, err)
		}
	}
	if _, err := z80WasmCompileBlock([]z80WasmInstr{{prefix: z80JITPrefixFD, opcode: 0x21, operand: 0x34, operandHi: 0x12}}, 0x1200); err != nil {
		t.Fatalf("FD LD IY,nn: %v", err)
	}
}

func TestZ80WasmCompileBlockCBSetResRegister(t *testing.T) {
	for _, opcode := range []byte{0x80, 0xF9} { // RES 0,B; SET 7,C
		if _, err := z80WasmCompileBlock([]z80WasmInstr{{prefix: z80JITPrefixCB, opcode: opcode}}, 0x1200); err != nil {
			t.Fatalf("CB %02X: %v", opcode, err)
		}
	}
	if _, err := z80WasmCompileBlock([]z80WasmInstr{{prefix: z80JITPrefixCB, opcode: 0x86}}, 0x1200); err != nil {
		t.Fatalf("guarded RES 0,(HL): %v", err)
	}
}

func TestZ80WasmCBSetResRegisterExecutes(t *testing.T) {
	module, err := z80WasmCompileBlock([]z80WasmInstr{
		{prefix: z80JITPrefixCB, opcode: 0x80}, // RES 0,B
		{prefix: z80JITPrefixCB, opcode: 0xF9}, // SET 7,C
	}, 0x1200)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	r := wazero.NewRuntime(ctx)
	defer r.Close(ctx)
	envBuilder := newWasmModuleBuilder()
	envBuilder.defineMemory(1)
	envBuilder.exportMemory("mem")
	env, err := r.InstantiateWithConfig(ctx, envBuilder.build(), wazero.NewModuleConfig().WithName("env"))
	if err != nil {
		t.Fatal(err)
	}
	mem := env.ExportedMemory("mem")
	const contextAddress, cpuAddress = 0x100, 0x200
	if !mem.WriteUint32Le(contextAddress+z80WasmCtxOffCPUPtr, cpuAddress) || !mem.WriteByte(cpuAddress+z80WasmCPUOffB, 0x03) || !mem.WriteByte(cpuAddress+z80WasmCPUOffC, 0x01) || !mem.WriteByte(cpuAddress+z80WasmCPUOffF, z80FlagC) {
		t.Fatal("set CB register state")
	}
	mod, err := r.Instantiate(ctx, module)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = mod.ExportedFunction("block").Call(ctx, contextAddress); err != nil {
		t.Fatal(err)
	}
	if got, _ := mem.ReadByte(cpuAddress + z80WasmCPUOffB); got != 0x02 {
		t.Fatalf("RES 0,B = %02X, want 02", got)
	}
	if got, _ := mem.ReadByte(cpuAddress + z80WasmCPUOffC); got != 0x81 {
		t.Fatalf("SET 7,C = %02X, want 81", got)
	}
	if got, _ := mem.ReadByte(cpuAddress + z80WasmCPUOffF); got != z80FlagC {
		t.Fatalf("CB SET/RES changed F = %02X", got)
	}
	if got, _ := mem.ReadUint32Le(contextAddress + z80WasmCtxOffRetPC); got != 0x1204 {
		t.Fatalf("PC = %04X, want 1204", got)
	}
	if got, _ := mem.ReadUint64Le(contextAddress + z80WasmCtxOffRetCycles); got != 16 {
		t.Fatalf("cycles = %d, want 16", got)
	}
	if got, _ := mem.ReadUint32Le(contextAddress + z80WasmCtxOffRIncrements); got != 4 {
		t.Fatalf("R increments = %d, want 4", got)
	}
}

func TestZ80WasmCompileBlockCBSetResHL(t *testing.T) {
	for _, opcode := range []byte{0x86, 0xFE} { // RES 0,(HL); SET 7,(HL)
		if _, err := z80WasmCompileBlock([]z80WasmInstr{{prefix: z80JITPrefixCB, opcode: opcode}}, 0x1200); err != nil {
			t.Fatalf("CB %02X: %v", opcode, err)
		}
	}
}

func TestZ80WasmCBResHLExecutes(t *testing.T) {
	module, err := z80WasmCompileBlock([]z80WasmInstr{{prefix: z80JITPrefixCB, opcode: 0x86}}, 0x1200) // RES 0,(HL)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	r := wazero.NewRuntime(ctx)
	defer r.Close(ctx)
	envBuilder := newWasmModuleBuilder()
	envBuilder.defineMemory(1)
	envBuilder.exportMemory("mem")
	env, err := r.InstantiateWithConfig(ctx, envBuilder.build(), wazero.NewModuleConfig().WithName("env"))
	if err != nil {
		t.Fatal(err)
	}
	mem := env.ExportedMemory("mem")
	const contextAddress, cpuAddress, ramAddress, pagesAddress, codePagesAddress = 0x100, 0x200, 0x1000, 0x3000, 0x3100
	if !mem.WriteUint32Le(contextAddress+z80WasmCtxOffCPUPtr, cpuAddress) || !mem.WriteUint32Le(contextAddress+z80WasmCtxOffMemPtr, ramAddress) || !mem.WriteUint32Le(contextAddress+z80WasmCtxOffDirectPageBitmap, pagesAddress) || !mem.WriteUint32Le(contextAddress+z80WasmCtxOffCodePageBitmap, codePagesAddress) || !mem.WriteByte(cpuAddress+z80WasmCPUOffH, 0x40) || !mem.WriteByte(cpuAddress+z80WasmCPUOffL, 0x34) || !mem.WriteByte(ramAddress+0x4034, 0xFF) {
		t.Fatal("set RES (HL) state")
	}
	mod, err := r.Instantiate(ctx, module)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = mod.ExportedFunction("block").Call(ctx, contextAddress); err != nil {
		t.Fatal(err)
	}
	if got, _ := mem.ReadByte(ramAddress + 0x4034); got != 0xFE {
		t.Fatalf("RES 0,(HL) = %02X, want FE", got)
	}
	if got, _ := mem.ReadUint32Le(contextAddress + z80WasmCtxOffRetPC); got != 0x1202 {
		t.Fatalf("PC = %04X, want 1202", got)
	}
	if got, _ := mem.ReadUint64Le(contextAddress + z80WasmCtxOffRetCycles); got != 15 {
		t.Fatalf("cycles = %d, want 15", got)
	}
	if got, _ := mem.ReadUint32Le(contextAddress + z80WasmCtxOffRIncrements); got != 2 {
		t.Fatalf("R increments = %d, want 2", got)
	}
}

func TestZ80WasmCBSetResHLInvalidatesCodePage(t *testing.T) {
	module, err := z80WasmCompileBlock([]z80WasmInstr{{prefix: z80JITPrefixCB, opcode: 0x86}}, 0x1200)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	r := wazero.NewRuntime(ctx)
	defer r.Close(ctx)
	envBuilder := newWasmModuleBuilder()
	envBuilder.defineMemory(1)
	envBuilder.exportMemory("mem")
	env, err := r.InstantiateWithConfig(ctx, envBuilder.build(), wazero.NewModuleConfig().WithName("env"))
	if err != nil {
		t.Fatal(err)
	}
	mem := env.ExportedMemory("mem")
	const contextAddress, cpuAddress, ramAddress, pagesAddress, codePagesAddress = 0x100, 0x200, 0x1000, 0x3000, 0x3100
	if !mem.WriteUint32Le(contextAddress+z80WasmCtxOffCPUPtr, cpuAddress) || !mem.WriteUint32Le(contextAddress+z80WasmCtxOffMemPtr, ramAddress) || !mem.WriteUint32Le(contextAddress+z80WasmCtxOffDirectPageBitmap, pagesAddress) || !mem.WriteUint32Le(contextAddress+z80WasmCtxOffCodePageBitmap, codePagesAddress) || !mem.WriteByte(cpuAddress+z80WasmCPUOffH, 0x40) || !mem.WriteByte(cpuAddress+z80WasmCPUOffL, 0x34) || !mem.WriteByte(codePagesAddress+0x40, 1) || !mem.WriteByte(ramAddress+0x4034, 0xFF) {
		t.Fatal("set SMC RES (HL) state")
	}
	mod, err := r.Instantiate(ctx, module)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = mod.ExportedFunction("block").Call(ctx, contextAddress); err != nil {
		t.Fatal(err)
	}
	if got, _ := mem.ReadByte(ramAddress + 0x4034); got != 0xFE {
		t.Fatalf("write was not committed: %02X", got)
	}
	if got, _ := mem.ReadUint32Le(contextAddress + z80WasmCtxOffNeedInval); got != 1 {
		t.Fatalf("NeedInval = %d, want 1", got)
	}
	if got, _ := mem.ReadUint32Le(contextAddress + z80WasmCtxOffInvalPage); got != 0x40 {
		t.Fatalf("InvalPage = %02X, want 40", got)
	}
}

func TestZ80WasmCBBITRegisterExecutes(t *testing.T) {
	module, err := z80WasmCompileBlock([]z80WasmInstr{{prefix: z80JITPrefixCB, opcode: 0x40}}, 0x1200) // BIT 0,B
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	r := wazero.NewRuntime(ctx)
	defer r.Close(ctx)
	envBuilder := newWasmModuleBuilder()
	envBuilder.defineMemory(1)
	envBuilder.exportMemory("mem")
	env, err := r.InstantiateWithConfig(ctx, envBuilder.build(), wazero.NewModuleConfig().WithName("env"))
	if err != nil {
		t.Fatal(err)
	}
	mem := env.ExportedMemory("mem")
	const contextAddress, cpuAddress = 0x100, 0x200
	if !mem.WriteUint32Le(contextAddress+z80WasmCtxOffCPUPtr, cpuAddress) || !mem.WriteByte(cpuAddress+z80WasmCPUOffB, 0x28) || !mem.WriteByte(cpuAddress+z80WasmCPUOffF, z80FlagC) {
		t.Fatal("set BIT state")
	}
	mod, err := r.Instantiate(ctx, module)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = mod.ExportedFunction("block").Call(ctx, contextAddress); err != nil {
		t.Fatal(err)
	}
	if got, _ := mem.ReadByte(cpuAddress + z80WasmCPUOffF); got != z80FlagZ|z80FlagPV|z80FlagH|z80FlagX|z80FlagY|z80FlagC {
		t.Fatalf("BIT 0,B F = %02X", got)
	}
}

func TestZ80WasmCBRLCRegisterExecutes(t *testing.T) {
	module, err := z80WasmCompileBlock([]z80WasmInstr{{prefix: z80JITPrefixCB, opcode: 0x00}}, 0x1200) // RLC B
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	r := wazero.NewRuntime(ctx)
	defer r.Close(ctx)
	envBuilder := newWasmModuleBuilder()
	envBuilder.defineMemory(1)
	envBuilder.exportMemory("mem")
	env, err := r.InstantiateWithConfig(ctx, envBuilder.build(), wazero.NewModuleConfig().WithName("env"))
	if err != nil {
		t.Fatal(err)
	}
	mem := env.ExportedMemory("mem")
	const contextAddress, cpuAddress = 0x100, 0x200
	if !mem.WriteUint32Le(contextAddress+z80WasmCtxOffCPUPtr, cpuAddress) || !mem.WriteByte(cpuAddress+z80WasmCPUOffB, 0x81) {
		t.Fatal("set RLC state")
	}
	mod, err := r.Instantiate(ctx, module)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = mod.ExportedFunction("block").Call(ctx, contextAddress); err != nil {
		t.Fatal(err)
	}
	if got, _ := mem.ReadByte(cpuAddress + z80WasmCPUOffB); got != 0x03 {
		t.Fatalf("RLC B = %02X, want 03", got)
	}
	if got, _ := mem.ReadByte(cpuAddress + z80WasmCPUOffF); got != z80FlagPV|z80FlagC {
		t.Fatalf("RLC B F = %02X, want 05", got)
	}
}

func TestZ80WasmCBRRCRegisterExecutes(t *testing.T) {
	module, err := z80WasmCompileBlock([]z80WasmInstr{{prefix: z80JITPrefixCB, opcode: 0x08}}, 0x1200) // RRC B
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	r := wazero.NewRuntime(ctx)
	defer r.Close(ctx)
	envBuilder := newWasmModuleBuilder()
	envBuilder.defineMemory(1)
	envBuilder.exportMemory("mem")
	env, err := r.InstantiateWithConfig(ctx, envBuilder.build(), wazero.NewModuleConfig().WithName("env"))
	if err != nil {
		t.Fatal(err)
	}
	mem := env.ExportedMemory("mem")
	const contextAddress, cpuAddress = 0x100, 0x200
	if !mem.WriteUint32Le(contextAddress+z80WasmCtxOffCPUPtr, cpuAddress) || !mem.WriteByte(cpuAddress+z80WasmCPUOffB, 0x03) {
		t.Fatal("set RRC state")
	}
	mod, err := r.Instantiate(ctx, module)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = mod.ExportedFunction("block").Call(ctx, contextAddress); err != nil {
		t.Fatal(err)
	}
	if got, _ := mem.ReadByte(cpuAddress + z80WasmCPUOffB); got != 0x81 {
		t.Fatalf("RRC B = %02X, want 81", got)
	}
	if got, _ := mem.ReadByte(cpuAddress + z80WasmCPUOffF); got != z80FlagS|z80FlagPV|z80FlagC {
		t.Fatalf("RRC B F = %02X, want 85", got)
	}
}

func TestZ80WasmCBSLARegisterExecutes(t *testing.T) {
	module, err := z80WasmCompileBlock([]z80WasmInstr{{prefix: z80JITPrefixCB, opcode: 0x20}}, 0x1200) // SLA B
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	r := wazero.NewRuntime(ctx)
	defer r.Close(ctx)
	envBuilder := newWasmModuleBuilder()
	envBuilder.defineMemory(1)
	envBuilder.exportMemory("mem")
	env, err := r.InstantiateWithConfig(ctx, envBuilder.build(), wazero.NewModuleConfig().WithName("env"))
	if err != nil {
		t.Fatal(err)
	}
	mem := env.ExportedMemory("mem")
	const contextAddress, cpuAddress = 0x100, 0x200
	if !mem.WriteUint32Le(contextAddress+z80WasmCtxOffCPUPtr, cpuAddress) || !mem.WriteByte(cpuAddress+z80WasmCPUOffB, 0x81) {
		t.Fatal("set SLA state")
	}
	mod, err := r.Instantiate(ctx, module)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = mod.ExportedFunction("block").Call(ctx, contextAddress); err != nil {
		t.Fatal(err)
	}
	if got, _ := mem.ReadByte(cpuAddress + z80WasmCPUOffB); got != 0x02 {
		t.Fatalf("SLA B = %02X, want 02", got)
	}
	if got, _ := mem.ReadByte(cpuAddress + z80WasmCPUOffF); got != z80FlagC {
		t.Fatalf("SLA B F = %02X, want 01", got)
	}
}

func TestZ80WasmCBSRLRegisterExecutes(t *testing.T) {
	module, err := z80WasmCompileBlock([]z80WasmInstr{{prefix: z80JITPrefixCB, opcode: 0x38}}, 0x1200) // SRL B
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	r := wazero.NewRuntime(ctx)
	defer r.Close(ctx)
	envBuilder := newWasmModuleBuilder()
	envBuilder.defineMemory(1)
	envBuilder.exportMemory("mem")
	env, err := r.InstantiateWithConfig(ctx, envBuilder.build(), wazero.NewModuleConfig().WithName("env"))
	if err != nil {
		t.Fatal(err)
	}
	mem := env.ExportedMemory("mem")
	const contextAddress, cpuAddress = 0x100, 0x200
	if !mem.WriteUint32Le(contextAddress+z80WasmCtxOffCPUPtr, cpuAddress) || !mem.WriteByte(cpuAddress+z80WasmCPUOffB, 0x81) {
		t.Fatal("set SRL state")
	}
	mod, err := r.Instantiate(ctx, module)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = mod.ExportedFunction("block").Call(ctx, contextAddress); err != nil {
		t.Fatal(err)
	}
	if got, _ := mem.ReadByte(cpuAddress + z80WasmCPUOffB); got != 0x40 {
		t.Fatalf("SRL B = %02X, want 40", got)
	}
	if got, _ := mem.ReadByte(cpuAddress + z80WasmCPUOffF); got != z80FlagC {
		t.Fatalf("SRL B F = %02X, want 01", got)
	}
}

func TestZ80WasmCBSRARegisterExecutes(t *testing.T) {
	module, err := z80WasmCompileBlock([]z80WasmInstr{{prefix: z80JITPrefixCB, opcode: 0x28}}, 0x1200) // SRA B
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	r := wazero.NewRuntime(ctx)
	defer r.Close(ctx)
	envBuilder := newWasmModuleBuilder()
	envBuilder.defineMemory(1)
	envBuilder.exportMemory("mem")
	env, err := r.InstantiateWithConfig(ctx, envBuilder.build(), wazero.NewModuleConfig().WithName("env"))
	if err != nil {
		t.Fatal(err)
	}
	mem := env.ExportedMemory("mem")
	const contextAddress, cpuAddress = 0x100, 0x200
	if !mem.WriteUint32Le(contextAddress+z80WasmCtxOffCPUPtr, cpuAddress) || !mem.WriteByte(cpuAddress+z80WasmCPUOffB, 0x81) {
		t.Fatal("set SRA state")
	}
	mod, err := r.Instantiate(ctx, module)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = mod.ExportedFunction("block").Call(ctx, contextAddress); err != nil {
		t.Fatal(err)
	}
	if got, _ := mem.ReadByte(cpuAddress + z80WasmCPUOffB); got != 0xC0 {
		t.Fatalf("SRA B = %02X, want C0", got)
	}
	if got, _ := mem.ReadByte(cpuAddress + z80WasmCPUOffF); got != z80FlagS|z80FlagPV|z80FlagC {
		t.Fatalf("SRA B F = %02X, want 85", got)
	}
}

func TestZ80WasmCBSLLRegisterExecutes(t *testing.T) {
	module, err := z80WasmCompileBlock([]z80WasmInstr{{prefix: z80JITPrefixCB, opcode: 0x30}}, 0x1200) // SLL B
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	r := wazero.NewRuntime(ctx)
	defer r.Close(ctx)
	envBuilder := newWasmModuleBuilder()
	envBuilder.defineMemory(1)
	envBuilder.exportMemory("mem")
	env, err := r.InstantiateWithConfig(ctx, envBuilder.build(), wazero.NewModuleConfig().WithName("env"))
	if err != nil {
		t.Fatal(err)
	}
	mem := env.ExportedMemory("mem")
	const contextAddress, cpuAddress = 0x100, 0x200
	if !mem.WriteUint32Le(contextAddress+z80WasmCtxOffCPUPtr, cpuAddress) || !mem.WriteByte(cpuAddress+z80WasmCPUOffB, 0x80) {
		t.Fatal("set SLL state")
	}
	mod, err := r.Instantiate(ctx, module)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = mod.ExportedFunction("block").Call(ctx, contextAddress); err != nil {
		t.Fatal(err)
	}
	if got, _ := mem.ReadByte(cpuAddress + z80WasmCPUOffB); got != 0x01 {
		t.Fatalf("SLL B = %02X, want 01", got)
	}
	if got, _ := mem.ReadByte(cpuAddress + z80WasmCPUOffF); got != z80FlagC {
		t.Fatalf("SLL B F = %02X, want 01", got)
	}
}

func TestZ80WasmCBRLRegisterExecutes(t *testing.T) {
	module, err := z80WasmCompileBlock([]z80WasmInstr{{prefix: z80JITPrefixCB, opcode: 0x10}}, 0x1200) // RL B
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	r := wazero.NewRuntime(ctx)
	defer r.Close(ctx)
	envBuilder := newWasmModuleBuilder()
	envBuilder.defineMemory(1)
	envBuilder.exportMemory("mem")
	env, err := r.InstantiateWithConfig(ctx, envBuilder.build(), wazero.NewModuleConfig().WithName("env"))
	if err != nil {
		t.Fatal(err)
	}
	mem := env.ExportedMemory("mem")
	const contextAddress, cpuAddress = 0x100, 0x200
	if !mem.WriteUint32Le(contextAddress+z80WasmCtxOffCPUPtr, cpuAddress) || !mem.WriteByte(cpuAddress+z80WasmCPUOffB, 0x80) || !mem.WriteByte(cpuAddress+z80WasmCPUOffF, z80FlagC) {
		t.Fatal("set RL state")
	}
	mod, err := r.Instantiate(ctx, module)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = mod.ExportedFunction("block").Call(ctx, contextAddress); err != nil {
		t.Fatal(err)
	}
	if got, _ := mem.ReadByte(cpuAddress + z80WasmCPUOffB); got != 0x01 {
		t.Fatalf("RL B = %02X, want 01", got)
	}
	if got, _ := mem.ReadByte(cpuAddress + z80WasmCPUOffF); got != z80FlagC {
		t.Fatalf("RL B F = %02X, want 01", got)
	}
}

func TestZ80WasmCBRRRegisterExecutes(t *testing.T) {
	module, err := z80WasmCompileBlock([]z80WasmInstr{{prefix: z80JITPrefixCB, opcode: 0x18}}, 0x1200) // RR B
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	r := wazero.NewRuntime(ctx)
	defer r.Close(ctx)
	envBuilder := newWasmModuleBuilder()
	envBuilder.defineMemory(1)
	envBuilder.exportMemory("mem")
	env, err := r.InstantiateWithConfig(ctx, envBuilder.build(), wazero.NewModuleConfig().WithName("env"))
	if err != nil {
		t.Fatal(err)
	}
	mem := env.ExportedMemory("mem")
	const contextAddress, cpuAddress = 0x100, 0x200
	if !mem.WriteUint32Le(contextAddress+z80WasmCtxOffCPUPtr, cpuAddress) || !mem.WriteByte(cpuAddress+z80WasmCPUOffB, 0x01) || !mem.WriteByte(cpuAddress+z80WasmCPUOffF, z80FlagC) {
		t.Fatal("set RR state")
	}
	mod, err := r.Instantiate(ctx, module)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = mod.ExportedFunction("block").Call(ctx, contextAddress); err != nil {
		t.Fatal(err)
	}
	if got, _ := mem.ReadByte(cpuAddress + z80WasmCPUOffB); got != 0x80 {
		t.Fatalf("RR B = %02X, want 80", got)
	}
	if got, _ := mem.ReadByte(cpuAddress + z80WasmCPUOffF); got != z80FlagS|z80FlagC {
		t.Fatalf("RR B F = %02X, want 81", got)
	}
}

func TestZ80WasmCompileBlockDirectStoreToHL(t *testing.T) {
	module, err := z80WasmCompileBlock([]z80WasmInstr{{opcode: 0x70}}, 0x1200) // LD (HL),B
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	r := wazero.NewRuntime(ctx)
	defer r.Close(ctx)
	envBuilder := newWasmModuleBuilder()
	envBuilder.defineMemory(1)
	envBuilder.exportMemory("mem")
	env, err := r.InstantiateWithConfig(ctx, envBuilder.build(), wazero.NewModuleConfig().WithName("env"))
	if err != nil {
		t.Fatal(err)
	}
	mem := env.ExportedMemory("mem")
	const contextAddress, cpuAddress, ramAddress, pagesAddress, codePagesAddress = 0x100, 0x200, 0x1000, 0x3000, 0x3100
	if !mem.WriteUint32Le(contextAddress+z80WasmCtxOffCPUPtr, cpuAddress) || !mem.WriteUint32Le(contextAddress+z80WasmCtxOffMemPtr, ramAddress) || !mem.WriteUint32Le(contextAddress+z80WasmCtxOffDirectPageBitmap, pagesAddress) || !mem.WriteUint32Le(contextAddress+z80WasmCtxOffCodePageBitmap, codePagesAddress) || !mem.WriteByte(cpuAddress+z80WasmCPUOffH, 0x40) || !mem.WriteByte(cpuAddress+z80WasmCPUOffL, 0x34) || !mem.WriteByte(cpuAddress+z80WasmCPUOffB, 0x7C) {
		t.Fatal("set LD (HL),B state")
	}
	mod, err := r.Instantiate(ctx, module)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = mod.ExportedFunction("block").Call(ctx, contextAddress); err != nil {
		t.Fatal(err)
	}
	if got, _ := mem.ReadByte(ramAddress + 0x4034); got != 0x7C {
		t.Fatalf("stored = %02X, want 7C", got)
	}
	if got, _ := mem.ReadUint32Le(contextAddress + z80WasmCtxOffNeedInval); got != 0 {
		t.Fatalf("NeedInval = %d, want 0", got)
	}
}

func TestZ80WasmCompileBlockDirectStoreImmediateToHL(t *testing.T) {
	if _, err := z80WasmCompileBlock([]z80WasmInstr{{opcode: 0x36, operand: 0x7C}}, 0x1200); err != nil {
		t.Fatalf("LD (HL),n direct store: %v", err)
	}
}

func TestZ80WasmCompileBlockStoreToHLInvalidatesCodePageAfterWrite(t *testing.T) {
	module, err := z80WasmCompileBlock([]z80WasmInstr{{opcode: 0x70}}, 0x1200) // LD (HL),B
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	r := wazero.NewRuntime(ctx)
	defer r.Close(ctx)
	envBuilder := newWasmModuleBuilder()
	envBuilder.defineMemory(1)
	envBuilder.exportMemory("mem")
	env, err := r.InstantiateWithConfig(ctx, envBuilder.build(), wazero.NewModuleConfig().WithName("env"))
	if err != nil {
		t.Fatal(err)
	}
	mem := env.ExportedMemory("mem")
	const contextAddress, cpuAddress, ramAddress, pagesAddress, codePagesAddress = 0x100, 0x200, 0x1000, 0x3000, 0x3100
	if !mem.WriteUint32Le(contextAddress+z80WasmCtxOffCPUPtr, cpuAddress) || !mem.WriteUint32Le(contextAddress+z80WasmCtxOffMemPtr, ramAddress) || !mem.WriteUint32Le(contextAddress+z80WasmCtxOffDirectPageBitmap, pagesAddress) || !mem.WriteUint32Le(contextAddress+z80WasmCtxOffCodePageBitmap, codePagesAddress) || !mem.WriteByte(cpuAddress+z80WasmCPUOffH, 0x40) || !mem.WriteByte(cpuAddress+z80WasmCPUOffL, 0x34) || !mem.WriteByte(cpuAddress+z80WasmCPUOffB, 0x7C) || !mem.WriteByte(codePagesAddress+0x40, 1) {
		t.Fatal("set code-page store state")
	}
	mod, err := r.Instantiate(ctx, module)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = mod.ExportedFunction("block").Call(ctx, contextAddress); err != nil {
		t.Fatal(err)
	}
	if got, _ := mem.ReadByte(ramAddress + 0x4034); got != 0x7C {
		t.Fatalf("stored = %02X, want 7C", got)
	}
	if got, _ := mem.ReadUint32Le(contextAddress + z80WasmCtxOffNeedInval); got != 1 {
		t.Fatalf("NeedInval = %d, want 1", got)
	}
	if got, _ := mem.ReadUint32Le(contextAddress + z80WasmCtxOffInvalPage); got != 0x40 {
		t.Fatalf("InvalPage = %02X, want 40", got)
	}
	if got, _ := mem.ReadUint32Le(contextAddress + z80WasmCtxOffRetPC); got != 0x1201 {
		t.Fatalf("RetPC = %04X, want 1201", got)
	}
	if got, _ := mem.ReadUint64Le(contextAddress + z80WasmCtxOffRetCycles); got != 7 {
		t.Fatalf("cycles = %d, want 7", got)
	}
	if got, _ := mem.ReadUint32Le(contextAddress + z80WasmCtxOffRetCount); got != 1 {
		t.Fatalf("count = %d, want 1", got)
	}
}

func TestZ80WasmCompileBlockAddHLBailsBeforeNonDirectRead(t *testing.T) {
	module, err := z80WasmCompileBlock([]z80WasmInstr{{opcode: 0x3E, operand: 0x12}, {opcode: 0x86}}, 0x1200)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	r := wazero.NewRuntime(ctx)
	defer r.Close(ctx)
	envBuilder := newWasmModuleBuilder()
	envBuilder.defineMemory(1)
	envBuilder.exportMemory("mem")
	env, err := r.InstantiateWithConfig(ctx, envBuilder.build(), wazero.NewModuleConfig().WithName("env"))
	if err != nil {
		t.Fatal(err)
	}
	mem := env.ExportedMemory("mem")
	const contextAddress, cpuAddress, ramAddress, pagesAddress = 0x100, 0x200, 0x1000, 0x3000
	if !mem.WriteUint32Le(contextAddress+z80WasmCtxOffCPUPtr, cpuAddress) || !mem.WriteUint32Le(contextAddress+z80WasmCtxOffMemPtr, ramAddress) || !mem.WriteUint32Le(contextAddress+z80WasmCtxOffDirectPageBitmap, pagesAddress) || !mem.WriteByte(cpuAddress+z80WasmCPUOffH, 0x40) || !mem.WriteByte(cpuAddress+z80WasmCPUOffL, 0x34) || !mem.WriteByte(pagesAddress+0x40, 1) {
		t.Fatal("set bailout ADD A,(HL) state")
	}
	mod, err := r.Instantiate(ctx, module)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = mod.ExportedFunction("block").Call(ctx, contextAddress); err != nil {
		t.Fatal(err)
	}
	if got, _ := mem.ReadByte(cpuAddress + z80WasmCPUOffA); got != 0x12 {
		t.Fatalf("A = %02X, want prefix result 12", got)
	}
	if got, _ := mem.ReadUint32Le(contextAddress + z80WasmCtxOffNeedBail); got != 1 {
		t.Fatalf("NeedBail = %d, want 1", got)
	}
	if got, _ := mem.ReadUint32Le(contextAddress + z80WasmCtxOffRetPC); got != 0x1202 {
		t.Fatalf("RetPC = %04X, want 1202", got)
	}
	if got, _ := mem.ReadUint64Le(contextAddress + z80WasmCtxOffRetCycles); got != 7 {
		t.Fatalf("cycles = %d, want 7", got)
	}
	if got, _ := mem.ReadUint32Le(contextAddress + z80WasmCtxOffRetCount); got != 1 {
		t.Fatalf("count = %d, want 1", got)
	}
	if got, _ := mem.ReadUint32Le(contextAddress + z80WasmCtxOffRIncrements); got != 1 {
		t.Fatalf("R increments = %d, want 1", got)
	}
}

func z80WasmCompileAddALUTest(t *testing.T, instr z80WasmInstr, a, b, flags, wantA, wantF byte) {
	t.Helper()
	module, err := z80WasmCompileBlock([]z80WasmInstr{instr}, 0x1200)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	r := wazero.NewRuntime(ctx)
	defer r.Close(ctx)
	envBuilder := newWasmModuleBuilder()
	envBuilder.defineMemory(1)
	envBuilder.exportMemory("mem")
	env, err := r.InstantiateWithConfig(ctx, envBuilder.build(), wazero.NewModuleConfig().WithName("env"))
	if err != nil {
		t.Fatal(err)
	}
	mem := env.ExportedMemory("mem")
	const contextAddress, cpuAddress = 0x100, 0x200
	if !mem.WriteUint32Le(contextAddress+z80WasmCtxOffCPUPtr, cpuAddress) ||
		!mem.WriteByte(cpuAddress+z80WasmCPUOffA, a) ||
		!mem.WriteByte(cpuAddress+z80WasmCPUOffB, b) ||
		!mem.WriteByte(cpuAddress+z80WasmCPUOffF, flags) {
		t.Fatal("set ADD state")
	}
	mod, err := r.Instantiate(ctx, module)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = mod.ExportedFunction("block").Call(ctx, contextAddress); err != nil {
		t.Fatal(err)
	}
	if got, _ := mem.ReadByte(cpuAddress + z80WasmCPUOffA); got != wantA {
		t.Fatalf("A = %02X, want %02X", got, wantA)
	}
	if got, _ := mem.ReadByte(cpuAddress + z80WasmCPUOffF); got != wantF {
		t.Fatalf("F = %02X, want %02X", got, wantF)
	}
}

func TestZ80WasmCompileBlockDirectStaticJump(t *testing.T) {
	module, err := z80WasmCompileBlock([]z80WasmInstr{{opcode: 0xC3, operand: 0x34, operandHi: 0x12}}, 0x0200)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	r := wazero.NewRuntime(ctx)
	defer r.Close(ctx)
	envBuilder := newWasmModuleBuilder()
	envBuilder.defineMemory(1)
	envBuilder.exportMemory("mem")
	env, err := r.InstantiateWithConfig(ctx, envBuilder.build(), wazero.NewModuleConfig().WithName("env"))
	if err != nil {
		t.Fatal(err)
	}
	mod, err := r.Instantiate(ctx, module)
	if err != nil {
		t.Fatal(err)
	}
	const contextAddress = 0x100
	if _, err = mod.ExportedFunction("block").Call(ctx, contextAddress); err != nil {
		t.Fatal(err)
	}
	if got, ok := env.ExportedMemory("mem").ReadUint32Le(contextAddress + z80WasmCtxOffRetPC); !ok || got != 0x1234 {
		t.Fatalf("PC = %04X, ok=%v", got, ok)
	}
}

func TestZ80WasmCompileBlockDirectIndirectJump(t *testing.T) {
	module, err := z80WasmCompileBlock([]z80WasmInstr{{opcode: 0xE9}}, 0x0200) // JP (HL)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	r := wazero.NewRuntime(ctx)
	defer r.Close(ctx)
	envBuilder := newWasmModuleBuilder()
	envBuilder.defineMemory(1)
	envBuilder.exportMemory("mem")
	env, err := r.InstantiateWithConfig(ctx, envBuilder.build(), wazero.NewModuleConfig().WithName("env"))
	if err != nil {
		t.Fatal(err)
	}
	mem := env.ExportedMemory("mem")
	const contextAddress, cpuAddress = 0x100, 0x200
	if !mem.WriteUint32Le(contextAddress+z80WasmCtxOffCPUPtr, cpuAddress) ||
		!mem.WriteByte(cpuAddress+z80WasmCPUOffH, 0x12) || !mem.WriteByte(cpuAddress+z80WasmCPUOffL, 0x34) {
		t.Fatal("set JP (HL) state")
	}
	mod, err := r.Instantiate(ctx, module)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = mod.ExportedFunction("block").Call(ctx, contextAddress); err != nil {
		t.Fatal(err)
	}
	if got, ok := mem.ReadUint32Le(contextAddress + z80WasmCtxOffRetPC); !ok || got != 0x1234 {
		t.Fatalf("JP (HL) PC = %04X, ok=%v", got, ok)
	}
}

func TestZ80WasmCompileBlockRejectsObservationBoundary(t *testing.T) {
	for _, opcode := range []byte{0x76, 0xD3, 0xDB, 0xCB, 0xDD, 0xED, 0xFD} {
		if _, err := z80WasmCompileBlock([]z80WasmInstr{{opcode: opcode}}, 0); err == nil {
			t.Errorf("%02X compiled despite requiring a helper or halt boundary", opcode)
		}
	}
}
