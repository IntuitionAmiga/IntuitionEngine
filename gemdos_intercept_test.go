//go:build headless

package main

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// newTestGemdos creates a GemdosInterceptor with a temp directory as host root.
func newTestGemdos(t *testing.T) (*GemdosInterceptor, *M68KCPU, *MachineBus) {
	t.Helper()
	dir := t.TempDir()
	bus := NewMachineBus()
	cpu := NewM68KCPU(bus)
	g, err := NewGemdosInterceptor(cpu, bus, dir, 20) // U: = drive 20
	if err != nil {
		t.Fatalf("NewGemdosInterceptor: %v", err)
	}
	cpu.gemdosHandler = g
	return g, cpu, bus
}

// pushTrapFrame pushes a GEMDOS TRAP #1 stack frame with the function number.
func pushTrapFrame(cpu *M68KCPU, funcNum uint16, args ...uint32) uint32 {
	// Build frame: function number (word) followed by args
	// Stack grows down; we'll place SP high enough
	sp := uint32(0x8000)
	cpu.Write16(sp, funcNum)
	offset := uint32(2)
	for _, arg := range args {
		cpu.Write32(sp+offset, arg)
		offset += 4
	}
	cpu.AddrRegs[7] = sp
	return sp
}

// pushTrapFrameWords pushes a TRAP frame with arbitrary word/long args.
// layout is pairs of (size, value) where size is 2 for word, 4 for long.
func pushTrapFrameRaw(cpu *M68KCPU, funcNum uint16, params []struct {
	size  int
	value uint32
}) uint32 {
	sp := uint32(0x8000)
	cpu.Write16(sp, funcNum)
	offset := uint32(2)
	for _, p := range params {
		if p.size == 2 {
			cpu.Write16(sp+offset, uint16(p.value))
			offset += 2
		} else {
			cpu.Write32(sp+offset, p.value)
			offset += 4
		}
	}
	cpu.AddrRegs[7] = sp
	return sp
}

func writeGuestStringToBus(bus *MachineBus, addr uint32, s string) {
	for i := 0; i < len(s); i++ {
		bus.Write8(addr+uint32(i), s[i])
	}
	bus.Write8(addr+uint32(len(s)), 0)
}

// --- Path resolution tests ---

func TestGemdos_PathResolution_DriveLetter(t *testing.T) {
	g, _, _ := newTestGemdos(t)
	defer g.Close()

	// U:\FILE.TXT should resolve to hostRoot/FILE.TXT
	path, ok := g.resolvePathForOurDrive("U:\\FILE.TXT")
	if !ok {
		t.Fatal("expected path to target our drive")
	}
	if !strings.HasSuffix(path, "FILE.TXT") {
		t.Fatalf("expected path ending in FILE.TXT, got %s", path)
	}

	// C:\FILE.TXT should not target our drive
	_, ok = g.resolvePathForOurDrive("C:\\FILE.TXT")
	if ok {
		t.Fatal("expected C: path to NOT target our drive")
	}
}

func TestGemdos_PathResolution_DefaultDrive(t *testing.T) {
	g, _, _ := newTestGemdos(t)
	defer g.Close()

	// Default drive is 0 (A:), path without drive letter should not target us
	_, ok := g.resolvePathForOurDrive("FILE.TXT")
	if ok {
		t.Fatal("expected path without drive letter to NOT target our drive when default != U:")
	}

	// Set default drive to U:
	g.defaultDrive = 20
	path, ok := g.resolvePathForOurDrive("FILE.TXT")
	if !ok {
		t.Fatal("expected path to target our drive when default == U:")
	}
	if !strings.HasSuffix(path, "FILE.TXT") {
		t.Fatalf("expected path ending in FILE.TXT, got %s", path)
	}
}

func TestGemdos_PathResolution_BackslashConversion(t *testing.T) {
	g, _, _ := newTestGemdos(t)
	defer g.Close()

	// Create subdirectory
	os.MkdirAll(filepath.Join(g.hostRoot, "SUBDIR"), 0o755)

	path, ok := g.resolvePathForOurDrive("U:\\SUBDIR\\FILE.TXT")
	if !ok {
		t.Fatal("expected path to resolve")
	}
	// Should not contain backslashes in the resolved path
	if strings.Contains(path, "\\") {
		t.Fatalf("resolved path should not contain backslashes: %s", path)
	}
}

func TestGemdos_PathResolution_DotDotRejection(t *testing.T) {
	g, _, _ := newTestGemdos(t)
	defer g.Close()

	_, ok := g.resolvePathForOurDrive("U:\\..\\etc\\passwd")
	if ok {
		t.Fatal("expected .. path to be rejected")
	}
}

func TestGemdos_PathResolution_CaseInsensitive(t *testing.T) {
	g, _, _ := newTestGemdos(t)
	defer g.Close()

	// Create a file with mixed case
	os.WriteFile(filepath.Join(g.hostRoot, "Hello.Txt"), []byte("hi"), 0o644)

	path, ok := g.resolvePathForOurDrive("U:\\HELLO.TXT")
	if !ok {
		t.Fatal("expected path to resolve")
	}
	// Should resolve to actual file name
	if !strings.HasSuffix(path, "Hello.Txt") {
		t.Fatalf("expected case-insensitive match to Hello.Txt, got %s", path)
	}
}

// --- Handle allocation tests ---

func TestGemdos_HandleAllocation(t *testing.T) {
	g, _, _ := newTestGemdos(t)
	defer g.Close()

	// Create a test file
	testFile := filepath.Join(g.hostRoot, "test.txt")
	os.WriteFile(testFile, []byte("data"), 0o644)

	f, err := os.Open(testFile)
	if err != nil {
		t.Fatal(err)
	}

	h := g.allocHandle(f)
	if h != GEMDOS_HANDLE_MIN {
		t.Fatalf("first handle should be %d, got %d", GEMDOS_HANDLE_MIN, h)
	}

	f2, err := os.Open(testFile)
	if err != nil {
		t.Fatal(err)
	}
	h2 := g.allocHandle(f2)
	if h2 != GEMDOS_HANDLE_MIN+1 {
		t.Fatalf("second handle should be %d, got %d", GEMDOS_HANDLE_MIN+1, h2)
	}
}

func TestGemdos_HandleExhaustion(t *testing.T) {
	g, _, _ := newTestGemdos(t)
	defer g.Close()

	// Force next handle near max
	g.nextHandle = GEMDOS_HANDLE_MAX

	testFile := filepath.Join(g.hostRoot, "test.txt")
	os.WriteFile(testFile, []byte("data"), 0o644)

	f, _ := os.Open(testFile)
	h := g.allocHandle(f)
	if h != GEMDOS_HANDLE_MAX {
		t.Fatalf("expected handle %d, got %d", GEMDOS_HANDLE_MAX, h)
	}

	f2, _ := os.Open(testFile)
	h2 := g.allocHandle(f2)
	if h2 != -1 {
		t.Fatalf("expected -1 (exhausted), got %d", h2)
	}
	f2.Close()
}

// --- Wildcard matching tests ---

func TestGemdos_WildcardMatch(t *testing.T) {
	tests := []struct {
		pattern string
		name    string
		want    bool
	}{
		{"*.*", "FILE.TXT", true},
		{"*.*", "README", true}, // GEMDOS: *.* matches all files including dotless
		{"*.TXT", "FILE.TXT", true},
		{"*.TXT", "FILE.DOC", false},
		{"FILE*.*", "FILE.TXT", true},
		{"FILE*.*", "FILEABC.TXT", true},
		{"FILE*.*", "OTHER.TXT", false},
		{"?ELLO.TXT", "HELLO.TXT", true},
		{"?ELLO.TXT", "JELLO.TXT", true},
		{"?ELLO.TXT", "ELLO.TXT", false},
		{"*", "ANYTHING", true},
		{"*", "FILE.TXT", true},
	}
	for _, tt := range tests {
		got := gemdosWildcardMatch(tt.pattern, tt.name)
		if got != tt.want {
			t.Errorf("wildcardMatch(%q, %q) = %v, want %v", tt.pattern, tt.name, got, tt.want)
		}
	}
}

