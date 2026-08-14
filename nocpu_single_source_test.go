//go:build headless

package main

import (
	"bytes"
	"encoding/binary"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

func copyNoCPUTestInput(t *testing.T, root, destination, source string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, source))
	if err != nil {
		t.Fatalf("read %s: %v", source, err)
	}
	target := filepath.Join(destination, source)
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("create directory for %s: %v", source, err)
	}
	if err := os.WriteFile(target, data, 0o644); err != nil {
		t.Fatalf("write %s: %v", target, err)
	}
}

func TestNoCPUAssemblyUsesOnlyPermittedFileDependencies(t *testing.T) {
	source, err := os.ReadFile("sdk/examples/asm/rotozoomer_nocpu.asm")
	if err != nil {
		t.Fatalf("read no-CPU assembly: %v", err)
	}
	directive := regexp.MustCompile(`(?i)^\s*(include|incbin)\s+"([^"]+)"`)
	var dependencies []string
	for _, line := range strings.Split(string(source), "\n") {
		match := directive.FindStringSubmatch(line)
		if match != nil {
			dependencies = append(dependencies, strings.ToLower(match[1])+" "+match[2])
		}
	}
	want := []string{
		`include ie64.inc`,
		`incbin ../assets/music/yourlove.mid`,
		`incbin ../assets/rotozoomtexture_nocpu.raw`,
	}
	if !slices.Equal(dependencies, want) {
		t.Fatalf("no-CPU assembly dependencies = %q, want %q", dependencies, want)
	}
}

func TestNoCPUSingleSourceCodeStaysCompact(t *testing.T) {
	source, err := os.ReadFile("sdk/examples/asm/rotozoomer_nocpu.asm")
	if err != nil {
		t.Fatalf("read no-CPU assembly: %v", err)
	}
	lines := strings.Split(string(source), "\n")
	codeLines := 0
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" && !strings.HasPrefix(trimmed, ";") {
			codeLines++
		}
	}
	if codeLines > 300 {
		t.Fatalf("no-CPU assembly has %d code lines, want no more than 300", codeLines)
	}
}

func TestNoCPUAssemblyCommentsExplainTheCompleteCircuit(t *testing.T) {
	source, err := os.ReadFile("sdk/examples/asm/rotozoomer_nocpu.asm")
	if err != nil {
		t.Fatalf("read no-CPU assembly: %v", err)
	}
	var comments strings.Builder
	for _, line := range strings.Split(string(source), "\n") {
		if index := strings.IndexByte(line, ';'); index >= 0 {
			comments.WriteString(line[index+1:])
			comments.WriteByte('\n')
		}
	}
	text := strings.Join(strings.Fields(strings.ToLower(comments.String())), " ")
	for _, required := range []string{
		"make nocpu-rotozoomer",
		"live usb image",
		"dma blitter with mode 7",
		"raster effects unit",
		"standard midi file",
		"no runtime file dependency",
		"65,536 compact affine records",
		"clears the circuit state",
		"bootstrap stage 1: clear the circuit state",
		"reciprocal multiplication",
		"16.16",
		"source stride of one byte",
		"destination stride of four bytes",
		"unified memory",
		"640-byte clut8 raster band",
		"ordered suffix writes",
		"bit-sliced",
		"16 bit planes",
		"all-zero or all-one masks",
		"angle and scale planes alternate in memory",
		"each active plane occupies 0x4000 bytes",
		"phase_bit_stride advances 0x8000 bytes",
		"zero source",
		"lookup workspace",
		"carry mask",
		"expanded affine lookup tables and angle-address lanes",
		"zero padding for the expanded and angle-address lanes",
		"low byte of each 32-bit lane",
		"upper three bytes remain zero for rgba32 boolean blits",
		"carry",
		"blt_op remains zero from the frame's shadow clear",
		"height suffix",
		"restores both strides to zero before blt_ctrl",
		"complete four-byte copper operand",
		"video_color_mode selects clut8 raster bands",
		"blt_flags independently selects rgba32 for boolean arithmetic and operand-copy blits",
		"boolean blits derive the sum bit",
		"core blitter and mode 7 shadow",
		"blt_flags lies beyond the 640-byte band",
		"written separately for every command",
		"angle table address",
		"six mode 7 parameters",
		"two signed 16.16 origins and four signed 16.16 increments",
		"later copper operands",
		"presentation hold",
		"first frame primes presentation hold",
		"later frames retain the preceding completed frame",
		"advances both phases for the following frame",
		"ptr/len/ctrl protocol",
		"volume is 180 out of 255",
		"control value 5 starts playback and enables looping",
		"blt_flags selects rgba32 mode 7 pixels",
		"video_color_mode selects rgba32 presentation",
		"blt_ctrl completes the mode 7 render synchronously",
		"raster suffixes leave video_fb_base pointing into the register shadow",
		"restore video_fb_base to framebuffer_base before releasing presentation hold",
		"wait for scanline 1",
		"60 hz",
		"ie64 remains halted",
		"gplv3 or later",
	} {
		if !strings.Contains(text, required) {
			t.Errorf("no-CPU assembly comments do not explain %q", required)
		}
	}
	for _, misleading := range []string{
		"build a complete synchronous blitter command",
		"complete blitter shadow",
		"bit-sliced phase lane",
		"carry lanes",
		"zero bits for unused lanes",
		"clears the arithmetic state",
		"bootstrap stage 1: clear the expanded state",
		"because no completed image exists",
		"presentation hold retains the preceding completed frame while",
		"the raster operations derive",
		"through 16 lanes",
		"selected values are 16.16 coordinates",
		"mode 7 blitter",
		"updates its phases, selects the six mode 7 parameters",
	} {
		if strings.Contains(text, misleading) {
			t.Errorf("no-CPU assembly comments retain misleading text %q", misleading)
		}
	}
}

