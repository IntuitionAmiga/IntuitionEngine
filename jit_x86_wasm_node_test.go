//go:build !js

package main

import (
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"math"
	"os/exec"
	"testing"
)

func requireNode(t testing.TB) string {
	t.Helper()
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not found in PATH")
	}
	return node
}

func runNodeJSON(t testing.TB, script string, args ...string) map[string]uint32 {
	t.Helper()
	node := requireNode(t)
	cmdArgs := append([]string{"-e", script}, args...)
	out, err := exec.Command(node, cmdArgs...).CombinedOutput()
	if err != nil {
		t.Fatalf("node failed: %v\n%s", err, out)
	}
	var got map[string]uint32
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("decode node JSON: %v\n%s", err, out)
	}
	return got
}

func TestX86WasmNode_InstantiatesDirectX87SIMDBlock(t *testing.T) {
	const startPC = uint32(0x1000)
	mem := make([]byte, 0x2000)
	copy(mem[startPC:], []byte{0xD8, 0xC1}) // FADD ST(0),ST(1)
	instrs := x86ScanBlock(mem, startPC)
	compiled, err := x86WasmCompileBlockModule(instrs, startPC, mem)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	script := `
const bytes = Buffer.from(process.argv[1], "base64");
const mem = new WebAssembly.Memory({initial:1});
new WebAssembly.Instance(new WebAssembly.Module(bytes), {env:{mem}});
console.log(JSON.stringify({ok:1}));
`
	got := runNodeJSON(t, script, base64.StdEncoding.EncodeToString(compiled.module))
	if got["ok"] != 1 {
		t.Fatalf("node result = %#v, want ok=1", got)
	}
}

func TestX86WasmNode_DriverChainsAcrossCachedBlocks(t *testing.T) {
	const (
		ctxAddr    = uint32(0x80)
		cacheBase  = uint32(0x200)
		cacheMask  = uint32(0x0F)
		startPC    = uint32(0x100)
		secondPC   = uint32(0x108)
		finalPC    = uint32(0x118)
		entry0Slot = uint32(1)
		entry1Slot = uint32(2)
	)

	m := newWasmModuleBuilder()
	m.defineMemory(1)
	m.defineTable(4)
	typ := m.addType([]byte{wasmTypeI32}, nil)

	b0 := &wasmBody{}
	x86WasmEmitRetPCAndCount(b0, secondPC, 1, 3, 5)
	b0.end()
	f0 := m.addFunc(typ, nil, b0.code)

	b1 := &wasmBody{}
	x86WasmEmitRetPCAndCount(b1, finalPC, 1, 7, 11)
	b1.end()
	f1 := m.addFunc(typ, nil, b1.code)

	m.elemSeed(0, []uint32{f0, f1})
	m.exportMemory("mem")
	m.exportTable("tab")
	envMod := m.build()
	driverMod := x86WasmBuildDriverModule(cacheBase, cacheMask)

	script := `
const envBytes = Buffer.from(process.argv[1], "base64");
const driverBytes = Buffer.from(process.argv[2], "base64");
const envInst = new WebAssembly.Instance(new WebAssembly.Module(envBytes), {});
const mem = envInst.exports.mem;
const tab = envInst.exports.tab;
const driver = new WebAssembly.Instance(new WebAssembly.Module(driverBytes), {env:{mem, tab}});
const dv = new DataView(mem.buffer);
const ctxAddr = ` + "0x80" + `;
const cacheBase = ` + "0x200" + `;
const cacheMask = ` + "0x0f" + `;
function put32(addr, v) { dv.setUint32(addr, v >>> 0, true); }
function writeCache(pc, slot) {
  const entry = cacheBase + ((pc & cacheMask) << 3);
  put32(entry + 0, pc);
  put32(entry + 4, slot);
}
put32(ctxAddr + ` + "56" + `, ` + "0x100" + `);
put32(ctxAddr + ` + "96" + `, 4);
writeCache(` + "0x100" + `, ` + "1" + `);
writeCache(` + "0x108" + `, ` + "2" + `);
driver.exports.drive(ctxAddr);
console.log(JSON.stringify({
  retPC: dv.getUint32(ctxAddr + ` + "56" + `, true),
  retCount: dv.getUint32(ctxAddr + ` + "60" + `, true),
  chainCount: dv.getUint32(ctxAddr + ` + "100" + `, true),
  chainCycles: dv.getUint32(ctxAddr + ` + "200" + `, true),
  chainTicks: dv.getUint32(ctxAddr + ` + "204" + `, true)
}));
`
	got := runNodeJSON(t, script,
		base64.StdEncoding.EncodeToString(envMod),
		base64.StdEncoding.EncodeToString(driverMod),
	)
	if got["retPC"] != finalPC {
		t.Fatalf("RetPC=%#x want %#x", got["retPC"], finalPC)
	}
	if got["retCount"] != 0 {
		t.Fatalf("RetCount=%d want 0", got["retCount"])
	}
	if got["chainCount"] != 2 {
		t.Fatalf("ChainCount=%d want 2", got["chainCount"])
	}
	if got["chainCycles"] != 10 {
		t.Fatalf("ChainCycles=%d want 10", got["chainCycles"])
	}
	if got["chainTicks"] != 16 {
		t.Fatalf("ChainTicks=%d want 16", got["chainTicks"])
	}
}

