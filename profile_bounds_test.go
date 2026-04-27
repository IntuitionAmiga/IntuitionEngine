package main

import (
	"testing"
)

// fakeProfileBus is a minimal stand-in for *MachineBus that exposes a chosen
// effective RAM cap to ProfileBounds constructors. Lets profile-bound tests
// run without a full bus + sizing wiring.
type fakeProfileBus struct {
	activeVisible uint64
}

func (f fakeProfileBus) ProfileMemoryCap() uint64 { return f.activeVisible }

func TestEmuTOSProfileBounds_HasExplicitContract(t *testing.T) {
	bus := fakeProfileBus{activeVisible: 32 * 1024 * 1024}
	pb := EmuTOSProfileBounds(bus)
	if pb.Err != nil {
		t.Fatalf("unexpected err: %v", pb.Err)
	}
	if pb.Name != "EmuTOS" {
		t.Fatalf("name=%q want EmuTOS", pb.Name)
	}
	if pb.TopOfRAM != EmuTOS_PROFILE_TOP {
		t.Fatalf("TopOfRAM=0x%X want 0x%X", pb.TopOfRAM, EmuTOS_PROFILE_TOP)
	}
	if pb.ROMBase != emutosBaseStd {
		t.Fatalf("ROMBase=0x%X want 0x%X", pb.ROMBase, emutosBaseStd)
	}
	if pb.LowVecBase != 0x1000 {
		t.Fatalf("LowVecBase=0x%X want 0x1000", pb.LowVecBase)
	}
}

func TestEmuTOSProfileBounds_RejectsBelowMinimum(t *testing.T) {
	bus := fakeProfileBus{activeVisible: 16 * 1024 * 1024}
	pb := EmuTOSProfileBounds(bus)
	if pb.Err == nil {
		t.Fatalf("expected err on undersized RAM")
	}
}

func TestEmuTOSProfileBounds_DoesNotInheritFullM68KRange(t *testing.T) {
	// M68K has a 4 GiB architectural visible range. EmuTOS profile must NOT
	// expose all 4 GiB just because the CPU can address it; PLAN_MAX_RAM.md
	// requires explicit profile bounds independent of the M68K ceiling.
	bus := fakeProfileBus{activeVisible: 4 * 1024 * 1024 * 1024}
	pb := EmuTOSProfileBounds(bus)
	if pb.Err != nil {
		t.Fatalf("unexpected err: %v", pb.Err)
	}
	if pb.TopOfRAM != EmuTOS_PROFILE_TOP {
		t.Fatalf("TopOfRAM=0x%X want EmuTOS_PROFILE_TOP=0x%X (must not inherit full 4 GiB)",
			pb.TopOfRAM, EmuTOS_PROFILE_TOP)
	}
}

func TestEmuTOSProfileBounds_ROMRangeFitsTopOfRAM(t *testing.T) {
	bus := fakeProfileBus{activeVisible: 32 * 1024 * 1024}
	pb := EmuTOSProfileBounds(bus)
	if pb.ROMEnd > pb.TopOfRAM {
		t.Fatalf("ROM range 0x%X..0x%X exceeds TopOfRAM 0x%X",
			pb.ROMBase, pb.ROMEnd, pb.TopOfRAM)
	}
	if pb.ROMBase >= pb.ROMEnd {
		t.Fatalf("ROMBase 0x%X >= ROMEnd 0x%X", pb.ROMBase, pb.ROMEnd)
	}
}

func TestAROSProfileBounds_HasExplicitContract(t *testing.T) {
	bus := fakeProfileBus{activeVisible: 32 * 1024 * 1024}
	pb := AROSProfileBounds(bus)
	if pb.Err != nil {
		t.Fatalf("unexpected err: %v", pb.Err)
	}
	if pb.Name != "AROS" {
		t.Fatalf("name=%q want AROS", pb.Name)
	}
	if pb.TopOfRAM != AROS_PROFILE_TOP {
		t.Fatalf("TopOfRAM=0x%X want 0x%X", pb.TopOfRAM, AROS_PROFILE_TOP)
	}
	if pb.ROMBase != arosROMBase {
		t.Fatalf("ROMBase=0x%X want 0x%X", pb.ROMBase, arosROMBase)
	}
}

func TestAROSProfileBounds_DirectVRAMGuaranteed(t *testing.T) {
	// PLAN_MAX_RAM.md requires AROS direct VRAM at 0x1E00000..0x2000000 to
	// remain guaranteed by the M68K profile or be moved deliberately. The
	// profile contract anchors this so any future move trips the test.
	bus := fakeProfileBus{activeVisible: 32 * 1024 * 1024}
	pb := AROSProfileBounds(bus)
	if pb.VRAMBase != 0x1E00000 {
		t.Fatalf("VRAMBase=0x%X want 0x1E00000 (AROS direct VRAM contract)", pb.VRAMBase)
	}
	if pb.VRAMEnd != 0x2000000 {
		t.Fatalf("VRAMEnd=0x%X want 0x2000000", pb.VRAMEnd)
	}
	if pb.VRAMEnd > pb.TopOfRAM {
		t.Fatalf("VRAM 0x%X..0x%X exceeds TopOfRAM 0x%X", pb.VRAMBase, pb.VRAMEnd, pb.TopOfRAM)
	}
}

func TestAROSProfileBounds_DoesNotInheritFullM68KRange(t *testing.T) {
	bus := fakeProfileBus{activeVisible: 4 * 1024 * 1024 * 1024}
	pb := AROSProfileBounds(bus)
	if pb.Err != nil {
		t.Fatalf("unexpected err: %v", pb.Err)
	}
	if pb.TopOfRAM != AROS_PROFILE_TOP {
		t.Fatalf("TopOfRAM=0x%X want AROS_PROFILE_TOP=0x%X (must not inherit full 4 GiB)",
			pb.TopOfRAM, AROS_PROFILE_TOP)
	}
}