func TestNoCPUAssemblyCommentsUseProductLanguage(t *testing.T) {
	source, err := os.ReadFile("sdk/examples/asm/rotozoomer_nocpu.asm")
	if err != nil {
		t.Fatalf("read no-CPU assembly: %v", err)
	}
	var comments strings.Builder
	for _, line := range strings.Split(string(source), "\n") {
		if index := strings.IndexByte(line, ';'); index >= 0 {
			comments.WriteString(line[index+1:])
			comments.WriteByte('\n')
		}
	}
	text := strings.ToLower(comments.String())
	for _, forbidden := range []string{
		`[—–]`, `\bemulat(or|ion|ed|ing)\b`, `\bhost\b`, `\bguest\b`,
		`\bcolors?\b`, `\bcolored\b`, `\bprogram(s|med|ming)?\b`,
		`\binitializ(e|ed|es|ing)\b`, `\bbehaviors?\b`, `\bcenters?\b`,
		`\boptimized?\b`, `\blicense[ds]?\b`,
	} {
		if regexp.MustCompile(forbidden).FindStringIndex(text) != nil {
			t.Errorf("no-CPU assembly comments contain unsuitable text %q", forbidden)
		}
	}
}

func TestNoCPUAssemblyOmitsOpaqueNames(t *testing.T) {
	source, err := os.ReadFile("sdk/examples/asm/rotozoomer_nocpu.asm")
	if err != nil {
		t.Fatalf("read no-CPU assembly: %v", err)
	}
	for _, forbidden := range []string{
		"NOCPU_RENDER_A", "NOCPU_COPPER_BASE", "NOCPU_STATE_SIZE",
		"NOCPU_LOOKUP_WORK", "NOCPU_LOOKUP_TEMP", "NOCPU_CARRY",
		"NOCPU_FRAME_TABLES", "NOCPU_ADDRESS_TABLE", "NOCPU_COMPACT_TABLES",
		"NOCPU_REPEAT_WORDS", "NOCPU_FRAME_WORDS", "DRAW_NOT",
		"copper_shadow_byte", "copper_shadow_bytes",
		"copper_shadow_word_target", "copper_lookup_stages",
		"copper_patch_word", "copper_add_constant_bit",
		"copper_add_constant", "write_value", "angle_loop", "scale_loop",
		"table_source_b0", "mode7_u0_b0",
	} {
		if bytes.Contains(source, []byte(forbidden)) {
			t.Errorf("no-CPU assembly retains opaque name %q", forbidden)
		}
	}
}