func TestX86WasmNode_ExecutesForwardRegionModule(t *testing.T) {
	mem := make([]byte, 0x130)
	copy(mem[0x100:], []byte{
		0xB8, 0x78, 0x56, 0x34, 0x12, // MOV EAX,0x12345678
		0xEB, 0x09, // -> 0x110
	})
	copy(mem[0x110:], []byte{
		0x8B, 0xC8, // MOV ECX,EAX
		0xEB, 0x0C, // -> 0x120
	})
	copy(mem[0x120:], []byte{
		0xB0, 0xAA, // MOV AL,0xAA
		0xEB, 0x0C, // -> 0x130
	})
	region := x86FormRegion(0x100, NewCodeCache(), mem)
	if region == nil || len(region.blocks) != 3 {
		t.Fatalf("region=%#v", region)
	}
	compiled, err := x86WasmCompileRegionModule(region, mem)
	if err != nil {
		t.Fatalf("compile region: %v", err)
	}
	script := `
const bytes = Buffer.from(process.argv[1], "base64");
const mem = new WebAssembly.Memory({initial:1});
const inst = new WebAssembly.Instance(new WebAssembly.Module(bytes), {env:{mem}});
const dv = new DataView(mem.buffer);
const ctxAddr = 0x80;
const regsAddr = 0xC0;
dv.setUint32(ctxAddr + ` + "0" + `, regsAddr, true);
inst.exports.block(ctxAddr);
console.log(JSON.stringify({
  retPC: dv.getUint32(ctxAddr + ` + "56" + `, true),
  retCount: dv.getUint32(ctxAddr + ` + "60" + `, true),
  eax: dv.getUint32(regsAddr + 0, true),
  ecx: dv.getUint32(regsAddr + 4, true)
}));
`
	got := runNodeJSON(t, script, base64.StdEncoding.EncodeToString(compiled.module))
	if got["retPC"] != 0x130 {
		t.Fatalf("RetPC=%#x want %#x", got["retPC"], uint32(0x130))
	}
	if got["retCount"] != 6 {
		t.Fatalf("RetCount=%d want 6", got["retCount"])
	}
	if got["eax"] != 0x123456AA {
		t.Fatalf("EAX=%#x want %#x", got["eax"], uint32(0x123456AA))
	}
	if got["ecx"] != 0x12345678 {
		t.Fatalf("ECX=%#x want %#x", got["ecx"], uint32(0x12345678))
	}
}