// --- 8.3 name conversion tests ---

func TestGemdos_ToGemdos83(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"readme.txt", "README.TXT"},
		{"FILE.DOC", "FILE.DOC"},
		{"GoLand-2024.1.1", "GOLAND-2.1"},           // last dot split, name truncated to 8
		{"product-info.json", "PRODUCT-.JSO"},       // name truncated, ext truncated
		{"amiberry.tmp", "AMIBERRY.TMP"},            // already 8.3
		{".config", "CONFIG"},                       // leading dot stripped
		{".bash_history", "BASH_HIS"},               // leading dot stripped, truncated
		{"bin", "BIN"},                              // no extension
		{"LONGDIRNAME", "LONGDIRN"},                 // 11 chars → truncated to 8
		{"a.b.c.d", "ABC.D"},                        // multiple dots: last dot splits
		{"hello world.txt", "HELLOWOR.TXT"},         // spaces removed (invalid GEMDOS char)
		{"", ""},                                    // empty
		{"...", ""},                                 // all dots
		{"Calibre Library", "CALIBREL"},             // spaces removed, truncated
		{"2026-02-20 20-10-36.mp4", "2026-02-.MP4"}, // spaces removed
	}
	for _, tt := range tests {
		got := toGemdos83(tt.input)
		if got != tt.want {
			t.Errorf("toGemdos83(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestGemdos_MakeUnique83(t *testing.T) {
	used := make(map[string]bool)

	// First use should return base name unchanged
	name1 := makeUnique83("INTUITIO", used)
	if name1 != "INTUITIO" {
		t.Errorf("first = %q, want INTUITIO", name1)
	}
	used[name1] = true

	// Second use should return ~1 suffix
	name2 := makeUnique83("INTUITIO", used)
	if name2 != "INTUIT~1" {
		t.Errorf("second = %q, want INTUIT~1", name2)
	}
	used[name2] = true

	// Third use should return ~2
	name3 := makeUnique83("INTUITIO", used)
	if name3 != "INTUIT~2" {
		t.Errorf("third = %q, want INTUIT~2", name3)
	}
	used[name3] = true

	// With extension
	used2 := make(map[string]bool)
	e1 := makeUnique83("2026-01-.MP4", used2)
	if e1 != "2026-01-.MP4" {
		t.Errorf("ext first = %q, want 2026-01-.MP4", e1)
	}
	used2[e1] = true

	e2 := makeUnique83("2026-01-.MP4", used2)
	if e2 != "2026-0~1.MP4" {
		t.Errorf("ext second = %q, want 2026-0~1.MP4", e2)
	}
	used2[e2] = true

	// Many collisions (>9 should use ~10, ~11, etc.)
	used3 := make(map[string]bool)
	for i := 0; i < 12; i++ {
		n := makeUnique83("AMIBERRY", used3)
		used3[n] = true
		if i == 0 && n != "AMIBERRY" {
			t.Errorf("collision %d = %q, want AMIBERRY", i, n)
		}
		if i == 10 && n != "AMIBE~10" {
			t.Errorf("collision %d = %q, want AMIBE~10", i, n)
		}
	}
	// All 12 should be unique
	if len(used3) != 12 {
		t.Errorf("got %d unique names, want 12", len(used3))
	}
}

// --- DTA encoding tests ---

func TestGemdos_DateTimePacking(t *testing.T) {
	// 2024-03-15 14:30:22
	testTime := time.Date(2024, 3, 15, 14, 30, 22, 0, time.Local)
	dateWord := packGemdosDate(testTime)
	timeWord := packGemdosTime(testTime)
	tm := unpackGemdosDateTime(dateWord, timeWord)
	if tm.Year() != 2024 {
		t.Errorf("year=%d, want 2024", tm.Year())
	}
	if tm.Month() != 3 {
		t.Errorf("month=%d, want 3", tm.Month())
	}
	if tm.Day() != 15 {
		t.Errorf("day=%d, want 15", tm.Day())
	}
	if tm.Hour() != 14 {
		t.Errorf("hour=%d, want 14", tm.Hour())
	}
	if tm.Minute() != 30 {
		t.Errorf("minute=%d, want 30", tm.Minute())
	}
	// Seconds are rounded to even (GEMDOS stores sec/2)
	if tm.Second() != 22 {
		t.Errorf("second=%d, want 22", tm.Second())
	}
}

// --- Trap hook tests ---

func TestGemdos_TrapHook_Dsetdrv(t *testing.T) {
	g, cpu, _ := newTestGemdos(t)
	defer g.Close()

	// Dsetdrv(20) — set to U:
	pushTrapFrameRaw(cpu, GEMDOS_DSETDRV, []struct {
		size  int
		value uint32
	}{{2, 20}})

	handled := g.HandleTrap1()
	if !handled {
		t.Fatal("Dsetdrv(U:) should be handled")
	}
	// D0 should have drvbits with bit 20 set
	if cpu.DataRegs[0]&(1<<20) == 0 {
		t.Fatal("D0 should have bit 20 set for drive U:")
	}
	if g.defaultDrive != 20 {
		t.Fatalf("defaultDrive=%d, want 20", g.defaultDrive)
	}
}

func TestGemdos_TrapHook_Dsetdrv_OtherDrive(t *testing.T) {
	g, cpu, _ := newTestGemdos(t)
	defer g.Close()

	// Dsetdrv(2) — set to C:
	pushTrapFrameRaw(cpu, GEMDOS_DSETDRV, []struct {
		size  int
		value uint32
	}{{2, 2}})

	handled := g.HandleTrap1()
	if handled {
		t.Fatal("Dsetdrv(C:) should NOT be handled")
	}
	if g.defaultDrive != 2 {
		t.Fatalf("defaultDrive=%d, want 2", g.defaultDrive)
	}
}

func TestGemdos_TrapHook_Dgetdrv(t *testing.T) {
	g, cpu, _ := newTestGemdos(t)
	defer g.Close()

	// Set default to U: first
	g.defaultDrive = 20

	pushTrapFrame(cpu, GEMDOS_DGETDRV)
	handled := g.HandleTrap1()
	if !handled {
		t.Fatal("Dgetdrv when default=U: should be handled")
	}
	if cpu.DataRegs[0] != 20 {
		t.Fatalf("D0=%d, want 20", cpu.DataRegs[0])
	}
}

func TestGemdos_TrapHook_Dgetdrv_OtherDrive(t *testing.T) {
	g, cpu, _ := newTestGemdos(t)
	defer g.Close()

	// Default drive is A: (0), not U:
	pushTrapFrame(cpu, GEMDOS_DGETDRV)
	handled := g.HandleTrap1()
	if handled {
		t.Fatal("Dgetdrv when default=A: should NOT be handled")
	}
}

func TestGemdos_TrapHook_Dgetdrv_DivergentState(t *testing.T) {
	g, cpu, _ := newTestGemdos(t)
	defer g.Close()

	// Dsetdrv(U:) then Dsetdrv(C:)
	g.defaultDrive = 20
	g.defaultDrive = 2

	pushTrapFrame(cpu, GEMDOS_DGETDRV)
	handled := g.HandleTrap1()
	if handled {
		t.Fatal("Dgetdrv after Dsetdrv(C:) should NOT be handled (diverged state)")
	}
}

// --- File I/O tests ---

func TestGemdos_FileOpenReadClose(t *testing.T) {
	g, cpu, bus := newTestGemdos(t)
	defer g.Close()

	// Create a test file
	testData := []byte("Hello, EmuTOS!")
	os.WriteFile(filepath.Join(g.hostRoot, "TEST.TXT"), testData, 0o644)

	// Put filename in guest memory
	fnameAddr := uint32(0x2000)
	writeGuestStringToBus(bus, fnameAddr, "U:\\TEST.TXT")

	// Fopen
	pushTrapFrameRaw(cpu, GEMDOS_FOPEN, []struct {
		size  int
		value uint32
	}{{4, fnameAddr}, {2, GEMDOS_OPEN_READ}})
	if !g.HandleTrap1() {
		t.Fatal("Fopen should be handled")
	}
	handle := int16(cpu.DataRegs[0])
	if handle < GEMDOS_HANDLE_MIN {
		t.Fatalf("expected handle >= %d, got %d", GEMDOS_HANDLE_MIN, handle)
	}

	// Fread
	readBuf := uint32(0x3000)
	pushTrapFrameRaw(cpu, GEMDOS_FREAD, []struct {
		size  int
		value uint32
	}{{2, uint32(handle)}, {4, uint32(len(testData))}, {4, readBuf}})
	if !g.HandleTrap1() {
		t.Fatal("Fread should be handled")
	}
	nRead := cpu.DataRegs[0]
	if nRead != uint32(len(testData)) {
		t.Fatalf("read %d bytes, want %d", nRead, len(testData))
	}

	// Verify read data
	for i, b := range testData {
		got := bus.Read8(readBuf + uint32(i))
		if got != b {
			t.Fatalf("byte %d: got 0x%02X, want 0x%02X", i, got, b)
		}
	}

	// Fclose
	pushTrapFrameRaw(cpu, GEMDOS_FCLOSE, []struct {
		size  int
		value uint32
	}{{2, uint32(handle)}})
	if !g.HandleTrap1() {
		t.Fatal("Fclose should be handled")
	}
	if cpu.DataRegs[0] != 0 {
		t.Fatalf("Fclose D0=%d, want 0", cpu.DataRegs[0])
	}
}

func TestGemdos_FileCreateWriteRead(t *testing.T) {
	g, cpu, bus := newTestGemdos(t)
	defer g.Close()

	fnameAddr := uint32(0x2000)
	writeGuestStringToBus(bus, fnameAddr, "U:\\NEWFILE.TXT")

	// Fcreate
	pushTrapFrameRaw(cpu, GEMDOS_FCREATE, []struct {
		size  int
		value uint32
	}{{4, fnameAddr}, {2, 0}})
	if !g.HandleTrap1() {
		t.Fatal("Fcreate should be handled")
	}
	handle := int16(cpu.DataRegs[0])
	if handle < GEMDOS_HANDLE_MIN {
		t.Fatalf("expected handle >= %d, got %d", GEMDOS_HANDLE_MIN, handle)
	}

	// Fwrite
	writeData := []byte("Written data")
	writeBuf := uint32(0x3000)
	for i, b := range writeData {
		bus.Write8(writeBuf+uint32(i), b)
	}
	pushTrapFrameRaw(cpu, GEMDOS_FWRITE, []struct {
		size  int
		value uint32
	}{{2, uint32(handle)}, {4, uint32(len(writeData))}, {4, writeBuf}})
	if !g.HandleTrap1() {
		t.Fatal("Fwrite should be handled")
	}
	if cpu.DataRegs[0] != uint32(len(writeData)) {
		t.Fatalf("wrote %d bytes, want %d", cpu.DataRegs[0], len(writeData))
	}

	// Fclose
	pushTrapFrameRaw(cpu, GEMDOS_FCLOSE, []struct {
		size  int
		value uint32
	}{{2, uint32(handle)}})
	g.HandleTrap1()

	// Verify file on host
	data, err := os.ReadFile(filepath.Join(g.hostRoot, "NEWFILE.TXT"))
	if err != nil {
		t.Fatalf("host file not created: %v", err)
	}
	if string(data) != "Written data" {
		t.Fatalf("file content=%q, want %q", string(data), "Written data")
	}
}

func TestGemdos_Fseek(t *testing.T) {
	g, cpu, bus := newTestGemdos(t)
	defer g.Close()

	os.WriteFile(filepath.Join(g.hostRoot, "SEEK.TXT"), []byte("0123456789"), 0o644)

	fnameAddr := uint32(0x2000)
	writeGuestStringToBus(bus, fnameAddr, "U:\\SEEK.TXT")

	// Open
	pushTrapFrameRaw(cpu, GEMDOS_FOPEN, []struct {
		size  int
		value uint32
	}{{4, fnameAddr}, {2, GEMDOS_OPEN_READ}})
	g.HandleTrap1()
	handle := int16(cpu.DataRegs[0])

	// Seek to offset 5 from beginning
	pushTrapFrameRaw(cpu, GEMDOS_FSEEK, []struct {
		size  int
		value uint32
	}{{4, 5}, {2, uint32(handle)}, {2, GEMDOS_SEEK_SET}})
	if !g.HandleTrap1() {
		t.Fatal("Fseek should be handled")
	}
	if cpu.DataRegs[0] != 5 {
		t.Fatalf("seek position=%d, want 5", cpu.DataRegs[0])
	}
}

func TestGemdos_HandleForward_LowHandle(t *testing.T) {
	g, cpu, _ := newTestGemdos(t)
	defer g.Close()

	// Fclose with a low handle (< 1000) should forward to EmuTOS
	pushTrapFrameRaw(cpu, GEMDOS_FCLOSE, []struct {
		size  int
		value uint32
	}{{2, 5}})
	handled := g.HandleTrap1()
	if handled {
		t.Fatal("Fclose(5) should forward to EmuTOS")
	}
}

func TestGemdos_StaleHandle(t *testing.T) {
	g, cpu, _ := newTestGemdos(t)
	defer g.Close()

	// Try to read from a handle that doesn't exist but is in our range
	pushTrapFrameRaw(cpu, GEMDOS_FREAD, []struct {
		size  int
		value uint32
	}{{2, 1500}, {4, 10}, {4, 0x3000}})
	handled := g.HandleTrap1()
	if !handled {
		t.Fatal("Fread(1500) should be handled (stale handle)")
	}
	if int32(cpu.DataRegs[0]) != GEMDOS_EIHNDL {
		t.Fatalf("D0=%d, want %d (EIHNDL)", int32(cpu.DataRegs[0]), GEMDOS_EIHNDL)
	}
}

func TestGemdos_FreadRejectsCountBeyondProfileRAM(t *testing.T) {
	g, cpu, _ := newTestGemdos(t)
	defer g.Close()

	f, err := os.OpenFile(filepath.Join(g.hostRoot, "COUNT.BIN"), os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	handle := g.allocHandle(f)
	count := uint32(g.bus.ProfileMemoryCap() + 1)

	pushTrapFrameRaw(cpu, GEMDOS_FREAD, []struct {
		size  int
		value uint32
	}{{2, uint32(handle)}, {4, count}, {4, 0x3000}})
	if !g.HandleTrap1() {
		t.Fatal("Fread should be handled")
	}
	if int32(cpu.DataRegs[0]) != GEMDOS_EIMBA {
		t.Fatalf("D0=%d, want %d", int32(cpu.DataRegs[0]), GEMDOS_EIMBA)
	}
}

func TestGemdos_FreadRejectsBufAddrOutsideMappedRAM(t *testing.T) {
	g, cpu, _ := newTestGemdos(t)
	defer g.Close()

	f, err := os.OpenFile(filepath.Join(g.hostRoot, "ADDR.BIN"), os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	handle := g.allocHandle(f)

	pushTrapFrameRaw(cpu, GEMDOS_FREAD, []struct {
		size  int
		value uint32
	}{{2, uint32(handle)}, {4, 1}, {4, uint32(g.bus.ProfileMemoryCap())}})
	if !g.HandleTrap1() {
		t.Fatal("Fread should be handled")
	}
	if int32(cpu.DataRegs[0]) != GEMDOS_EIMBA {
		t.Fatalf("D0=%d, want %d", int32(cpu.DataRegs[0]), GEMDOS_EIMBA)
	}
}

func TestGemdos_FreadStreamsLargeValidRequest(t *testing.T) {
	g, cpu, bus := newTestGemdos(t)
	defer g.Close()

	data := bytes.Repeat([]byte{0x5A}, 8<<20)
	if err := os.WriteFile(filepath.Join(g.hostRoot, "LARGE.RD"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(filepath.Join(g.hostRoot, "LARGE.RD"))
	if err != nil {
		t.Fatal(err)
	}
	handle := g.allocHandle(f)
	bufAddr := uint32(0x3001)

	pushTrapFrameRaw(cpu, GEMDOS_FREAD, []struct {
		size  int
		value uint32
	}{{2, uint32(handle)}, {4, uint32(len(data))}, {4, bufAddr}})
	if !g.HandleTrap1() {
		t.Fatal("Fread should be handled")
	}
	if cpu.DataRegs[0] != uint32(len(data)) {
		t.Fatalf("read %d bytes, want %d", cpu.DataRegs[0], len(data))
	}
	if bus.Read8(bufAddr) != 0x5A || bus.Read8(bufAddr+uint32(len(data)-1)) != 0x5A {
		t.Fatal("large Fread did not populate expected buffer bytes")
	}
}

func TestGemdos_FwriteStreamsLargeValidRequest(t *testing.T) {
	g, cpu, bus := newTestGemdos(t)
	defer g.Close()

	f, err := os.Create(filepath.Join(g.hostRoot, "LARGE.WR"))
	if err != nil {
		t.Fatal(err)
	}
	handle := g.allocHandle(f)
	bufAddr := uint32(0x4001)
	count := uint32(8 << 20)
	for i := uint32(0); i < count; i++ {
		bus.Write8(bufAddr+i, uint8(i))
	}

	pushTrapFrameRaw(cpu, GEMDOS_FWRITE, []struct {
		size  int
		value uint32
	}{{2, uint32(handle)}, {4, count}, {4, bufAddr}})
	if !g.HandleTrap1() {
		t.Fatal("Fwrite should be handled")
	}
	if cpu.DataRegs[0] != count {
		t.Fatalf("wrote %d bytes, want %d", cpu.DataRegs[0], count)
	}
	info, err := f.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() != int64(count) {
		t.Fatalf("host size=%d, want %d", info.Size(), count)
	}
}

func TestGemdos_FreadHandlesUnalignedBufAddr(t *testing.T) {
	g, cpu, bus := newTestGemdos(t)
	defer g.Close()

	data := []byte{1, 2, 3, 4, 5}
	if err := os.WriteFile(filepath.Join(g.hostRoot, "ODD.RD"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(filepath.Join(g.hostRoot, "ODD.RD"))
	if err != nil {
		t.Fatal(err)
	}
	handle := g.allocHandle(f)
	bufAddr := uint32(0x3001)

	pushTrapFrameRaw(cpu, GEMDOS_FREAD, []struct {
		size  int
		value uint32
	}{{2, uint32(handle)}, {4, uint32(len(data))}, {4, bufAddr}})
	if !g.HandleTrap1() {
		t.Fatal("Fread should be handled")
	}
	for i, want := range data {
		if got := bus.Read8(bufAddr + uint32(i)); got != want {
			t.Fatalf("byte %d: got 0x%02X, want 0x%02X", i, got, want)
		}
	}
}

func TestGemdos_NewGemdosInterceptorRejectsInvalidDrive(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewM68KCPU(bus)
	if _, err := NewGemdosInterceptor(cpu, bus, t.TempDir(), 26); err == nil {
		t.Fatal("expected drive >= 26 to be rejected")
	}
}

func TestGemdos_FsetdtaBadPointerLeavesDtaAddrUntouched(t *testing.T) {
	g, cpu, _ := newTestGemdos(t)
	defer g.Close()
	g.dtaAddr = 0x1234

	pushTrapFrame(cpu, GEMDOS_FSETDTA, uint32(g.bus.ProfileMemoryCap()))
	if g.HandleTrap1() {
		t.Fatal("Fsetdta should remain snoop-only and forward to EmuTOS")
	}
	if g.dtaAddr != 0x1234 {
		t.Fatalf("dtaAddr=0x%X, want unchanged 0x1234", g.dtaAddr)
	}
}

func TestGemdos_FsetdtaValidPointerUpdatesDtaAddr(t *testing.T) {
	g, cpu, _ := newTestGemdos(t)
	defer g.Close()

	pushTrapFrame(cpu, GEMDOS_FSETDTA, 0x2345)
	if g.HandleTrap1() {
		t.Fatal("Fsetdta should remain snoop-only and forward to EmuTOS")
	}
	if g.dtaAddr != 0x2345 {
		t.Fatalf("dtaAddr=0x%X, want 0x2345", g.dtaAddr)
	}
}

func TestGemdos_FreadOverflowSafeBoundsCheck(t *testing.T) {
	g, cpu, _ := newTestGemdos(t)
	defer g.Close()

	f, err := os.OpenFile(filepath.Join(g.hostRoot, "WRAP.BIN"), os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	handle := g.allocHandle(f)
	cap := uint32(g.bus.ProfileMemoryCap())

	pushTrapFrameRaw(cpu, GEMDOS_FREAD, []struct {
		size  int
		value uint32
	}{{2, uint32(handle)}, {4, 20}, {4, cap - 10}})
	if !g.HandleTrap1() {
		t.Fatal("Fread should be handled")
	}
	if int32(cpu.DataRegs[0]) != GEMDOS_EIMBA {
		t.Fatalf("D0=%d, want %d", int32(cpu.DataRegs[0]), GEMDOS_EIMBA)
	}
}

// --- Close / lifecycle tests ---

func TestGemdos_Close(t *testing.T) {
	g, cpu, bus := newTestGemdos(t)

	os.WriteFile(filepath.Join(g.hostRoot, "LIFE.TXT"), []byte("data"), 0o644)

	fnameAddr := uint32(0x2000)
	writeGuestStringToBus(bus, fnameAddr, "U:\\LIFE.TXT")

	pushTrapFrameRaw(cpu, GEMDOS_FOPEN, []struct {
		size  int
		value uint32
	}{{4, fnameAddr}, {2, GEMDOS_OPEN_READ}})
	g.HandleTrap1()

	if len(g.handles) != 1 {
		t.Fatalf("expected 1 open handle, got %d", len(g.handles))
	}

	g.Close()

	if len(g.handles) != 0 {
		t.Fatalf("expected 0 handles after Close, got %d", len(g.handles))
	}
	if cpu.gemdosHandler != nil {
		t.Fatal("cpu.gemdosHandler should be nil after Close")
	}
}

func TestGemdos_Lifecycle_ResetRecreate(t *testing.T) {
	dir := t.TempDir()
	bus := NewMachineBus()
	cpu := NewM68KCPU(bus)

	os.WriteFile(filepath.Join(dir, "TEST.TXT"), []byte("data"), 0o644)

	// Create first interceptor, open a file
	g1, err := NewGemdosInterceptor(cpu, bus, dir, 20)
	if err != nil {
		t.Fatal(err)
	}
	cpu.gemdosHandler = g1

	fnameAddr := uint32(0x2000)
	writeGuestStringToBus(bus, fnameAddr, "U:\\TEST.TXT")
	pushTrapFrameRaw(cpu, GEMDOS_FOPEN, []struct {
		size  int
		value uint32
	}{{4, fnameAddr}, {2, GEMDOS_OPEN_READ}})
	g1.HandleTrap1()
	h1 := int16(cpu.DataRegs[0])

	// Close first interceptor
	g1.Close()
	if cpu.gemdosHandler != nil {
		t.Fatal("handler should be nil after Close")
	}

	// Create second interceptor
	g2, err := NewGemdosInterceptor(cpu, bus, dir, 20)
	if err != nil {
		t.Fatal(err)
	}
	cpu.gemdosHandler = g2

	// New interceptor should start handles from 1000 again
	writeGuestStringToBus(bus, fnameAddr, "U:\\TEST.TXT")
	pushTrapFrameRaw(cpu, GEMDOS_FOPEN, []struct {
		size  int
		value uint32
	}{{4, fnameAddr}, {2, GEMDOS_OPEN_READ}})
	g2.HandleTrap1()
	h2 := int16(cpu.DataRegs[0])

	if h2 != GEMDOS_HANDLE_MIN {
		t.Fatalf("new interceptor should start at %d, got %d", GEMDOS_HANDLE_MIN, h2)
	}
	_ = h1

	g2.Close()
}

// --- Fsfirst/Fsnext tests ---

func TestGemdos_Fsfirst_Fsnext(t *testing.T) {
	g, cpu, bus := newTestGemdos(t)
	defer g.Close()

	// Create test files
	os.WriteFile(filepath.Join(g.hostRoot, "FILE1.TXT"), []byte("one"), 0o644)
	os.WriteFile(filepath.Join(g.hostRoot, "FILE2.TXT"), []byte("two"), 0o644)
	os.WriteFile(filepath.Join(g.hostRoot, "OTHER.DOC"), []byte("doc"), 0o644)

	// Set DTA address
	dtaAddr := uint32(0x5000)
	g.dtaAddr = dtaAddr

	// Fsfirst "U:\*.TXT"
	fspecAddr := uint32(0x2000)
	writeGuestStringToBus(bus, fspecAddr, "U:\\*.TXT")

	pushTrapFrameRaw(cpu, GEMDOS_FSFIRST, []struct {
		size  int
		value uint32
	}{{4, fspecAddr}, {2, 0}})
	if !g.HandleTrap1() {
		t.Fatal("Fsfirst should be handled")
	}
	if cpu.DataRegs[0] != 0 {
		t.Fatalf("Fsfirst D0=%d, want 0", cpu.DataRegs[0])
	}

	// Read filename from DTA
	name1 := readDTAName(bus, dtaAddr)
	if !strings.HasSuffix(name1, ".TXT") {
		t.Fatalf("first result=%q, expected .TXT file", name1)
	}

	// Fsnext
	pushTrapFrame(cpu, GEMDOS_FSNEXT)
	if !g.HandleTrap1() {
		t.Fatal("Fsnext should be handled")
	}
	if cpu.DataRegs[0] != 0 {
		t.Fatalf("Fsnext D0=%d, want 0", cpu.DataRegs[0])
	}

	name2 := readDTAName(bus, dtaAddr)
	if !strings.HasSuffix(name2, ".TXT") {
		t.Fatalf("second result=%q, expected .TXT file", name2)
	}
	if name1 == name2 {
		t.Fatalf("Fsfirst and Fsnext returned the same file: %s", name1)
	}

	// Third Fsnext should return ENMFIL
	pushTrapFrame(cpu, GEMDOS_FSNEXT)
	g.HandleTrap1()
	if int32(cpu.DataRegs[0]) != GEMDOS_ENMFIL {
		t.Fatalf("third Fsnext D0=%d, want %d (ENMFIL)", int32(cpu.DataRegs[0]), GEMDOS_ENMFIL)
	}
}

// --- Directory operation tests ---

func TestGemdos_Dsetpath_Dgetpath(t *testing.T) {
	g, cpu, bus := newTestGemdos(t)
	defer g.Close()

	// Create subdirectory
	os.MkdirAll(filepath.Join(g.hostRoot, "MYDIR"), 0o755)

	g.defaultDrive = 20

	// Dsetpath "U:\MYDIR"
	pathAddr := uint32(0x2000)
	writeGuestStringToBus(bus, pathAddr, "U:\\MYDIR")

	pushTrapFrameRaw(cpu, GEMDOS_DSETPATH, []struct {
		size  int
		value uint32
	}{{4, pathAddr}})
	if !g.HandleTrap1() {
		t.Fatal("Dsetpath should be handled")
	}
	if cpu.DataRegs[0] != 0 {
		t.Fatalf("Dsetpath D0=%d, want 0", cpu.DataRegs[0])
	}

	// Dgetpath
	bufAddr := uint32(0x4000)
	// drv=0 means default (which is U:)
	pushTrapFrameRaw(cpu, GEMDOS_DGETPATH, []struct {
		size  int
		value uint32
	}{{4, bufAddr}, {2, 0}})
	if !g.HandleTrap1() {
		t.Fatal("Dgetpath should be handled")
	}

	// Read result
	var pathBuf []byte
	for i := uint32(0); i < 128; i++ {
		b := bus.Read8(bufAddr + i)
		if b == 0 {
			break
		}
		pathBuf = append(pathBuf, b)
	}
	pathStr := string(pathBuf)
	if pathStr != "\\MYDIR" {
		t.Fatalf("Dgetpath=%q, want %q", pathStr, "\\MYDIR")
	}
}

func TestGemdos_Dcreate_Ddelete(t *testing.T) {
	g, cpu, bus := newTestGemdos(t)
	defer g.Close()

	// Dcreate "U:\NEWDIR"
	pathAddr := uint32(0x2000)
	writeGuestStringToBus(bus, pathAddr, "U:\\NEWDIR")

	pushTrapFrameRaw(cpu, GEMDOS_DCREATE, []struct {
		size  int
		value uint32
	}{{4, pathAddr}})
	if !g.HandleTrap1() {
		t.Fatal("Dcreate should be handled")
	}
	if cpu.DataRegs[0] != 0 {
		t.Fatalf("Dcreate D0=%d, want 0", cpu.DataRegs[0])
	}

	// Verify on host
	info, err := os.Stat(filepath.Join(g.hostRoot, "NEWDIR"))
	if err != nil || !info.IsDir() {
		t.Fatal("directory was not created on host")
	}

	// Ddelete
	pushTrapFrameRaw(cpu, GEMDOS_DDELETE, []struct {
		size  int
		value uint32
	}{{4, pathAddr}})
	if !g.HandleTrap1() {
		t.Fatal("Ddelete should be handled")
	}
	if cpu.DataRegs[0] != 0 {
		t.Fatalf("Ddelete D0=%d, want 0", cpu.DataRegs[0])
	}

	// Verify deleted
	_, err = os.Stat(filepath.Join(g.hostRoot, "NEWDIR"))
	if err == nil {
		t.Fatal("directory should have been deleted")
	}
}

// --- DrvBits injection test ---

func TestGemdos_DrvbitsInjection(t *testing.T) {
	g, cpu, _ := newTestGemdos(t)
	defer g.Close()

	// Before any trap call, drvbits should not have bit 20
	initial := cpu.Read32(GEMDOS_DRVBITS_ADDR)
	if initial&(1<<20) != 0 {
		t.Fatal("_drvbits should not have bit 20 before any call")
	}

	// Trigger a trap call (Dgetdrv when not our drive — should still inject drvbits)
	pushTrapFrame(cpu, GEMDOS_DGETDRV)
	g.HandleTrap1() // Returns false but runs ensureDrvbits

	// Check drvbits in memory (read via cpu for BE)
	drvbits := cpu.Read32(GEMDOS_DRVBITS_ADDR)
	if drvbits&(1<<20) == 0 {
		t.Fatal("_drvbits should have bit 20 set after HandleTrap1")
	}
}

func TestGemdos_PollDrvbits(t *testing.T) {
	g, cpu, _ := newTestGemdos(t)
	defer g.Close()

	// PollDrvbits should set bit 20 even without any TRAP call
	g.PollDrvbits()
	drvbits := cpu.Read32(GEMDOS_DRVBITS_ADDR)
	if drvbits&(1<<20) == 0 {
		t.Fatal("PollDrvbits should set bit 20 in _drvbits")
	}

	// Simulate EmuTOS re-initialising _drvbits (clearing our bit)
	cpu.Write32(GEMDOS_DRVBITS_ADDR, 0x00000004) // only C: bit
	g.PollDrvbits()
	drvbits = cpu.Read32(GEMDOS_DRVBITS_ADDR)
	if drvbits&(1<<20) == 0 {
		t.Fatal("PollDrvbits should re-set bit 20 after EmuTOS overwrites _drvbits")
	}
	if drvbits&0x04 == 0 {
		t.Fatal("PollDrvbits should preserve existing drive bits")
	}
}

// --- Pexec tests ---

// buildMinimalPRG creates a TOS .PRG binary with a small text segment and
// optional relocation. The text segment is: move.l #data_label,a0; clr.w -(sp); trap #1; dc.l 0
// This tests both loading and relocation.
func buildMinimalPRG(withReloc bool) []byte {
	// Text segment (14 bytes):
	// +0: 207C XXXX XXXX  move.l #addr,a0  (6 bytes, immediate = 10 = offset of data_label)
	// +6: 4267             clr.w -(sp)       (2 bytes)
	// +8: 4E41             trap #1           (2 bytes)
	// +10: 00000000        dc.l 0            (4 bytes) ← data_label
	text := []byte{
		0x20, 0x7C, 0x00, 0x00, 0x00, 0x0A, // move.l #10,a0
		0x42, 0x67, // clr.w -(sp)
		0x4E, 0x41, // trap #1
		0x00, 0x00, 0x00, 0x00, // dc.l 0 (data_label)
	}
	textSize := uint32(len(text))

	// Header (28 bytes)
	header := make([]byte, TOS_PRG_HEADER_LEN)
	binary.BigEndian.PutUint16(header[0:2], TOS_PRG_MAGIC)
	binary.BigEndian.PutUint32(header[2:6], textSize)
	// data=0, bss=0, sym=0, reserved=0, flags=0

	prg := append(header, text...)

	// Relocation table
	if withReloc {
		// First offset = 2 (the immediate at text+2), then 0 = end
		reloc := make([]byte, 5)
		binary.BigEndian.PutUint32(reloc[0:4], 2) // offset of first reloc from text start
		reloc[4] = 0                              // end
		prg = append(prg, reloc...)
	} else {
		// No relocations: first long = 0
		prg = append(prg, 0, 0, 0, 0)
	}
	return prg
}

func buildPRGWithSizes(textSize, dataSize, bssSize, symSize uint32, body []byte, reloc []byte) []byte {
	header := make([]byte, TOS_PRG_HEADER_LEN)
	binary.BigEndian.PutUint16(header[0:2], TOS_PRG_MAGIC)
	binary.BigEndian.PutUint32(header[2:6], textSize)
	binary.BigEndian.PutUint32(header[6:10], dataSize)
	binary.BigEndian.PutUint32(header[10:14], bssSize)
	binary.BigEndian.PutUint32(header[14:18], symSize)
	prg := append(header, body...)
	prg = append(prg, reloc...)
	return prg
}

func TestGemdos_Pexec_LoadAndRelocate(t *testing.T) {
	g, cpu, bus := newTestGemdos(t)
	defer g.Close()

	// Write a minimal PRG with relocation to the host directory
	prg := buildMinimalPRG(true)
	os.WriteFile(filepath.Join(g.hostRoot, "TEST.PRG"), prg, 0o644)

	// Save "parent" state
	parentPC := uint32(0x1234)
	parentSP := uint32(0x8000)
	cpu.PC = parentPC
	cpu.AddrRegs[7] = parentSP
	cpu.DataRegs[1] = 0xDEADBEEF // should be saved and restored

	// Push Pexec(0, "U:\TEST.PRG", 0, 0) on stack
	fnameAddr := uint32(0x2000)
	writeGuestStringToBus(bus, fnameAddr, "U:\\TEST.PRG")

	pushTrapFrameRaw(cpu, GEMDOS_PEXEC, []struct {
		size  int
		value uint32
	}{
		{2, GEMDOS_PEXEC_LOAD_GO},
		{4, fnameAddr},
		{4, 0}, // cmdline
		{4, 0}, // envstr
	})

	handled := g.HandleTrap1()
	if !handled {
		t.Fatal("Pexec should be handled for U: drive")
	}

	// Verify CPU state was redirected to child
	textBase := uint32(PEXEC_TPA_BASE) + TOS_BASEPAGE_SIZE
	if cpu.PC != textBase {
		t.Fatalf("PC=$%06X, want $%06X (text base)", cpu.PC, textBase)
	}

	// Verify basepage at TPA base
	bpAddr := uint32(PEXEC_TPA_BASE)
	pLowtpa := cpu.Read32(bpAddr + 0)
	pTbase := cpu.Read32(bpAddr + 8)
	pTlen := cpu.Read32(bpAddr + 12)
	if pLowtpa != PEXEC_TPA_BASE {
		t.Fatalf("p_lowtpa=$%06X, want $%06X", pLowtpa, PEXEC_TPA_BASE)
	}
	if pTbase != textBase {
		t.Fatalf("p_tbase=$%06X, want $%06X", pTbase, textBase)
	}
	if pTlen != 14 {
		t.Fatalf("p_tlen=%d, want 14", pTlen)
	}

	// Verify relocation: the immediate at text+2 should be textBase+10
	relocatedVal := cpu.Read32(textBase + 2)
	expectedVal := textBase + 10
	if relocatedVal != expectedVal {
		t.Fatalf("relocated value at text+2: $%08X, want $%08X", relocatedVal, expectedVal)
	}

	// Verify 4(SP) = basepage
	childSP := cpu.AddrRegs[7]
	bpOnStack := cpu.Read32(childSP + 4)
	if bpOnStack != bpAddr {
		t.Fatalf("basepage at 4(SP): $%06X, want $%06X", bpOnStack, bpAddr)
	}

	// Verify pexecState was saved
	if g.pexecState == nil {
		t.Fatal("pexecState should be saved")
	}
	if g.pexecState.pc != parentPC {
		t.Fatalf("saved PC=$%06X, want $%06X", g.pexecState.pc, parentPC)
	}

	// Now simulate child calling Pterm0
	pushTrapFrame(cpu, 0x00) // Pterm0
	handled = g.HandleTrap1()
	if !handled {
		t.Fatal("Pterm0 should be handled during Pexec")
	}

	// Verify parent state was restored
	if cpu.PC != parentPC {
		t.Fatalf("restored PC=$%06X, want $%06X", cpu.PC, parentPC)
	}
	if cpu.DataRegs[1] != 0xDEADBEEF {
		t.Fatalf("restored D1=$%08X, want $DEADBEEF", cpu.DataRegs[1])
	}
	if cpu.DataRegs[0] != 0 {
		t.Fatalf("exit code D0=%d, want 0", cpu.DataRegs[0])
	}
	if g.pexecState != nil {
		t.Fatal("pexecState should be nil after Pterm")
	}
}

func TestGemdos_Pexec_MshrinkIntercept(t *testing.T) {
	g, cpu, bus := newTestGemdos(t)
	defer g.Close()

	// Write PRG and launch it
	prg := buildMinimalPRG(false)
	os.WriteFile(filepath.Join(g.hostRoot, "SHRINK.PRG"), prg, 0o644)

	fnameAddr := uint32(0x2000)
	writeGuestStringToBus(bus, fnameAddr, "U:\\SHRINK.PRG")
	cpu.PC = 0x1234
	pushTrapFrameRaw(cpu, GEMDOS_PEXEC, []struct {
		size  int
		value uint32
	}{
		{2, GEMDOS_PEXEC_LOAD_GO},
		{4, fnameAddr},
		{4, 0},
		{4, 0},
	})
	g.HandleTrap1()

	// Now simulate child calling Mshrink on our TPA
	pushTrapFrameRaw(cpu, GEMDOS_MSHRINK, []struct {
		size  int
		value uint32
	}{
		{2, 0},                       // reserved
		{4, PEXEC_TPA_BASE},          // block address
		{4, TOS_BASEPAGE_SIZE + 100}, // new size
	})
	handled := g.HandleTrap1()
	if !handled {
		t.Fatal("Mshrink for our TPA should be intercepted")
	}
	if cpu.DataRegs[0] != 0 {
		t.Fatalf("Mshrink D0=%d, want 0 (success)", cpu.DataRegs[0])
	}

	// Mshrink for a different address should NOT be intercepted
	pushTrapFrameRaw(cpu, GEMDOS_MSHRINK, []struct {
		size  int
		value uint32
	}{
		{2, 0},
		{4, 0x60000}, // some other address
		{4, 100},
	})
	handled = g.HandleTrap1()
	if handled {
		t.Fatal("Mshrink for non-TPA address should NOT be intercepted")
	}
}

func TestGemdos_Pexec_NotOurDrive(t *testing.T) {
	g, cpu, _ := newTestGemdos(t)
	defer g.Close()

	// Pexec for a path on drive A: should not be handled
	fnameAddr := uint32(0x2000)
	writeGuestStringToBus(g.bus, fnameAddr, "A:\\PROGRAM.PRG")

	pushTrapFrameRaw(cpu, GEMDOS_PEXEC, []struct {
		size  int
		value uint32
	}{
		{2, GEMDOS_PEXEC_LOAD_GO},
		{4, fnameAddr},
		{4, 0},
		{4, 0},
	})
	handled := g.HandleTrap1()
	if handled {
		t.Fatal("Pexec for A: drive should NOT be handled")
	}
}

func TestGemdos_PexecRejectsTPAOverflow(t *testing.T) {
	g, cpu, bus := newTestGemdos(t)
	defer g.Close()

	body := make([]byte, 0x100)
	prg := buildPRGWithSizes(0xFFFFFF00, 0x200, 0, 0, body, nil)
	if err := os.WriteFile(filepath.Join(g.hostRoot, "OVER.PRG"), prg, 0o644); err != nil {
		t.Fatal(err)
	}
	fnameAddr := uint32(0x2000)
	writeGuestStringToBus(bus, fnameAddr, "U:\\OVER.PRG")
	pushTrapFrameRaw(cpu, GEMDOS_PEXEC, []struct {
		size  int
		value uint32
	}{{2, GEMDOS_PEXEC_LOAD_GO}, {4, fnameAddr}, {4, 0}, {4, 0}})

	if !g.HandleTrap1() {
		t.Fatal("Pexec should be handled")
	}
	if int32(cpu.DataRegs[0]) != GEMDOS_EIMBA {
		t.Fatalf("D0=%d, want %d", int32(cpu.DataRegs[0]), GEMDOS_EIMBA)
	}
}

func TestGemdos_PexecRejectsRelocOutOfRange(t *testing.T) {
	g, cpu, bus := newTestGemdos(t)
	defer g.Close()

	body := []byte{0, 0, 0, 0}
	reloc := make([]byte, 5)
	binary.BigEndian.PutUint32(reloc[0:4], 8)
	prg := buildPRGWithSizes(4, 0, 0, 0, body, reloc)
	if err := os.WriteFile(filepath.Join(g.hostRoot, "BADREL.PRG"), prg, 0o644); err != nil {
		t.Fatal(err)
	}
	fnameAddr := uint32(0x2000)
	writeGuestStringToBus(bus, fnameAddr, "U:\\BADREL.PRG")
	pushTrapFrameRaw(cpu, GEMDOS_PEXEC, []struct {
		size  int
		value uint32
	}{{2, GEMDOS_PEXEC_LOAD_GO}, {4, fnameAddr}, {4, 0}, {4, 0}})

	if !g.HandleTrap1() {
		t.Fatal("Pexec should be handled")
	}
	if int32(cpu.DataRegs[0]) != GEMDOS_EIMBA {
		t.Fatalf("D0=%d, want %d", int32(cpu.DataRegs[0]), GEMDOS_EIMBA)
	}
}

func TestGemdos_PexecAcceptsMaxValidTPA(t *testing.T) {
	g, cpu, bus := newTestGemdos(t)
	defer g.Close()

	maxSize := EmuTOS_PROFILE_TOP - PEXEC_TPA_BASE
	bssSize := maxSize - TOS_BASEPAGE_SIZE - 8192
	prg := buildPRGWithSizes(0, 0, bssSize, 0, nil, []byte{0, 0, 0, 0})
	if err := os.WriteFile(filepath.Join(g.hostRoot, "MAX.PRG"), prg, 0o644); err != nil {
		t.Fatal(err)
	}
	fnameAddr := uint32(0x2000)
	writeGuestStringToBus(bus, fnameAddr, "U:\\MAX.PRG")
	pushTrapFrameRaw(cpu, GEMDOS_PEXEC, []struct {
		size  int
		value uint32
	}{{2, GEMDOS_PEXEC_LOAD_GO}, {4, fnameAddr}, {4, 0}, {4, 0}})

	if !g.HandleTrap1() {
		t.Fatal("Pexec should be handled")
	}
	if int32(cpu.DataRegs[0]) < 0 {
		t.Fatalf("Pexec unexpectedly failed: D0=%d", int32(cpu.DataRegs[0]))
	}
	if got := cpu.Read32(PEXEC_TPA_BASE + 4); got != EmuTOS_PROFILE_TOP {
		t.Fatalf("p_hitpa=0x%X, want 0x%X", got, EmuTOS_PROFILE_TOP)
	}
}

func TestGemdos_SymlinkEscapeRejected(t *testing.T) {
	g, _, _ := newTestGemdos(t)
	defer g.Close()

	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "SECRET.TXT"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(g.hostRoot, "LINK")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if _, ok := g.resolvePathForOurDrive("U:\\LINK\\SECRET.TXT"); ok {
		t.Fatal("symlink escape should be rejected")
	}
}

func TestGemdos_FcreateResolvesParentOnly(t *testing.T) {
	g, cpu, bus := newTestGemdos(t)
	defer g.Close()
	if err := os.Mkdir(filepath.Join(g.hostRoot, "DIR"), 0o755); err != nil {
		t.Fatal(err)
	}
	addr := uint32(0x2000)
	writeGuestStringToBus(bus, addr, "U:\\DIR\\NEW.TXT")
	pushTrapFrameRaw(cpu, GEMDOS_FCREATE, []struct {
		size  int
		value uint32
	}{{4, addr}, {2, 0}})
	if !g.HandleTrap1() || int32(cpu.DataRegs[0]) < 0 {
		t.Fatalf("Fcreate failed: handled D0=%d", int32(cpu.DataRegs[0]))
	}
}

func TestGemdos_FcreateNotOurDriveFallsThrough(t *testing.T) {
	g, cpu, bus := newTestGemdos(t)
	defer g.Close()
	addr := uint32(0x2000)
	writeGuestStringToBus(bus, addr, "C:\\OTHER.TXT")
	pushTrapFrameRaw(cpu, GEMDOS_FCREATE, []struct {
		size  int
		value uint32
	}{{4, addr}, {2, 0}})
	if g.HandleTrap1() {
		t.Fatal("Fcreate for non-mapped drive should fall through to EmuTOS")
	}
}

func TestGemdos_FcreateExistingTargetResolvesCaseInsensitive(t *testing.T) {
	g, cpu, bus := newTestGemdos(t)
	defer g.Close()
	existing := filepath.Join(g.hostRoot, "Mixed.Txt")
	if err := os.WriteFile(existing, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	addr := uint32(0x2000)
	writeGuestStringToBus(bus, addr, "U:\\MIXED.TXT")
	pushTrapFrameRaw(cpu, GEMDOS_FCREATE, []struct {
		size  int
		value uint32
	}{{4, addr}, {2, 0}})
	if !g.HandleTrap1() || int32(cpu.DataRegs[0]) < 0 {
		t.Fatalf("Fcreate failed: handled D0=%d", int32(cpu.DataRegs[0]))
	}
	if _, err := os.Stat(filepath.Join(g.hostRoot, "MIXED.TXT")); err == nil {
		t.Fatal("Fcreate created a second case-variant file instead of resolving existing target")
	}
	if _, err := os.Stat(existing); err != nil {
		t.Fatalf("existing file missing after Fcreate: %v", err)
	}
}

func TestGemdos_DcreateResolvesParentOnly(t *testing.T) {
	g, cpu, bus := newTestGemdos(t)
	defer g.Close()
	if err := os.Mkdir(filepath.Join(g.hostRoot, "PARENT"), 0o755); err != nil {
		t.Fatal(err)
	}
	addr := uint32(0x2000)
	writeGuestStringToBus(bus, addr, "U:\\PARENT\\CHILD")
	pushTrapFrame(cpu, GEMDOS_DCREATE, addr)
	if !g.HandleTrap1() || cpu.DataRegs[0] != 0 {
		t.Fatalf("Dcreate failed: handled D0=%d", int32(cpu.DataRegs[0]))
	}
}

func TestGemdos_FrenameNewTargetResolvesParentOnly(t *testing.T) {
	g, cpu, bus := newTestGemdos(t)
	defer g.Close()
	if err := os.Mkdir(filepath.Join(g.hostRoot, "DIR"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(g.hostRoot, "OLD.TXT"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	oldAddr, newAddr := uint32(0x2000), uint32(0x2100)
	writeGuestStringToBus(bus, oldAddr, "U:\\OLD.TXT")
	writeGuestStringToBus(bus, newAddr, "U:\\DIR\\NEW.TXT")
	pushTrapFrameRaw(cpu, GEMDOS_FRENAME, []struct {
		size  int
		value uint32
	}{{2, 0}, {4, oldAddr}, {4, newAddr}})
	if !g.HandleTrap1() || cpu.DataRegs[0] != 0 {
		t.Fatalf("Frename failed: handled D0=%d", int32(cpu.DataRegs[0]))
	}
}

func TestGemdos_FrenameBothNotOurDriveFallsThrough(t *testing.T) {
	g, cpu, bus := newTestGemdos(t)
	defer g.Close()
	oldAddr, newAddr := uint32(0x2000), uint32(0x2100)
	writeGuestStringToBus(bus, oldAddr, "C:\\OLD.TXT")
	writeGuestStringToBus(bus, newAddr, "C:\\NEW.TXT")
	pushTrapFrameRaw(cpu, GEMDOS_FRENAME, []struct {
		size  int
		value uint32
	}{{2, 0}, {4, oldAddr}, {4, newAddr}})
	if g.HandleTrap1() {
		t.Fatal("Frename with both paths outside mapped drive should fall through to EmuTOS")
	}
}

func TestGemdos_FcreateRejectsParentSymlinkEscape(t *testing.T) {
	g, cpu, bus := newTestGemdos(t)
	defer g.Close()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(g.hostRoot, "LINK")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	addr := uint32(0x2000)
	writeGuestStringToBus(bus, addr, "U:\\LINK\\NEW.TXT")
	pushTrapFrameRaw(cpu, GEMDOS_FCREATE, []struct {
		size  int
		value uint32
	}{{4, addr}, {2, 0}})
	if !g.HandleTrap1() {
		t.Fatal("Fcreate should reject symlink parent itself")
	}
	if int32(cpu.DataRegs[0]) != GEMDOS_EACCDN {
		t.Fatalf("D0=%d, want %d", int32(cpu.DataRegs[0]), GEMDOS_EACCDN)
	}
}

func TestGemdos_FcreateRejectsLeafSymlinkEscape(t *testing.T) {
	g, cpu, bus := newTestGemdos(t)
	defer g.Close()
	outsideFile := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outsideFile, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outsideFile, filepath.Join(g.hostRoot, "LEAF.TXT")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	addr := uint32(0x2000)
	writeGuestStringToBus(bus, addr, "U:\\LEAF.TXT")
	pushTrapFrameRaw(cpu, GEMDOS_FCREATE, []struct {
		size  int
		value uint32
	}{{4, addr}, {2, 0}})
	if !g.HandleTrap1() {
		t.Fatal("Fcreate should reject leaf symlink itself")
	}
	if int32(cpu.DataRegs[0]) != GEMDOS_EACCDN {
		t.Fatalf("D0=%d, want %d", int32(cpu.DataRegs[0]), GEMDOS_EACCDN)
	}
	if got, err := os.ReadFile(outsideFile); err != nil || string(got) != "keep" {
		t.Fatalf("outside file mutated or unreadable: data=%q err=%v", string(got), err)
	}
}

func TestGemdos_FrenameRejectsTargetSymlinkEscape(t *testing.T) {
	g, cpu, bus := newTestGemdos(t)
	defer g.Close()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(g.hostRoot, "LINK")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if err := os.WriteFile(filepath.Join(g.hostRoot, "OLD.TXT"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	oldAddr, newAddr := uint32(0x2000), uint32(0x2100)
	writeGuestStringToBus(bus, oldAddr, "U:\\OLD.TXT")
	writeGuestStringToBus(bus, newAddr, "U:\\LINK\\NEW.TXT")
	pushTrapFrameRaw(cpu, GEMDOS_FRENAME, []struct {
		size  int
		value uint32
	}{{2, 0}, {4, oldAddr}, {4, newAddr}})
	if !g.HandleTrap1() {
		t.Fatal("Frename should reject symlink target itself")
	}
	if int32(cpu.DataRegs[0]) != GEMDOS_EACCDN {
		t.Fatalf("D0=%d, want %d", int32(cpu.DataRegs[0]), GEMDOS_EACCDN)
	}
}

func TestGemdos_DotDotStillRejected(t *testing.T) {
	g, _, _ := newTestGemdos(t)
	defer g.Close()
	if _, ok := g.resolvePathForOurDrive("U:\\..\\ESCAPE"); ok {
		t.Fatal("dot-dot path should be rejected")
	}
}

func TestGemdos_FixWnodeChainSkipsOnSizeMismatch(t *testing.T) {
	g, cpu, _ := newTestGemdos(t)
	defer g.Close()
	base := uint32(0x6000)
	last := base - 342
	prev := last - 342
	cpu.Write32(prev, last)
	cpu.Write32(last, base)
	g.fnodeBase = base
	g.fnodeSize = 1
	g.returnedCount = 1
	g.fixWnodeChain()
	if got := cpu.Read32(last); got != base {
		t.Fatalf("w_next=0x%X, want unchanged 0x%X", got, base)
	}
}

// --- Helpers ---

func readDTAName(bus *MachineBus, dtaAddr uint32) string {
	var buf []byte
	for i := uint32(0); i < 14; i++ {
		b := bus.Read8(dtaAddr + GEMDOS_DTA_NAME + i)
		if b == 0 {
			break
		}
		buf = append(buf, b)
	}
	return string(buf)
}