func TestNoCPUTestsOmitGeneratedLayoutDiagnostics(t *testing.T) {
	source, err := os.ReadFile("nocpu_rotozoomer_copper_test.go")
	if err != nil {
		t.Fatalf("read no-CPU Copper tests: %v", err)
	}
	for _, forbidden := range []string{"0x8AC", "nocpuOperandTargets"} {
		if bytes.Contains(source, []byte(forbidden)) {
			t.Errorf("no-CPU Copper tests retain generated-layout diagnostic %q", forbidden)
		}
	}
}

func TestNoCPUSourceOmitsOneShotAndPassThroughMachinery(t *testing.T) {
	source, err := os.ReadFile("sdk/examples/asm/rotozoomer_nocpu.asm")
	if err != nil {
		t.Fatalf("read no-CPU assembly: %v", err)
	}
	for _, forbidden := range []string{
		"jsr     clear_state",
		"jsr     build_frame_tables",
		"clear_state:",
		"build_frame_tables:",
		"store_packed_word macro",
		"write_register macro",
		"copper_shadow_word macro",
		"copper_move_target macro",
		"copper_lookup_stage macro",
		"copper_add_bit_zero macro",
		"copper_add_bit_one macro",
		"mode7_dst_b0",
		"mode7_dst_b1",
		"mode7_dst_b2",
		"mode7_dst_b3",
		"presented_fb",
	} {
		if bytes.Contains(source, []byte(forbidden)) {
			t.Errorf("no-CPU assembly retains unnecessary source %q", forbidden)
		}
	}
}

func TestNoCPUBuildHasNoGeneratedCircuitDependency(t *testing.T) {
	makefile, err := os.ReadFile("Makefile")
	if err != nil {
		t.Fatalf("read Makefile: %v", err)
	}
	for _, forbidden := range []string{
		"NOCPU_ROTO_GENERATED",
		".nocpu-rotozoomer-generated.stamp",
		"tools/gen_nocpu_roto",
		"rotozoomer_nocpu_copper.bin",
		"rotozoomer_nocpu_state.bin",
		"rotozoomer_nocpu_targets.bin",
		"rotozoomer_nocpu.inc",
	} {
		if strings.Contains(string(makefile), forbidden) {
			t.Errorf("Makefile still references %q", forbidden)
		}
	}
}

func TestNoCPUObsoleteGeneratedCircuitFilesAreAbsent(t *testing.T) {
	for _, path := range []string{
		"sdk/examples/generated/rotozoomer_nocpu_copper.bin",
		"sdk/examples/generated/rotozoomer_nocpu_state.bin",
		"sdk/examples/generated/rotozoomer_nocpu_targets.bin",
		"sdk/examples/generated/rotozoomer_nocpu.inc",
		"tools/gen_nocpu_roto",
	} {
		if _, err := os.Stat(path); err == nil {
			t.Errorf("obsolete generated circuit dependency still exists: %s", path)
		} else if !os.IsNotExist(err) {
			t.Errorf("stat %s: %v", path, err)
		}
	}
}

func TestNoCPUAssemblyBuildsWithOnlyPermittedInputs(t *testing.T) {
	root := repoRootDir(t)
	isolated := t.TempDir()
	for _, input := range []string{
		"sdk/include/ie64.inc",
		"sdk/examples/asm/rotozoomer_nocpu.asm",
		"sdk/examples/assets/rotozoomtexture_nocpu.raw",
		"sdk/examples/assets/music/yourlove.mid",
	} {
		copyNoCPUTestInput(t, root, isolated, input)
	}
	output := filepath.Join(isolated, "rotozoomer_nocpu.ie64")
	command := exec.Command(buildAssembler(t),
		"-I", filepath.Join(isolated, "sdk", "include"),
		"-o", output,
		filepath.Join(isolated, "sdk", "examples", "asm", "rotozoomer_nocpu.asm"),
	)
	if combined, err := command.CombinedOutput(); err != nil {
		t.Fatalf("assemble isolated no-CPU source: %v\n%s", err, combined)
	}
	info, err := os.Stat(output)
	if err != nil {
		t.Fatalf("stat isolated no-CPU binary: %v", err)
	}
	if info.Size() == 0 {
		t.Fatal("isolated no-CPU binary is empty")
	}
}