func TestX86WasmNode_ExecutesBackEdgeLoopRegion(t *testing.T) {
	mem := make([]byte, 0x140)
	copy(mem[0x100:], []byte{
		0xB8, 0x01, 0x00, 0x00, 0x00, // MOV EAX,1
		0xEB, 0x09, // -> 0x110
	})
	copy(mem[0x110:], []byte{
		0x40,       // INC EAX
		0xEB, 0x0D, // -> 0x120
	})
	copy(mem[0x120:], []byte{
		0x49,       // DEC ECX
		0xEB, 0xED, // -> 0x110
	})
	region := x86FormRegion(0x100, NewCodeCache(), mem)
	if region == nil || len(region.blocks) != 3 || len(region.backEdges) != 1 || region.backEdges[2] != 1 {
		t.Fatalf("region=%#v", region)
	}
	compiled, err := x86WasmCompileRegionModule(region, mem)
	if err != nil {
		t.Fatalf("compile region: %v", err)
	}
	script := `
const bytes = Buffer.from(process.argv[1], "base64");
const mem = new WebAssembly.Memory({initial:1});
const inst = new WebAssembly.Instance(new WebAssembly.Module(bytes), {env:{mem}});
const dv = new DataView(mem.buffer);
const ctxAddr = 0x80;
const regsAddr = 0xC0;
const flagsAddr = 0x120;
dv.setUint32(ctxAddr + 0, regsAddr, true);
dv.setUint32(ctxAddr + 24, flagsAddr, true);
dv.setUint32(regsAddr + 4, 7, true);
dv.setUint32(ctxAddr + 96, 3, true);
inst.exports.block(ctxAddr);
console.log(JSON.stringify({
  retPC: dv.getUint32(ctxAddr + 56, true),
  retCount: dv.getUint32(ctxAddr + 60, true),
  chainBudget: dv.getUint32(ctxAddr + 96, true),
  chainCycles: dv.getUint32(ctxAddr + 200, true),
  chainTicks: dv.getUint32(ctxAddr + 204, true),
  eax: dv.getUint32(regsAddr + 0, true),
  ecx: dv.getUint32(regsAddr + 4, true)
}));
`
	got := runNodeJSON(t, script, base64.StdEncoding.EncodeToString(compiled.module))
	if got["eax"] != 4 {
		t.Fatalf("EAX=%#x want %#x", got["eax"], uint32(4))
	}
	if got["ecx"] != 4 {
		t.Fatalf("ECX=%#x want %#x", got["ecx"], uint32(4))
	}
	if got["retPC"] != 0x110 {
		t.Fatalf("RetPC=%#x want %#x", got["retPC"], uint32(0x110))
	}
	if got["retCount"] != 14 {
		t.Fatalf("RetCount=%d want 14", got["retCount"])
	}
	if got["chainBudget"] != 0 {
		t.Fatalf("ChainBudget=%d want 0", got["chainBudget"])
	}
}