func TestAROSProfileBounds_RejectsBelowMinimum(t *testing.T) {
	bus := fakeProfileBus{activeVisible: 16 * 1024 * 1024}
	pb := AROSProfileBounds(bus)
	if pb.Err == nil {
		t.Fatalf("expected err on undersized RAM")
	}
}

func TestProfileTops_PageAligned(t *testing.T) {
	if EmuTOS_PROFILE_TOP&uint32(MMU_PAGE_SIZE-1) != 0 {
		t.Fatalf("EmuTOS_PROFILE_TOP not page aligned: 0x%X", EmuTOS_PROFILE_TOP)
	}
	if AROS_PROFILE_TOP&uint32(MMU_PAGE_SIZE-1) != 0 {
		t.Fatalf("AROS_PROFILE_TOP not page aligned: 0x%X", AROS_PROFILE_TOP)
	}
}

func TestEhBASICProfileBounds_FollowsActiveVisibleRAM(t *testing.T) {
	bus := fakeProfileBus{activeVisible: 64 * 1024 * 1024}
	pb := EhBASICProfileBounds(bus)
	if pb.Err != nil {
		t.Fatalf("unexpected err: %v", pb.Err)
	}
	if uint64(pb.TopOfRAM) != 64*1024*1024 {
		t.Fatalf("TopOfRAM=0x%X want 0x%X", pb.TopOfRAM, 64*1024*1024)
	}
	if pb.Name != "EhBASIC" {
		t.Fatalf("name=%q want EhBASIC", pb.Name)
	}
}

func TestEhBASICProfileBounds_RejectsBelowLayoutMinimum(t *testing.T) {
	// EhBASIC's source layout requires at least up to STACK_TOP (0x9F000)
	// of usable RAM. Anything smaller cannot host the runtime.
	bus := fakeProfileBus{activeVisible: 0x80000}
	pb := EhBASICProfileBounds(bus)
	if pb.Err == nil {
		t.Fatalf("expected err on undersized RAM")
	}
}

func TestEhBASICLayoutFitsDefaultProfile(t *testing.T) {
	// The EhBASIC source layout (BASIC_LINE_BUF..STACK_TOP) must fit inside
	// any active visible RAM at or above MIN_GUEST_RAM. If a future
	// re-layout pushes STACK_TOP above MIN_GUEST_RAM the test trips and
	// the source-owned profile contract is wrong.
	bus := fakeProfileBus{activeVisible: MIN_GUEST_RAM}
	pb := EhBASICProfileBounds(bus)
	if pb.Err != nil {
		t.Fatalf("EhBASIC profile rejected MIN_GUEST_RAM bus: %v", pb.Err)
	}
	if !EhBASICLayoutFitsTopOfRAM(pb.TopOfRAM) {
		t.Fatalf("EhBASIC layout does not fit profile TopOfRAM=0x%X", pb.TopOfRAM)
	}
}

func TestEnforceEhBASICProfile(t *testing.T) {
	if err := EnforceEhBASICProfile(fakeProfileBus{activeVisible: MIN_GUEST_RAM}); err != nil {
		t.Fatalf("EnforceEhBASICProfile returned err on MIN_GUEST_RAM bus: %v", err)
	}
	if err := EnforceEhBASICProfile(fakeProfileBus{activeVisible: 0x80000}); err == nil {
		t.Fatalf("EnforceEhBASICProfile accepted under-sized bus")
	}
}

func TestM68KCPU_ProfileTopOfRAM_LiteralConstruction(t *testing.T) {
	// Direct M68KCPU literal construction (e.g. the Harte test harness in
	// cpu_m68k_harte_test.go::getHarteTestCPU) bypasses NewM68KCPU and
	// leaves profileTopOfRAM at its zero value. The accessor must fall back
	// to len(memory) so out-of-profile fetch/branch checks still trap at
	// the bus ceiling instead of underflowing to ~4 GiB.
	bus := NewMachineBus()
	mem := bus.GetMemory()
	cpu := &M68KCPU{
		bus:     bus,
		memory:  mem,
		memBase: nil,
	}
	got := cpu.ProfileTopOfRAM()
	want := uint32(len(mem))
	if got != want {
		t.Fatalf("zero-init profileTopOfRAM accessor returned 0x%X, want len(memory)=0x%X (would underflow PC bounds checks otherwise)",
			got, want)
	}
	// And after explicit set, accessor must report the new value.
	cpu.SetProfileTopOfRAM(EmuTOS_PROFILE_TOP)
	if got := cpu.ProfileTopOfRAM(); got != EmuTOS_PROFILE_TOP {
		t.Fatalf("after SetProfileTopOfRAM: got 0x%X, want 0x%X", got, EmuTOS_PROFILE_TOP)
	}
}

func TestEhBASICProfileBounds_CapsAtUint32(t *testing.T) {
	// IE64 may report active visible RAM above 4 GiB. ProfileBounds.TopOfRAM
	// is uint32 (low-memory layout); above-4-GiB visibility is queried via
	// sysinfo/CR_RAM_SIZE_BYTES. The profile must cap rather than truncate.
	bus := fakeProfileBus{activeVisible: 8 * 1024 * 1024 * 1024}
	pb := EhBASICProfileBounds(bus)
	if pb.Err != nil {
		t.Fatalf("unexpected err: %v", pb.Err)
	}
	if uint64(pb.TopOfRAM) > 0x100000000-uint64(MMU_PAGE_SIZE) {
		t.Fatalf("TopOfRAM=0x%X must be capped at 4 GiB - 1 page", pb.TopOfRAM)
	}
}