func TestNoCPUSingleSourceCopperBudget(t *testing.T) {
	program := assembleNoCPUProgram(t)
	offset := int(nocpuCopperBase - PROG_START)
	end := offset + int(nocpuCopperSize)
	if offset < 0 || end > len(program) {
		t.Fatalf("Copper range [%d:%d] is outside %d-byte programme", offset, end, len(program))
	}
	list := program[offset:end]
	ops := 0
	segmentOps := 0
	maxSegmentOps := 0
	for pc := 0; pc < len(list); {
		word := binary.LittleEndian.Uint32(list[pc:])
		opcode := word >> copperOpcodeShift
		ops++
		switch opcode {
		case copperOpcodeWait:
			if segmentOps+1 > maxSegmentOps {
				maxSegmentOps = segmentOps + 1
			}
			segmentOps = 0
			pc += 4
		case copperOpcodeMove:
			segmentOps++
			pc += 8
		case copperOpcodeSetBase:
			segmentOps++
			pc += 4
		case copperOpcodeEnd:
			segmentOps++
			if segmentOps > maxSegmentOps {
				maxSegmentOps = segmentOps
			}
			pc = len(list)
		default:
			t.Fatalf("unknown Copper opcode %d at byte %d", opcode, pc)
		}
	}
	if ops != 7784 {
		t.Fatalf("Copper operations = %d, want 7784", ops)
	}
	if maxSegmentOps != 7779 {
		t.Fatalf("largest Copper segment = %d operations, want 7779", maxSegmentOps)
	}
}

func TestNoCPUFirstPartialBlitRestoresCompleteShadow(t *testing.T) {
	program := assembleNoCPUProgram(t)
	pc := int(nocpuCopperBase - PROG_START)
	end := pc + int(nocpuCopperSize)
	shadow := make(map[uint32]byte)
	var framebuffer, colour uint32
	started := false
	for pc < end {
		word := binary.LittleEndian.Uint32(program[pc:])
		switch word >> copperOpcodeShift {
		case copperOpcodeWait, copperOpcodeSetBase:
			pc += 4
		case copperOpcodeMove:
			value := binary.LittleEndian.Uint32(program[pc+4:])
			address := uint32(VIDEO_REG_BASE) + (((word >> copperRegShift) & copperRegMask) * 4)
			switch address {
			case VIDEO_FB_BASE:
				framebuffer = value
			case VIDEO_RASTER_COLOR:
				colour = value
			case VIDEO_RASTER_CTRL:
				if value == 1 {
					for offset := uint32(0); offset < 640; offset++ {
						shadow[framebuffer+offset] = byte(colour)
					}
				}
			case BLT_CTRL:
				started = value == 1
			}
			pc += 8
		case copperOpcodeEnd:
			pc = end
		}
		if started {
			break
		}
	}
	if !started {
		t.Fatal("Copper did not start its first arithmetic blit")
	}
	read := func(address uint32) uint32 {
		return uint32(shadow[address]) |
			uint32(shadow[address+1])<<8 |
			uint32(shadow[address+2])<<16 |
			uint32(shadow[address+3])<<24
	}
	for address, want := range map[uint32]uint32{
		BLT_OP: 0, BLT_SRC: nocpuAddrTable, BLT_DST: 0x1484000,
		BLT_WIDTH: 1024, BLT_HEIGHT: 1, BLT_SRC_STRIDE: 0,
		BLT_DST_STRIDE: 0, BLT_FLAGS: 3 << 4,
	} {
		if got := read(address); got != want {
			t.Errorf("first arithmetic shadow register %#x = %#x, want %#x", address, got, want)
		}
	}
}

func TestNoCPUPrebuiltMatchesSingleSource(t *testing.T) {
	prebuilt, err := os.ReadFile("sdk/examples/prebuilt/rotozoomer_nocpu.ie64")
	if err != nil {
		t.Fatalf("read no-CPU prebuilt: %v", err)
	}
	assembled := assembleNoCPUProgram(t)
	if !bytes.Equal(prebuilt, assembled) {
		t.Fatalf("no-CPU prebuilt differs from single-source assembly: prebuilt %d bytes, assembled %d bytes", len(prebuilt), len(assembled))
	}
}