func TestX86WasmNode_ConditionalRegionModuleBranchesInternally(t *testing.T) {
	mem := make([]byte, 0x140)
	copy(mem[0x100:], []byte{
		0x85, 0xC0, // TEST EAX,EAX
		0x0F, 0x84, 0x08, 0x00, 0x00, 0x00, // JZ 0x110
	})
	copy(mem[0x108:], []byte{
		0xBB, 0x01, 0x00, 0x00, 0x00, // MOV EBX,1
		0xEB, 0x0C, // -> 0x11b
	})
	copy(mem[0x110:], []byte{
		0xBB, 0x02, 0x00, 0x00, 0x00, // MOV EBX,2
		0xEB, 0x04, // -> 0x11b
	})
	region := x86WasmFormConditionalRegion(0x100, mem)
	if region == nil {
		t.Fatal("conditional region not formed")
	}
	compiled, err := x86WasmCompileConditionalRegionModule(region, mem)
	if err != nil {
		t.Fatalf("compile conditional region: %v", err)
	}
	script := `
const bytes = Buffer.from(process.argv[1], "base64");
const eax = Number(process.argv[2]) >>> 0;
const mem = new WebAssembly.Memory({initial:1});
const inst = new WebAssembly.Instance(new WebAssembly.Module(bytes), {env:{mem}});
const dv = new DataView(mem.buffer);
const ctxAddr = 0x80;
const regsAddr = 0xC0;
const flagsAddr = 0x120;
dv.setUint32(ctxAddr + ` + "0" + `, regsAddr, true);
dv.setUint32(ctxAddr + ` + "24" + `, flagsAddr, true);
dv.setUint32(regsAddr + 0, eax, true);
inst.exports.block(ctxAddr);
console.log(JSON.stringify({
  retPC: dv.getUint32(ctxAddr + ` + "56" + `, true),
  retCount: dv.getUint32(ctxAddr + ` + "60" + `, true),
  ebx: dv.getUint32(regsAddr + 12, true)
}));
`
	fallthroughRes := runNodeJSON(t, script, base64.StdEncoding.EncodeToString(compiled.module), "1")
	if fallthroughRes["retPC"] != 0x11b || fallthroughRes["retCount"] != 4 || fallthroughRes["ebx"] != 1 {
		t.Fatalf("fallthrough = %#v", fallthroughRes)
	}
	taken := runNodeJSON(t, script, base64.StdEncoding.EncodeToString(compiled.module), "0")
	if taken["retPC"] != 0x11b || taken["retCount"] != 4 || taken["ebx"] != 2 {
		t.Fatalf("taken = %#v", taken)
	}
}

func TestX86WasmNode_DirectX87SIMDBlockMatchesInterpreter(t *testing.T) {
	const startPC = uint32(0x1000)
	mem := make([]byte, 0x2000)
	code := []byte{0xD8, 0xC1} // FADD ST(0),ST(1)
	copy(mem[startPC:], code)
	instrs := x86ScanBlock(mem, startPC)
	compiled, err := x86WasmCompileBlockModule(instrs, startPC, mem)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	bus := NewMachineBus()
	adapter := NewX86BusAdapter(bus)
	cpu := NewCPU_X86(adapter)
	cpu.memory = adapter.GetMemory()
	copy(cpu.memory[startPC:], code)
	cpu.CS = 0x3456
	cpu.FPU.Reset()
	cpu.FPU.regs[0] = 1.5
	cpu.FPU.regs[1] = 2.25
	cpu.FPU.setTag(0, x87TagValid)
	cpu.FPU.setTag(1, x87TagValid)

	interp := runX86InterpreterOneInstr(t, startPC, func(cpu *CPU_X86) {
		cpu.CS = 0x3456
		cpu.FPU.Reset()
		cpu.FPU.regs[0] = 1.5
		cpu.FPU.regs[1] = 2.25
		cpu.FPU.setTag(0, x87TagValid)
		cpu.FPU.setTag(1, x87TagValid)
	}, code...)

	const (
		ctxAddr   = uint32(0x80)
		fpuAddr   = uint32(0x300)
		flagsAddr = uint32(0x280)
		segsAddr  = uint32(0x2C0)
	)
	image := make([]byte, 0x1000)
	binary.LittleEndian.PutUint32(image[int(ctxAddr+x86CtxOffFlagsPtr):], flagsAddr)
	binary.LittleEndian.PutUint32(image[int(ctxAddr+x86CtxOffSegRegsPtr):], segsAddr)
	binary.LittleEndian.PutUint32(image[int(ctxAddr+x86CtxOffFPUPtr):], fpuAddr)
	binary.LittleEndian.PutUint16(image[int(segsAddr+x86SegCS*2):], cpu.CS)
	binary.LittleEndian.PutUint32(image[int(ctxAddr+x86CtxOffMemSize):], 0x2000)
	copy(image[int(fpuAddr):], x86MarshalNodeFPU(cpu.FPU))

	script := `
const bytes = Buffer.from(process.argv[1], "base64");
const image = Buffer.from(process.argv[2], "base64");
const mem = new WebAssembly.Memory({initial:1});
new Uint8Array(mem.buffer).set(image, 0);
const inst = new WebAssembly.Instance(new WebAssembly.Module(bytes), {env:{mem}});
inst.exports.block(` + "0x80" + `);
const u8 = new Uint8Array(mem.buffer, ` + "0x300" + `, ` + "108" + `);
console.log(JSON.stringify({fpu: Buffer.from(u8).toString("base64")}));
`
	node := requireNode(t)
	out, err := exec.Command(node, "-e", script,
		base64.StdEncoding.EncodeToString(compiled.module),
		base64.StdEncoding.EncodeToString(image),
	).CombinedOutput()
	if err != nil {
		t.Fatalf("node failed: %v\n%s", err, out)
	}
	var got struct {
		FPU string `json:"fpu"`
	}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("decode node JSON: %v\n%s", err, out)
	}
	fpuBytes, err := base64.StdEncoding.DecodeString(got.FPU)
	if err != nil {
		t.Fatalf("decode node fpu: %v", err)
	}
	gotFPU := x86UnmarshalNodeFPU(t, fpuBytes)
	assertFPUStateEqual(t, gotFPU, *interp.FPU)
}

func x86MarshalNodeFPU(f *FPU_X87) []byte {
	buf := make([]byte, x86FPUSize)
	for i, v := range f.regs {
		binary.LittleEndian.PutUint64(buf[x86FPUOffRegs+i*8:], math.Float64bits(v))
	}
	buf[x86FPUOffFCW] = byte(f.FCW)
	buf[x86FPUOffFCW+1] = byte(f.FCW >> 8)
	buf[x86FPUOffFSW] = byte(f.FSW)
	buf[x86FPUOffFSW+1] = byte(f.FSW >> 8)
	binary.LittleEndian.PutUint16(buf[x86FPUOffFTW:], f.FTW)
	binary.LittleEndian.PutUint32(buf[x86FPUOffFIP:], f.FIP)
	binary.LittleEndian.PutUint16(buf[x86FPUOffFCS:], f.FCS)
	binary.LittleEndian.PutUint32(buf[x86FPUOffFDP:], f.FDP)
	binary.LittleEndian.PutUint16(buf[x86FPUOffFDS:], f.FDS)
	binary.LittleEndian.PutUint16(buf[x86FPUOffFOP:], f.FOP)
	return buf
}

func x86UnmarshalNodeFPU(t testing.TB, buf []byte) FPU_X87 {
	t.Helper()
	if len(buf) < x86FPUSize {
		t.Fatalf("fpu buffer too small: %d", len(buf))
	}
	var f FPU_X87
	for i := range f.regs {
		f.regs[i] = math.Float64frombits(binary.LittleEndian.Uint64(buf[x86FPUOffRegs+i*8:]))
	}
	f.FCW = binary.LittleEndian.Uint16(buf[x86FPUOffFCW:])
	f.FSW = binary.LittleEndian.Uint16(buf[x86FPUOffFSW:])
	f.FTW = binary.LittleEndian.Uint16(buf[x86FPUOffFTW:])
	f.FIP = binary.LittleEndian.Uint32(buf[x86FPUOffFIP:])
	f.FCS = binary.LittleEndian.Uint16(buf[x86FPUOffFCS:])
	f.FDP = binary.LittleEndian.Uint32(buf[x86FPUOffFDP:])
	f.FDS = binary.LittleEndian.Uint16(buf[x86FPUOffFDS:])
	f.FOP = binary.LittleEndian.Uint16(buf[x86FPUOffFOP:])
	return f
}
