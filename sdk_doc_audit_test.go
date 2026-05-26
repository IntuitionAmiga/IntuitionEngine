package main

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

var sdkAuditDocs = []string{
	"sdk/docs/IE64_ISA.md",
	"sdk/docs/IE32_ISA.md",
	"sdk/docs/iemon.md",
	"sdk/docs/iescript.md",
	"sdk/docs/architecture.md",
}

const sdkAuditLastModifiedDate = "2026-05-26"

func TestSDKCompanionDocs_PageOneLastModifiedDate(t *testing.T) {
	needle := "*Last modified: " + sdkAuditLastModifiedDate + "*"
	for _, path := range sdkAuditDocs {
		text := readAuditFile(t, path)
		lines := strings.Split(text, "\n")
		limit := min(12, len(lines))
		if !strings.Contains(strings.Join(lines[:limit], "\n"), needle) {
			t.Fatalf("%s does not declare %q on page 1", path, needle)
		}
	}
}

func TestSDKCompanionDocs_NoEmOrEnDash(t *testing.T) {
	for _, path := range sdkAuditDocs {
		text := readAuditFile(t, path)
		for lineNo, line := range strings.Split(text, "\n") {
			if strings.ContainsAny(line, "—–") {
				t.Fatalf("%s:%d contains forbidden dash character: %q", path, lineNo+1, line)
			}
		}
	}
}

func TestSDKCompanionDocs_NoOSManualMaterialOutsideArchitecture(t *testing.T) {
	forbidden := []string{
		"IntuitionOS",
		"Intuition OS",
		"Intuition-OS",
		"IExec",
		"`iexec`",
		"GURU MEDITATION",
		"fault printer",
		"fault printers",
	}
	for _, path := range sdkAuditDocs {
		if path == "sdk/docs/architecture.md" {
			continue
		}
		text := readAuditFile(t, path)
		for _, needle := range forbidden {
			if strings.Contains(text, needle) {
				t.Fatalf("%s mentions OS-owned material that is outside the five-book SDK scope: %s", path, needle)
			}
		}
	}

	architecture := readAuditFile(t, "sdk/docs/architecture.md")
	for _, forbiddenNeedle := range []string{
		"GURU MEDITATION",
		"fault printer",
		"fault printers",
		"`iexec`",
	} {
		if strings.Contains(architecture, forbiddenNeedle) {
			t.Fatalf("architecture.md contains OS-manual wording rather than whole-machine architecture material: %s", forbiddenNeedle)
		}
	}
}

func TestSDKCompanionDocs_IEMonIsHardwareMonitorNotOSManual(t *testing.T) {
	doc := readAuditFile(t, "sdk/docs/iemon.md")
	forbidden := []string{
		"## IE64 Fault Reports",
		"operating-system",
		"kernel",
		"guest handler",
		"guest supervisor",
		"AROS",
		"EmuTOS",
		"GURU MEDITATION",
		"fault printer",
	}
	for _, needle := range forbidden {
		if strings.Contains(doc, needle) {
			t.Fatalf("iemon.md contains OS-scope material in the hardware monitor manual: %s", needle)
		}
	}
	for _, required := range []string{
		"built-in hardware-level debugger",
		"## IE64 Fault Interception",
		"`fault` lets IEMon break when a CPU fault or exception is detected",
	} {
		if !strings.Contains(doc, required) {
			t.Fatalf("iemon.md is missing hardware-monitor wording: %s", required)
		}
	}
}

func TestSDKCompanionDocs_BritishEnglishProse(t *testing.T) {
	americanisms := map[string]string{
		"artifact":       "artefact",
		"artifacts":      "artefacts",
		"behavior":       "behaviour",
		"behaviors":      "behaviours",
		"center":         "centre",
		"centered":       "centred",
		"color":          "colour",
		"colored":        "coloured",
		"colors":         "colours",
		"customize":      "customise",
		"customized":     "customised",
		"finalize":       "finalise",
		"finalized":      "finalised",
		"gray":           "grey",
		"initialization": "initialisation",
		"initialize":     "initialise",
		"initialized":    "initialised",
		"license":        "licence",
		"licensed":       "licenced",
		"neighbor":       "neighbour",
		"organize":       "organise",
		"organized":      "organised",
	}

	for _, path := range sdkAuditDocs {
		text := readAuditFile(t, path)
		inFence := false
		for lineNo, line := range strings.Split(text, "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "```") {
				inFence = !inFence
				continue
			}
			if inFence {
				continue
			}
			prose := stripMarkdownInlineCode(line)
			for us, uk := range americanisms {
				re := regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(us) + `\b`)
				if re.MatchString(prose) {
					t.Fatalf("%s:%d uses American-English prose spelling %q; use %q unless this is source-owned syntax", path, lineNo+1, us, uk)
				}
			}
		}
	}
}

func TestSDKCompanionDocs_FitForPurposeReferenceScaffold(t *testing.T) {
	requiredHeadings := map[string][]string{
		"sdk/docs/IE64_ISA.md": {
			"## 1. Architecture Overview",
			"## 2. Register File",
			"## 3. Instruction Encoding",
			"## 4. Complete Instruction Reference",
			"### 4.0 Instruction Entry Schema",
			"## 8. Address Space and Reset State",
			"## 11. Memory Management Unit",
			"## Appendix A: Opcode Map",
			"## Appendix B: Encoding Examples",
		},
		"sdk/docs/IE32_ISA.md": {
			"## 1. Architecture Overview",
			"## 2. Register File",
			"## 3. Instruction Encoding",
			"## 4. Complete Instruction Reference",
			"### 4.0 Instruction Entry Schema",
			"## 7. Address Space and Reset Vectors",
			"## 9. Timer and Interrupt Model",
			"## 10. Architectural Caveats",
			"## Appendix A: Opcode Map",
			"## Appendix B: Encoding Examples",
		},
		"sdk/docs/iemon.md": {
			"## Quick Start",
			"## Address Formats",
			"### Argument Parsing Matrix",
			"## Conditional Breakpoints",
			"## Command Reference",
			"### Command Surface",
			"## CPU-Specific Notes",
			"## Multi-CPU Debugging Workflows",
			"## IE64 Fault Interception",
			"## Common Pitfalls",
		},
		"sdk/docs/iescript.md": {
			"## Script Runtime Model",
			"## Safety and Concurrency Rules",
			"## Module Reference",
			"### Module Reference Conventions",
			"## `sys`",
			"## `cpu`",
			"## `mem`",
			"## `audio`",
			"## `video`",
			"## `dbg`",
			"## Worked Examples",
			"## Troubleshooting",
			"## Quick Reference",
		},
		"sdk/docs/architecture.md": {
			"## Reading the Architecture Tables and Diagrams",
			"## Single Complete Architecture Diagram",
			"## 1. Whole-System Architecture",
			"### Subsystem Matrix",
			"## Platform JIT Matrix",
			"## 3. CPU Subsystem",
			"## 4. Video Subsystem",
			"## 5. Audio Subsystem",
			"## 6. Memory Map",
			"## 8. Data Flow",
			"## 9. Concurrency Model and System Timing",
			"## Appendix: Key Source Files",
		},
	}
	for path, headings := range requiredHeadings {
		doc := readAuditFile(t, path)
		for _, heading := range headings {
			if !strings.Contains(doc, heading) {
				t.Fatalf("%s is missing fit-for-purpose reference scaffold heading %q", path, heading)
			}
		}
	}
}

func TestSDKDocAuditLedger_InitialReviewAndPDFHardGates(t *testing.T) {
	ledger := readAuditFile(t, "sdk/docs/verify/SDK_DOC_AUDIT_LEDGER.md")
	for _, needle := range []string{
		"## Control Plan",
		"Chronological audit entries below are evidence",
		"authority. Later evidence entries may supersede earlier evidence",
		"## Audit Evidence Log",
		"The entries below are chronological evidence for completed audit work.",
		"Source comments, prose documents,",
		"generated PDFs, and audit inventories are not canonical.",
		"If a source comment contradicts executable behaviour in a source-routed",
		"area discovered by the audit, the comment must be fixed",
		"### Five Manual Contracts",
		"`IE64_ISA.md`: IE64 physical processor user's manual.",
		"`IE32_ISA.md`: IE32 physical processor user's manual.",
		"`iemon.md`: machine monitor reference.",
		"`iescript.md`: IE Script reference.",
		"`architecture.md`: whole-machine Intuition Engine architecture manual.",
		"stable public architecture and observable subsystem",
		"must not document every private",
		"become an IntuitionOS syscall or kernel manual",
		"### Empirical Inventories",
		"SDK_IEMON_SOURCE_AUDIT.md",
		"SDK_IESCRIPT_SOURCE_AUDIT.md",
		"SDK_ARCH_SOURCE_AUDIT.md",
		"Inventory fact rows must be generated or mechanically checked",
		"Manual edits to factual rows are forbidden",
		"Inventories must not contain",
		"statuses, TODOs, inferred claims, review prose, or future-work notes",
		"For IEMon claims, mechanically derive command names, aliases, syntax",
		"For IE Script claims, mechanically derive module and function names",
		"For architecture claims, perform source discovery before judging",
		"Architecture inventory rows are classified as:",
		"public/stable architecture surface",
		"observable implementation boundary",
		"private implementation detail",
		"test-only or support-only code",
		"out-of-scope OS-owned material",
		"Positive gates compare each manual against its empirical inventory.",
		"Accurate but shallow",
		"material is a defect.",
		"### TDD and Code Bugs",
		"add a failing behavioural test or focused runnable",
		"fix every implementation path that owns the behaviour",
		"The intended public contract is determined in this order:",
		"assess compatibility before",
		"Code bugs in public behaviour covered by the",
		"five manuals are in scope and must be fixed with full TDD.",
		"full `go test -tags headless ./...` suite is not a completion gate",
		"### Final PDF Render Manifest",
		"records the exact render command",
		"Any later source, source-comment, inventory, test, ledger, manual, or",
		"The review evidence must be chronological, not retrospective.",
		"display the five review finding inventories",
		"A successful final response must also display the per-book",
		"cannot be closed only by writing a ledger entry",
		"PDF rendering is the final delivery action",
		"IE ISA Manual Hard Gate",
		"CPU ISA material only",
		"exact, source-backed `Instruction Fields` text",
		"Generic field prose is an audit defect",
		"IE32 non-branch instructions must not mention branch targets",
		"the PDFs from",
		"user-facing engineering references for",
		"experienced software engineers",
		"agent instructions, audit process notes, ledger status",
	} {
		if !strings.Contains(ledger, needle) {
			t.Fatalf("SDK doc audit ledger is missing hard-gate text %q", needle)
		}
	}

	reviewEntries := []struct {
		id       string
		nextID   string
		document string
	}{
		{"SDK-DOC-0036", "SDK-DOC-0037", "IE64"},
		{"SDK-DOC-0037", "SDK-DOC-0038", "IE32"},
		{"SDK-DOC-0038", "SDK-DOC-0039", "IEMon"},
		{"SDK-DOC-0039", "SDK-DOC-0040", "IEScript"},
		{"SDK-DOC-0040", "SDK-DOC-0030", "architecture"},
	}
	for _, entry := range reviewEntries {
		section := markdownSection(t, ledger, "ID: "+entry.id, "ID: "+entry.nextID)
		for _, needle := range []string{
			"Review display:",
			"Finding closures:",
			`Challenge answer to "Do you disagree with this review?": no.`,
		} {
			if !strings.Contains(section, needle) {
				t.Fatalf("%s initial review entry for %s is missing %q", entry.id, entry.document, needle)
			}
		}
	}
}

func TestSDKCompanionDocs_NoAuditProcessLanguage(t *testing.T) {
	for _, path := range sdkAuditDocs {
		text := readAuditFile(t, path)
		for lineNo, line := range strings.Split(text, "\n") {
			lower := strings.ToLower(line)
			for _, forbidden := range []string{
				"audit process",
				"ledger",
				"run output",
				"hard gate",
				"review finding",
				"review display",
				"finding closures",
				"source-backed",
				"source-checked",
				"canonical source",
				"technical claim",
				"roadmap item",
				"agent-facing",
				"process language",
			} {
				if strings.Contains(lower, forbidden) {
					t.Fatalf("%s:%d contains audit/process language %q: %s", path, lineNo+1, forbidden, line)
				}
			}
		}
	}
}

func TestSDKCompanionDocs_UserManualsDoNotExposeAuditOrSourceFileInternals(t *testing.T) {
	for _, tc := range []struct {
		path      string
		forbidden []string
		required  []string
	}{
		{
			path: "sdk/docs/iemon.md",
			forbidden: []string{
				"### Source-Checked Command Surface",
				"`debug_commands.go`",
				"canonical source is",
			},
			required: []string{
				"### Command Surface",
				"The command surface below reflects the monitor command registry plus\n" +
					"dispatch-level aliases.",
				"reference context, see `IE64_ISA.md` section 11.8.",
				"| 0     | `page-not-present` | Absent PTE mapping or unavailable physical/atomic backing |",
			},
		},
		{
			path: "sdk/docs/iescript.md",
			forbidden: []string{
				"`script_engine.go`",
				"`psg_player.go`",
				"See `psg_player.go` for the authoritative list",
			},
			required: []string{
				"This manual documents the IE Script Lua API exposed to scripts.",
				"Supported extensions: `.vgm`, `.vgz`, `.vtx`, `.vt`, `.ym`, `.ay`, `.snd`, `.sndh`, `.pt3`, `.pt2`, `.pt1`, `.stc`, `.sqt`, `.asc`, `.ftc`. Returns: nothing. Raises on error.",
			},
		},
	} {
		doc := readAuditFile(t, tc.path)
		for _, forbidden := range tc.forbidden {
			if strings.Contains(doc, forbidden) {
				t.Fatalf("%s exposes audit/source internals: %s", tc.path, forbidden)
			}
		}
		for _, required := range tc.required {
			if !strings.Contains(doc, required) {
				t.Fatalf("%s missing user-facing replacement text: %s", tc.path, required)
			}
		}
	}
}

func TestSDKCompanionDocs_NoSourceCompatibilityNotes(t *testing.T) {
	for _, path := range sdkAuditDocs {
		text := readAuditFile(t, path)
		for _, forbidden := range []string{
			"## Source Compatibility Note",
			"source tree is the canonical definition",
		} {
			if strings.Contains(text, forbidden) {
				t.Fatalf("%s contains agent-facing source compatibility prose: %q", path, forbidden)
			}
		}
	}
}

func TestSDKCompanionDocs_NoUnresolvedPlanningPlaceholders(t *testing.T) {
	for _, path := range sdkAuditDocs {
		text := readAuditFile(t, path)
		for lineNo, line := range strings.Split(text, "\n") {
			lower := strings.ToLower(line)
			for _, forbidden := range []string{
				"todo",
				"fixme",
				"tbd",
				"wip",
				"future work",
				"coming soon",
				"placeholder",
			} {
				if strings.Contains(lower, forbidden) {
					t.Fatalf("%s:%d contains unresolved planning placeholder %q: %s", path, lineNo+1, forbidden, line)
				}
			}
		}
	}
}

func TestSDKCompanionDocs_IE64OpcodeTableMatchesSource(t *testing.T) {
	source := readAuditFile(t, "cpu_ie64.go")
	doc := readAuditFile(t, "sdk/docs/IE64_ISA.md")
	sourceOps := parseIE64SourceOpcodes(t, source)
	docOps := parseMarkdownOpcodeRows(t, doc)

	for _, op := range sourceOps {
		mnemonic := ie64DocMnemonic(op.name)
		if got, ok := docOps[mnemonic][op.value]; !ok {
			t.Fatalf("IE64 opcode %s %s=0x%02X missing from SDK table", op.name, mnemonic, op.value)
		} else if got != mnemonic {
			t.Fatalf("IE64 opcode 0x%02X mnemonic = %s, want %s", op.value, got, mnemonic)
		}
	}
}

func TestSDKCompanionDocs_IE64CompleteReferenceCoversSourceOpcodesWithoutNumberGaps(t *testing.T) {
	source := readAuditFile(t, "cpu_ie64.go")
	doc := readAuditFile(t, "sdk/docs/IE64_ISA.md")
	section := markdownSection(t, doc, "## 4. Complete Instruction Reference", "## 5. Architectural Instruction Idioms")

	headingRe := regexp.MustCompile(`(?m)^#### ([0-9]+)\. ([A-Z0-9]+) `)
	matches := headingRe.FindAllStringSubmatch(section, -1)
	if len(matches) == 0 {
		t.Fatal("IE64 complete instruction reference has no numbered instruction entries")
	}
	for i, match := range matches {
		got, _ := strconv.Atoi(match[1])
		want := i + 1
		if got != want {
			t.Fatalf("IE64 instruction entry numbering jumps at %s: got %d, want %d", match[2], got, want)
		}
	}

	for _, op := range parseIE64SourceOpcodes(t, source) {
		mnemonic := ie64DocMnemonic(op.name)
		if !regexp.MustCompile(`(?m)^#### [0-9]+\. ` + regexp.QuoteMeta(mnemonic) + `\b`).MatchString(section) {
			t.Fatalf("IE64 complete instruction reference missing source opcode entry %s (%s, 0x%02X)", op.name, mnemonic, op.value)
		}
	}
	for _, forbidden := range []string{
		"#### 11.6.1 MTCR",
		"#### 11.13.1 CAS",
		"software TLB",
	} {
		if strings.Contains(doc, forbidden) {
			t.Fatalf("IE64_ISA.md still carries out-of-reference or implementation wording: %s", forbidden)
		}
	}
}

func TestSDKCompanionDocs_IE64FPUSectionCoversSourceOpcodes(t *testing.T) {
	source := readAuditFile(t, "cpu_ie64.go")
	doc := readAuditFile(t, "sdk/docs/IE64_ISA.md")
	section := markdownSection(t, doc, "### 4.6 Floating Point (FPU)", "### 4.7 Branches")

	for _, op := range parseIE64SourceOpcodes(t, source) {
		if op.value < 0x60 || op.value > 0x90 {
			continue
		}
		mnemonic := ie64DocMnemonic(op.name)
		if !strings.Contains(section, "| "+mnemonic+" ") && !strings.Contains(section, "`"+strings.ToLower(mnemonic)+" ") {
			t.Fatalf("IE64 FPU section does not cover source opcode %s (%s, 0x%02X)", op.name, mnemonic, op.value)
		}
	}
}

func TestSDKCompanionDocs_IE64FPUEntriesUseConcreteAttributesAndInvalidEncoding(t *testing.T) {
	source := readAuditFile(t, "cpu_ie64.go")
	doc := readAuditFile(t, "sdk/docs/IE64_ISA.md")
	section := markdownSection(t, doc, "### 4.6 Floating Point (FPU)", "### 4.7 Branches")
	for _, needle := range []string{
		"if rd > 15 || rs > 15",
		"if !isValidDPairReg(rd) || !isValidDPairReg(rs)",
		"goto invalid_freg",
		"cpu.running.Store(false)",
	} {
		if !strings.Contains(source, needle) {
			t.Fatalf("cpu_ie64.go FPU invalid-register source changed; review IE64 FPU docs: %s", needle)
		}
	}
	for _, forbidden := range []string{
		"Operand and memory attributes are defined by this entry's syntax",
		"**Exceptions:** None.\n\n**Notes:** None.\n\n#### 52. FADD",
		"**Exceptions:** None.\n\n**Notes:** None.\n\n#### 80. DADD",
	} {
		if strings.Contains(section, forbidden) {
			t.Fatalf("IE64 FPU section still contains placeholder FPU reference text: %s", forbidden)
		}
	}
	for _, tc := range []struct {
		heading string
		next    string
		want    []string
	}{
		{
			"#### 52. FADD - fadd fd, fs, ft",
			"#### 53. FSUB - fsub fd, fs, ft",
			[]string{
				"FP operands: `fd`, `fs`, and `ft` encode `f0`-`f15`.",
				"An invalid `fd`, `fs`, or `ft` encoding enters the stopped processor state with PC unchanged.",
			},
		},
		{
			"#### 80. DADD - dadd fd, fs, ft",
			"#### 81. DSUB - dsub fd, fs, ft",
			[]string{
				"FP operands: `fd`, `fs`, and `ft` must encode even registers from `f0` through `f14`.",
				"An invalid or odd `fd`, `fs`, or `ft` encoding enters the stopped processor state with PC unchanged.",
			},
		},
		{
			"#### 73. FMOVSR - fmovsr rd",
			"#### 74. FMOVCR - fmovcr rd",
			[]string{
				"Operand size: 32-bit status word.",
				"FPSR/FPCR: reads FPSR; FPCR is not read or written.",
				"**Exceptions:** None.",
			},
		},
	} {
		entry := markdownSection(t, section, tc.heading, tc.next)
		for _, want := range tc.want {
			if !strings.Contains(entry, want) {
				t.Fatalf("%s missing concrete FPU attribute/exception text: %s", tc.heading, want)
			}
		}
	}
}

func TestSDKCompanionDocs_IE64FPUCompareAndConstantSemanticsMatchSource(t *testing.T) {
	source := readAuditFile(t, "fpu_ie64.go")
	doc := readAuditFile(t, "sdk/docs/IE64_ISA.md")
	fpuSection := markdownSection(t, doc, "### 4.6 Floating Point (FPU)", "### 4.7 Branches")
	for _, needle := range []string{
		"math.Float32bits(math.SmallestNonzeroFloat32)",
		"return 0",
		"IE64_FPU_CC_NAN",
		"IE64_FPU_EX_IO",
	} {
		if !strings.Contains(source, needle) {
			t.Fatalf("fpu_ie64.go source changed; review IE64 FPU docs: %s", needle)
		}
	}
	for _, needle := range []string{
		"Smallest positive FP32 subnormal (`0x00000001`, approximately `1.40129846e-45`)",
		"If either operand is NaN, the comparison is unordered: `rd` receives `0`",
		"the NaN condition code is set, and the IO sticky exception flag is set",
	} {
		if !strings.Contains(fpuSection, needle) {
			t.Fatalf("IE64_ISA.md FPU section missing source-backed compare/constant wording: %s", needle)
		}
	}
	if strings.Contains(fpuSection, "FLT_MIN") {
		t.Fatal("IE64_ISA.md still labels FMOVECR #14 with ambiguous FLT_MIN wording")
	}
}

func TestSDKCompanionDocs_IE64TLBINVALDocumentsVAOperand(t *testing.T) {
	source := readAuditFile(t, "cpu_ie64.go")
	doc := readAuditFile(t, "sdk/docs/IE64_ISA.md")
	if !strings.Contains(source, "vpn := cpu.regs[rs] >> MMU_PAGE_SHIFT") {
		t.Fatal("source no longer derives TLBINVAL VPN from the virtual address in Rs")
	}
	if strings.Contains(doc, "TLBINVAL: [0xEA] [0] [Rs<<3] [0] [0 0 0 0]                     ; Rs = register holding VPN") {
		t.Fatal("IE64_ISA.md incorrectly documents TLBINVAL Rs as holding a VPN; source takes a VA and shifts it")
	}
	entry := markdownSection(t, doc, "#### 119. TLBINVAL - tlbinval Rs", "#### 120. SYSCALL - syscall #imm32")
	if !strings.Contains(entry, "The processor treats `Rs` as a virtual address") ||
		!strings.Contains(entry, "shifting it right by 12") {
		t.Fatal("IE64_ISA.md does not document TLBINVAL Rs as holding a virtual address")
	}
}

func TestSDKCompanionDocs_ISADocsStayCPUScope(t *testing.T) {
	forbidden := []string{
		"## 8. Memory Map",
		"## 7. Memory Map",
		"Platform bus routing",
		"Bus Method",
		"Read8/Write8",
		"branch decisions use the encoded comparison or transfer target",
		"absolute subroutine-call form uses opcode `0x50`",
		"Appendix C: IntuitionOS IE64 ABI v0",
		"IntuitionOS ABI",
		"SYSINFO",
		"MMIO",
		"MapIO",
		"MMIO64",
		"Host helper",
		"HOST_CMD",
		"AROS DOS",
		"AROS audio",
		"Media loader",
		"VGA",
		"Voodoo",
		"ULA",
		"ANTIC",
		"GTIA",
		"SID",
		"POKEY",
		"TED",
		"AHX",
		"audio-chip I/O base",
		"Video chip",
		"video registers",
		"VRAM",
		"IO_BASE",
		"`0xF0800`",
		"`0xF0804`",
		"`0xF0808`",
		"`$F0000`",
		"`$F0800`",
		"`$F1000`",
		"`$F1400`",
		"`$FA000`",
	}
	for _, path := range []string{"sdk/docs/IE64_ISA.md", "sdk/docs/IE32_ISA.md"} {
		doc := readAuditFile(t, path)
		for _, needle := range forbidden {
			if strings.Contains(doc, needle) {
				t.Fatalf("%s documents platform/hardware material in an ISA manual: %s", path, needle)
			}
		}
	}
}

func TestSDKCompanionDocs_IE64TOCAppendixAnchor(t *testing.T) {
	doc := readAuditFile(t, "sdk/docs/IE64_ISA.md")
	if strings.Contains(doc, "#appendix-a-opcode-summary") {
		t.Fatal("IE64_ISA.md still links to the removed appendix-a-opcode-summary anchor")
	}
	for _, needle := range []string{
		"8. [Address Space and Reset State](#8-address-space-and-reset-state)",
		"12. [Appendix A: Opcode Map](#appendix-a-opcode-map)",
		"13. [Appendix B: Encoding Examples](#appendix-b-encoding-examples)",
		"## Appendix A: Opcode Map",
		"## Appendix B: Encoding Examples",
	} {
		if !strings.Contains(doc, needle) {
			t.Fatalf("IE64_ISA.md TOC or appendix list is missing entry: %s", needle)
		}
	}
	for _, forbidden := range []string{
		"15. [Appendix C: IntuitionOS IE64 ABI v0](#appendix-c-intuitionos-ie64-abi-v0)",
		"## Appendix C: IntuitionOS IE64 ABI v0",
		"**IntuitionOS ABI**: The discoverable IE64 ABI v0 contract is documented in",
	} {
		if strings.Contains(doc, forbidden) {
			t.Fatalf("IE64_ISA.md still includes non-ISA ABI material: %s", forbidden)
		}
	}
}

func TestSDKCompanionDocs_IE32StoreNotationIsConsistent(t *testing.T) {
	doc := readAuditFile(t, "sdk/docs/IE32_ISA.md")
	source := readAuditFile(t, "cpu_ie32.go")
	for _, needle := range []string{
		"func (cpu *CPU) storeRegister(value uint32, addrMode byte, operand uint32)",
		"if addrMode == ADDR_REG_IND",
		"} else if addrMode == ADDR_MEM_IND {",
		"cpu.Write32(operand, value)",
	} {
		if !strings.Contains(source, needle) {
			t.Fatalf("cpu_ie32.go store destination source changed; review IE32 store docs: %s", needle)
		}
	}
	if strings.Contains(doc, "store(operand, value)") {
		t.Fatal("IE32_ISA.md defines store arguments in the opposite order from the opcode tables")
	}
	if !strings.Contains(doc, "`store(value, operand)` writes `value`") {
		t.Fatal("IE32_ISA.md does not define store(value, operand) notation")
	}
	if strings.Contains(doc, "destination register or 32-bit memory location") {
		t.Fatal("IE32_ISA.md still implies STORE can write an architectural register")
	}
	normalizedDoc := strings.Join(strings.Fields(doc), " ")
	if !strings.Contains(normalizedDoc, "Store instructions do not write architectural registers.") {
		t.Fatal("IE32_ISA.md does not split read operand resolution from store destination resolution")
	}
	section := markdownSection(t, doc, "#### 18. STORE - STORE R, operand", "#### 19. STA - STA operand")
	if !strings.Contains(section, "**Operation:** `store(R, operand)`.") {
		t.Fatal("IE32_ISA.md STORE entry no longer matches store(value, operand) notation")
	}
	if !strings.Contains(section, "Immediate, direct, and register addressing modes use `operand32` as the destination address") {
		t.Fatal("IE32 STORE entry does not document source-backed store destination modes")
	}
}

func TestSDKCompanionDocs_IE32ReservedAddressingModesMatchSource(t *testing.T) {
	doc := readAuditFile(t, "sdk/docs/IE32_ISA.md")
	source := readAuditFile(t, "cpu_ie32.go")
	for _, needle := range []string{
		"case ADDR_DIRECT:",
		"return cpu.Read32(operand)",
		"return 0",
		"cpu.Write32(operand, value)",
	} {
		if !strings.Contains(source, needle) {
			t.Fatalf("cpu_ie32.go addressing-mode source changed; review reserved-mode docs: %s", needle)
		}
	}
	section := markdownSection(t, doc, "### 3.5 Addressing Mode Codes", "---\n\n## 4. Complete Instruction Reference")
	normalizedSection := strings.Join(strings.Fields(section), " ")
	for _, needle := range []string{
		"| `0x05`-`0xFF` | Reserved | Read-style operand resolution returns zero. Store-style resolution treats `operand32` as a direct destination memory address. |",
	} {
		if !strings.Contains(section, needle) {
			t.Fatalf("IE32_ISA.md missing source-backed reserved addressing-mode behavior: %s", needle)
		}
	}
	if !strings.Contains(normalizedSection, "Addressing-mode bytes `0x05` through `0xFF` are reserved encodings with the deterministic behaviour shown in the table.") {
		t.Fatal("IE32_ISA.md missing reserved addressing-mode prose for 0x05-0xFF")
	}
}

func TestSDKCompanionDocs_IE32LogicalORTableEscapesMarkdownPipe(t *testing.T) {
	doc := readAuditFile(t, "sdk/docs/IE32_ISA.md")
	section := markdownSection(t, doc, "### 4.4 Logical", "### 4.5 Shifts")
	if strings.Contains(section, "| OR | `0x09` | `OR R, operand` | `R = R | resolve(operand)` | Depends on operand |") {
		t.Fatal("IE32_ISA.md OR row contains an unescaped table pipe in the operation cell")
	}
	entry := markdownSection(t, section, "#### 43. OR - OR R, operand", "#### 44. XOR - XOR R, operand")
	if !strings.Contains(entry, "**Operation:** `R = R \\| resolve(operand)`.") {
		t.Fatal("IE32_ISA.md OR entry does not escape the operation pipe")
	}
}

func TestSDKCompanionDocs_ArchitectureTimerCadenceMatchesSource(t *testing.T) {
	arch := readAuditFile(t, "sdk/docs/architecture.md")
	normalizedArch := strings.Join(strings.Fields(arch), " ")
	ie64Source := readAuditFile(t, "cpu_ie64.go")
	ie64JITSource := readAuditFile(t, "jit_exec.go")
	ie32Source := readAuditFile(t, "cpu_ie32.go")
	if !strings.Contains(ie64Source, "newCount := count - 1") {
		t.Fatal("cpu_ie64.go no longer shows interpreter timer decrement per instruction")
	}
	if strings.Contains(ie64JITSource, "cpu.timerCount.Store(count - executed)") ||
		strings.Contains(ie64JITSource, "cpu.handleTimerJIT()") {
		t.Fatal("jit_exec.go still contains block-boundary IE64 timer countdown")
	}
	if !strings.Contains(ie64JITSource, "if cpu.timerEnabled.Load()") ||
		!strings.Contains(ie64JITSource, "executed := cpu.interpretOne()") {
		t.Fatal("jit_exec.go no longer routes armed IE64 timer execution through single-instruction stepping")
	}
	if !strings.Contains(ie32Source, "cpu.cycleCounter >= SAMPLE_RATE") {
		t.Fatal("cpu_ie32.go no longer shows IE32 SAMPLE_RATE-gated timer cadence")
	}
	if strings.Contains(arch, "The timer decrements once per `SAMPLE_RATE` instructions.") {
		t.Fatal("architecture.md still incorrectly applies SAMPLE_RATE timer cadence to both IE32 and IE64")
	}
	for _, needle := range []string{
		"IE32 increments an internal cycle counter once per decoded instruction step,",
		"after fetch and operand resolution and before the decoded instruction body",
		"IE64 decrements the timer count once per decoded instruction step, after",
		"fetch/decode and before the decoded instruction body executes.",
		"When the IE64 timer is armed, JIT dispatch uses the CPU single-instruction path",
		"interrupt can be delivered before the body of the instruction whose decoded",
		"step expires the timer.",
	} {
		if !strings.Contains(normalizedArch, needle) {
			t.Fatalf("architecture.md missing timer cadence detail: %s", needle)
		}
	}
	for _, forbidden := range []string{
		"IE32 increments an internal cycle counter once per executed instruction",
		"IE64 decrements the timer count in retired IE64 instruction units",
		"number of instructions retired by the native block",
		"subtracting the number of decoded",
	} {
		if strings.Contains(arch, forbidden) {
			t.Fatalf("architecture.md uses stale timer cadence wording: %s", forbidden)
		}
	}
}

func TestSDKCompanionDocs_ArchitectureCPUBridgeTablesCoverSourceRoutes(t *testing.T) {
	arch := readAuditFile(t, "sdk/docs/architecture.md")
	sourceInventory := readAuditFile(t, "sdk/docs/verify/SDK_ARCH_SOURCE_AUDIT.md")
	z80Source := readAuditFile(t, "cpu_z80_runner.go")
	x86Source := readAuditFile(t, "cpu_x86_runner.go")
	six5Source := readAuditFile(t, "cpu_six5go2.go")

	for _, pair := range []struct {
		source string
		needle string
	}{
		{z80Source, "case Z80_SN_PORT_DATA:"},
		{z80Source, "case Z80_ULA_PORT_ADDR_LO:"},
		{z80Source, "case Z80_ULA_PORT_DATA:"},
		{x86Source, "b.tedRegSelect >= X86_TED_V_INDEX_BASE && b.tedRegSelect <= X86_TED_V_INDEX_END"},
		{six5Source, "addr >= C6502_TED_V_BASE && addr <= C6502_TED_V_END"},
	} {
		if !strings.Contains(pair.source, pair.needle) {
			t.Fatalf("source no longer exposes bridge route %q", pair.needle)
		}
	}

	requiredBridgeRows := []struct {
		manualRow string
		factName  string
	}{
		{
			manualRow: "| `$E4/$E5` | SN76489 | `0xF0C30/0xF0C31` | Data write / last-written read and ready-status read |",
			factName:  "$E4/$E5 | SN76489 | 0xF0C30/0xF0C31 | Data write / last-written read and ready-status read",
		},
		{
			manualRow: "| `$F2/$F3` | TED | `0xF0F00` / `0xF0F20-0xF0F6B` | Register select / data (audio indices `$00-$05`, video indices `$20-$32` x4 stride) |",
			factName:  "$F2/$F3 | TED | 0xF0F00 / 0xF0F20-0xF0F6B | Register select / data (audio indices $00-$05, video indices $20-$32 x4 stride)",
		},
		{
			manualRow: "| `$FE/$FD/$BE/$FA/$FB/$FC` | ULA | `0xF2000-0xF2014` | Border, control, status, VRAM address latch low/high, and paged VRAM data |",
			factName:  "$FE/$FD/$BE/$FA/$FB/$FC | ULA | 0xF2000-0xF2014 | Border, control, status, VRAM address latch low/high, and paged VRAM data",
		},
		{
			manualRow: "| `$D620-$D632` | TED Video | `0xF0F20+offset x4` | Stride-4 register mapping including raster compare registers |",
			factName:  "$D620-$D632 | TED Video | 0xF0F20+offset x4 | Stride-4 register mapping including raster compare registers",
		},
	}
	for _, row := range requiredBridgeRows {
		if !strings.Contains(arch, row.manualRow) {
			t.Fatalf("architecture.md missing source-backed CPU bridge row: %s", row.manualRow)
		}
		if !normalizedContains(sourceInventory, row.factName) {
			t.Fatalf("SDK_ARCH_SOURCE_AUDIT.md missing source-backed CPU bridge row: %s", row.factName)
		}
	}

	for _, stale := range []string{
		"| `$D620-$D62F` | TED Video |",
		"| `$FE` | ULA | `0xF2000` | Border colour (bits 0-2). Bits 3-4 currently ignored. |",
		"video indices `$20-$2F`",
	} {
		if strings.Contains(arch, stale) {
			t.Fatalf("architecture.md still contains stale CPU bridge wording: %s", stale)
		}
	}
}

func TestSDKCompanionDocs_ArchitectureIRQDiagnosticsLifecycleMatchesSource(t *testing.T) {
	arch := readAuditFile(t, "sdk/docs/architecture.md")
	sourceInventory := readAuditFile(t, "sdk/docs/verify/SDK_ARCH_SOURCE_AUDIT.md")
	arosLoader := readAuditFile(t, "aros_loader.go")
	mainSource := readAuditFile(t, "main.go")
	arosDMA := readAuditFile(t, "aros_audio_dma.go")

	for _, needle := range []string{
		"func (l *AROSLoader) MapIRQDiagnostics()",
		"l.bus.MapIO(IRQ_DIAG_REGION_BASE, IRQ_DIAG_REGION_END",
	} {
		if !strings.Contains(arosLoader, needle) {
			t.Fatalf("aros_loader.go IRQ diagnostic mapping source changed: %s", needle)
		}
	}
	if strings.Count(mainSource, "loader.MapIRQDiagnostics()") < 2 {
		t.Fatal("main.go no longer maps IRQ diagnostics through the AROS loader paths")
	}
	if !strings.Contains(arosDMA, "sysBus.UnmapIO(IRQ_DIAG_REGION_BASE, IRQ_DIAG_REGION_END)") {
		t.Fatal("aros_audio_dma.go no longer unmaps IRQ diagnostics during AROS DMA teardown")
	}

	row := "| `0xF23C0-0xF23DF` | 32B | MMIO | AROS IRQ diagnostic registers | AROS M68K profile while AROS loader is active | AROS profile | Read-only diagnostic MMIO mapped by the AROS loader and unmapped during AROS DMA teardown; not a universal interrupt-controller ABI. |"
	if !strings.Contains(arch, row) {
		t.Fatalf("architecture.md missing AROS-scoped IRQ diagnostics row: %s", row)
	}
	for _, stale := range []string{
		"| `0xF23C0-0xF23DF` | 32B | MMIO | IRQ diagnostic registers | All CPU cores | IRQ diagnostics |",
		"Diagnostic MMIO, not a separate interrupt controller ABI.",
	} {
		if strings.Contains(arch, stale) {
			t.Fatalf("architecture.md still implies universal IRQ diagnostics mapping: %s", stale)
		}
	}
	for _, evidence := range []string{
		"`aros_loader.go` `MapIRQDiagnostics`",
		"`main.go` AROS call sites",
		"`aros_audio_dma.go` `UnmapIO` teardown",
	} {
		if !strings.Contains(sourceInventory, evidence) {
			t.Fatalf("SDK_ARCH_SOURCE_AUDIT.md IRQ diagnostics row missing mapping lifecycle evidence: %s", evidence)
		}
	}
}

func TestSDKCompanionDocs_ArchitectureProgramExecutorOpsMatchSource(t *testing.T) {
	arch := readAuditFile(t, "sdk/docs/architecture.md")
	sourceInventory := readAuditFile(t, "sdk/docs/verify/SDK_ARCH_SOURCE_AUDIT.md")
	constantsSource := readAuditFile(t, "program_executor_constants.go")
	dispatchSource := readAuditFile(t, "program_executor.go")
	testSource := readAuditFile(t, "program_executor_test.go")

	for _, needle := range []string{
		"EXEC_OP_EXECUTE    = 1",
		"EXEC_OP_EMUTOS     = 2",
		"EXEC_OP_AROS       = 3",
		"EXEC_OP_IEXEC      = 4",
		"EXEC_OP_HARD_RESET = 5",
	} {
		if !strings.Contains(constantsSource, needle) {
			t.Fatalf("program executor constants no longer expose %q", needle)
		}
	}
	for _, needle := range []string{
		"if val == EXEC_OP_EXECUTE",
		"} else if val == EXEC_OP_EMUTOS",
		"} else if val == EXEC_OP_AROS",
		"} else if val == EXEC_OP_IEXEC",
		"} else if val == EXEC_OP_HARD_RESET",
		"e.startIExec()",
		"e.startHardReset()",
	} {
		if !strings.Contains(dispatchSource, needle) {
			t.Fatalf("program executor dispatch no longer exposes %q", needle)
		}
	}
	for _, needle := range []string{
		"EXEC_OP_IEXEC != 4",
		"EXEC_OP_HARD_RESET != 5",
	} {
		if !strings.Contains(testSource, needle) {
			t.Fatalf("program executor tests no longer pin %q", needle)
		}
	}

	expected := "EXEC_CTRL operation values: 1=Execute, 2=EmuTOS, 3=AROS, 4=IntuitionOS IExec, 5=Hard reset"
	if !strings.Contains(arch, expected) {
		t.Fatalf("architecture.md missing source-backed Program Executor operation values: %s", expected)
	}
	if strings.Contains(arch, "4=Reserved") {
		t.Fatal("architecture.md still falsely documents EXEC_OP 4 as reserved")
	}
	if !normalizedContains(sourceInventory, expected) {
		t.Fatalf("SDK_ARCH_SOURCE_AUDIT.md missing Program Executor operation values: %s", expected)
	}
}

func TestSDKCompanionDocs_ArchitectureRAMSizingNamesPlatformDispatch(t *testing.T) {
	arch := readAuditFile(t, "sdk/docs/architecture.md")
	linuxSource := readAuditFile(t, "memory_sizing_usable_linux.go")
	darwinSource := readAuditFile(t, "memory_sizing_usable_darwin.go")
	windowsSource := readAuditFile(t, "memory_sizing_usable_windows.go")
	for _, pair := range []struct {
		source string
		needle string
	}{
		{linuxSource, `os.ReadFile("/proc/meminfo")`},
		{darwinSource, `unix.SysctlUint64("hw.memsize")`},
		{darwinSource, `pageAlignDown(total / 2)`},
		{windowsSource, `kernel32.NewProc("GlobalMemoryStatusEx")`},
	} {
		if !strings.Contains(pair.source, pair.needle) {
			t.Fatalf("host usable-RAM source path changed: %s", pair.needle)
		}
	}
	parts := strings.SplitN(arch, "\n\n", 3)
	if len(parts) < 3 {
		t.Fatal("architecture.md missing expected page-one paragraph structure")
	}
	intro := parts[2]
	for _, needle := range []string{
		"platform-dispatched usable-RAM detection",
		"`/proc/meminfo` on Linux",
		"Darwin RAM sizing uses a page-aligned conservative half of `hw.memsize` as the detected base before applying the per-platform reserve",
		"`GlobalMemoryStatusEx` on Windows",
	} {
		if !strings.Contains(intro, needle) {
			t.Fatalf("architecture.md intro does not describe platform RAM sizing dispatch: %s", needle)
		}
	}
	if strings.Contains(intro, "`/proc/meminfo` on Linux, `hw.memsize` on Darwin, and `GlobalMemoryStatusEx` on Windows") {
		t.Fatal("architecture.md omits Darwin half-of-physical-RAM detected base before reserve")
	}
	if strings.Contains(intro, "autodetected at boot from host `/proc/meminfo`") {
		t.Fatal("architecture.md still presents Linux /proc RAM sizing as a global host contract")
	}
}

func TestSDKCompanionDocs_IE64FPCRReservedBitsAreNormative(t *testing.T) {
	source := readAuditFile(t, "fpu_ie64.go")
	doc := readAuditFile(t, "sdk/docs/IE64_ISA.md")
	for _, needle := range []string{
		"return uint8(fpu.FPCR & 0x03)",
		"fpu.FPCR = val",
	} {
		if !strings.Contains(source, needle) {
			t.Fatalf("fpu_ie64.go FPCR behavior changed; review IE64 FPCR prose: %s", needle)
		}
	}
	for _, needle := range []string{
		"FPU arithmetic interprets only bits 1:0 as the rounding mode",
		"bits 31:2 are preserved and have no defined effect",
	} {
		if !strings.Contains(doc, needle) {
			t.Fatalf("IE64_ISA.md missing normative FPCR reserved-bit wording: %s", needle)
		}
	}
	if strings.Contains(doc, "Current FPU operations") {
		t.Fatal("IE64_ISA.md still uses version-ish FPCR wording")
	}
}

func TestSDKCompanionDocs_ArchitectureM68KBareUsesActiveVisibleRAMCap(t *testing.T) {
	doc := readAuditFile(t, "sdk/docs/architecture.md")
	source := readAuditFile(t, "boot_guest_ram.go")
	for _, needle := range []string{
		"case modeIE32, modeX86, modeM68KBare:",
		"case modeEmuTOS:",
		"case modeAros:",
		"clampProfileCapToDetected",
	} {
		if !strings.Contains(source, needle) {
			t.Fatalf("boot_guest_ram.go M68K sizing path changed; review architecture.md CPU selection table: %s", needle)
		}
	}
	if got := strings.Count(source, "case modeIE32, modeX86, modeM68KBare:"); got != 2 {
		t.Fatalf("expected modeM68KBare to share both IE32/x86 cap paths, found %d cases", got)
	}
	section := markdownSection(t, doc, "### CPU Selection by File Extension", "### Z80 Port I/O Bridge")
	for _, required := range []string{
		"| `.ie68` | M68K | 32-bit flat (clamped to active visible RAM) |",
		"Bare `.ie68` uses the active-visible RAM ceiling; EmuTOS and AROS M68K loader modes use profile bounds.",
	} {
		if !normalizedContains(section, required) {
			t.Fatalf("architecture.md M68K file-extension contract omits source-backed behavior: %s", required)
		}
	}
	if strings.Contains(section, "clamped to M68K profile bound") {
		t.Fatal("architecture.md still applies profile-bound M68K RAM sizing to bare .ie68")
	}
}

func TestSDKCompanionDocs_ArchitectureX86BankWindowsMatchSource(t *testing.T) {
	arch := readAuditFile(t, "sdk/docs/architecture.md")
	source := readAuditFile(t, "cpu_x86_runner.go")
	for _, needle := range []string{
		"// x86 Bank Windows (same as Z80/6502 for compatibility)",
		"X86_BANK1_WINDOW_BASE = 0x2000",
		"X86_VRAM_BANK_WINDOW_BASE = 0x8000",
		"if translated < DEFAULT_MEMORY_SIZE",
	} {
		if !strings.Contains(source, needle) {
			t.Fatalf("cpu_x86_runner.go no longer exposes the expected x86 compatibility bank-window behaviour: %s", needle)
		}
	}
	section := markdownSection(t, arch, "### Bank Windows (Z80 / 6502 / x86)", "### 6502 I/O Chip Page Dispatch")
	for _, needle := range []string{
		"x86 ordinary addressing remains flat 32-bit",
		"the x86 bus adapter also implements the same compatibility bank windows plus a VRAM bank window",
		"Those x86 window translations are capped at the legacy `DEFAULT_MEMORY_SIZE` ceiling",
		"ordinary x86 addressing remains clamped by the active visible RAM",
	} {
		if !strings.Contains(section, needle) {
			t.Fatalf("architecture.md x86 bank-window section missing source-backed wording: %s", needle)
		}
	}
	if strings.Contains(section, "x86 is flat 32-bit and does not use this banked visibility cap") {
		t.Fatal("architecture.md still says x86 has no banked visibility cap")
	}
}

func TestSDKCompanionDocs_ArchitectureCoprocessorMemoryReservationsAreShared(t *testing.T) {
	arch := readAuditFile(t, "sdk/docs/architecture.md")
	section := markdownSection(t, arch, "### Coprocessor Worker Dispatch", "### Lua Scripting")
	for _, needle := range []string{
		"dedicated shared-memory reservation in the unified physical map",
		"not a private per-core address space",
	} {
		if !strings.Contains(section, needle) {
			t.Fatalf("architecture.md coprocessor section still implies private memory: %s", needle)
		}
	}
	if strings.Contains(section, "its own dedicated memory region") {
		t.Fatal("architecture.md still uses ambiguous dedicated memory region wording for coprocessors")
	}
}

func TestSDKCompanionDocs_ArchitectureMemoryMapExplainsSharedReservations(t *testing.T) {
	arch := readAuditFile(t, "sdk/docs/architecture.md")
	section := markdownSection(t, arch, "## 6. Memory Map", "## 7. I/O Peripherals")
	for _, needle := range []string{
		"All CPU cores observe the same guest physical address space.",
		"not per-core private maps unless explicitly stated",
		"the table describes reservations within the shared",
		"physical map, not separate mappings",
		"| Range | Size | Decoded as | Architectural use | Visible to | Owner / lifetime | Notes |",
		"| `0x100000-0x5FFFFF` | 5MB | Shared RAM / VRAM-backed region | Main video framebuffer and graphics-visible memory | All CPU cores | Video subsystem plus guest convention | Subranges may be reserved for coprocessor worker buffers. |",
		"| `0x200000-0x27FFFF` | 512KB | Shared RAM | IE32 worker area | All CPU cores | Coprocessor convention | Lies inside the graphics-visible shared-memory range; not a separate address space. |",
		fmt.Sprintf("| `0x%X-0x%X` | 64KB | Shared RAM | Media-loader staging buffer | All CPU cores | Media-loader subsystem | Reservation at the base of the AROS fast-memory range when that profile is active. |", MEDIA_STAGING_BASE, MEDIA_STAGING_END),
	} {
		if !strings.Contains(section, needle) {
			t.Fatalf("architecture.md memory map missing shared-address-space contract detail: %s", needle)
		}
	}
	if strings.Contains(section, "Additional special regions used by the coprocessor subsystem") {
		t.Fatal("architecture.md still presents coprocessor reservations as a separate unexplained region table")
	}
}

func TestSDKCompanionDocs_ArchitectureMemoryMapCoversSIDSourceRanges(t *testing.T) {
	arch := readAuditFile(t, "sdk/docs/architecture.md")
	section := markdownSection(t, arch, "## 6. Memory Map", "## 7. I/O Peripherals")
	requiredRows := []string{
		fmt.Sprintf("| `0x%05X-0x%05X` | %dB | MMIO | SID1 engine (6581/8580) | All CPU cores | Audio subsystem | Primary SID register block. |", SID_BASE, SID_END, SID_END-SID_BASE+1),
		fmt.Sprintf("| `0x%05X-0x%05X` | %dB | MMIO | SID player | All CPU cores | Audio subsystem | Player-facing register block. |", SID_PLAY_PTR, SID_SUBSONG, SID_SUBSONG-SID_PLAY_PTR+1),
		fmt.Sprintf("| `0x%05X-0x%05X` | %dB | MMIO | SID2 engine (6581/8580) | All CPU cores | Audio subsystem | Secondary SID register block. |", SID2_BASE, SID2_END, SID2_END-SID2_BASE+1),
		fmt.Sprintf("| `0x%05X-0x%05X` | %dB | MMIO | SID3 engine (6581/8580) | All CPU cores | Audio subsystem | Tertiary SID register block. |", SID3_BASE, SID3_END, SID3_END-SID3_BASE+1),
	}
	for _, row := range requiredRows {
		if !strings.Contains(section, row) {
			t.Fatalf("architecture.md memory map missing source-backed SID row: %s", row)
		}
	}
	for _, stale := range []string{
		"| `0xF0E00-0xF0E19` | 26B | MMIO | SID1 engine",
		"| `0xF0E30-0xF0E6C` | 61B | MMIO | SID2 + SID3",
		"SID 6581/8580 x3<br/>0xF0E00-0xF0E6C",
	} {
		if strings.Contains(arch, stale) {
			t.Fatalf("architecture.md still documents stale SID mapping: %s", stale)
		}
	}
}

func TestSDKCompanionDocs_ArchitectureMemoryMapCoversTEDAndMediaSourceRanges(t *testing.T) {
	arch := readAuditFile(t, "sdk/docs/architecture.md")
	section := markdownSection(t, arch, "## 6. Memory Map", "## 7. I/O Peripherals")
	requiredRows := []string{
		fmt.Sprintf("| `0x%05X-0x%05X` | %dB | MMIO | TED player | All CPU cores | Audio subsystem | Player-facing register block. |", TED_PLAY_PTR, TED_PLAY_STATUS+3, TED_PLAY_STATUS+3-TED_PLAY_PTR+1),
		fmt.Sprintf("| `0x%05X-0x%05X` | 16KB | MMIO / TED VRAM aperture | TED private video RAM | All CPU cores | TED video subsystem | Bus-routed TED character, colour, and screen memory. |", TED_V_VRAM_BASE, TED_V_VRAM_BASE+TED_V_VRAM_SIZE-1),
		fmt.Sprintf("| `0x%X-0x%X` | 64KB | Shared RAM | Media-loader staging buffer | All CPU cores | Media-loader subsystem | Reservation at the base of the AROS fast-memory range when that profile is active. |", MEDIA_STAGING_BASE, MEDIA_STAGING_END),
	}
	for _, row := range requiredRows {
		if !strings.Contains(section, row) {
			t.Fatalf("architecture.md memory map missing source-backed row: %s", row)
		}
	}
	if strings.Contains(section, "| `0xF0F10-0xF0F1C` | 13B | MMIO | TED player |") {
		t.Fatal("architecture.md still documents the stale 13-byte TED player range")
	}
	if strings.Contains(section, "| `0x800000` | 64KB | Shared RAM | Media-loader staging buffer |") {
		t.Fatal("architecture.md still documents media staging as a base address instead of an inclusive range")
	}
}

func TestSDKCompanionDocs_IE64FaultCauseMFCRCR6Exception(t *testing.T) {
	doc := readAuditFile(t, "sdk/docs/IE64_ISA.md")
	privSection := markdownSection(t, doc, "### 11.1 Privilege Levels", "### 11.2 Control Registers")
	if !strings.Contains(privSection, "Privileged instructions cause a fault (cause code 5), except `MFCR Rd, CR6` which is user-readable.") {
		t.Fatal("IE64_ISA.md privilege summary row does not include the MFCR CR6 user-mode exception")
	}
	if !strings.Contains(doc, "**User-mode exception**: MFCR is normally supervisor-only, but reading CR6 (TP) is permitted in user mode.") {
		t.Fatal("IE64_ISA.md no longer documents the MFCR CR6 user-mode exception")
	}
	if !strings.Contains(doc, "MTCR, MFCR except `MFCR Rd, CR6`, ERET") {
		t.Fatal("IE64_ISA.md fault cause table does not qualify MFCR with the CR6/TP exception")
	}
	if strings.Contains(doc, "User-mode execution of a privileged instruction (MTCR, MFCR, ERET") {
		t.Fatal("IE64_ISA.md fault cause table still says all MFCR faults in user mode")
	}
}

func TestSDKCompanionDocs_IE64TimerContractLabelsImplementationFields(t *testing.T) {
	doc := readAuditFile(t, "sdk/docs/IE64_ISA.md")
	source := readAuditFile(t, "cpu_ie64.go")
	section := markdownSection(t, doc, "### 9.2 Timer Registers", "### 9.3 Timer Operation")
	for _, needle := range []string{
		"The architectural IE64 timer contract is the CR9/CR10/CR11 control-register",
		"| CR9 | TIMER_PERIOD | Auto-reload value in timer-step units |",
		"| CR10 | TIMER_COUNT | Current countdown value in timer-step units |",
		"| CR11 | TIMER_CTRL | Enable and interrupt-enable bits |",
		"The IE64 timer is CPU-integrated. The architectural programming contract is the control-register interface above.",
	} {
		if !strings.Contains(section, needle) {
			t.Fatalf("IE64_ISA.md timer register section does not state the CPU-visible contract: %s", needle)
		}
	}
	for _, forbidden := range []string{
		"Implementation note",
		"current Go implementation",
		"timer fields",
		"retired-instruction units",
	} {
		if strings.Contains(section, forbidden) {
			t.Fatalf("IE64_ISA.md timer register section still reads like implementation documentation: %s", forbidden)
		}
	}
	operation := markdownSection(t, doc, "### 9.3 Timer Operation", "### 9.4 Non-MMU Interrupt Flow")
	normalizedOperation := strings.Join(strings.Fields(operation), " ")
	for _, needle := range []string{
		"The timer countdown advances once per decoded instruction step",
		"after the instruction word and operand fields have been fetched and before the decoded",
		"for each decoded IE64 instruction step",
	} {
		if !strings.Contains(normalizedOperation, needle) {
			t.Fatalf("IE64_ISA.md timer operation section does not match source timing point: %s", needle)
		}
	}
	if strings.Contains(operation, "retired IE64 instruction") {
		t.Fatal("IE64_ISA.md timer operation uses over-strong retirement wording")
	}
	reloadIdx := strings.Index(source, "cpu.timerCount.Store(cpu.timerPeriod.Load())")
	dispatchIdx := strings.Index(source, "if !cpu.interruptEnabled.Load() || cpu.inInterrupt.Load()")
	if reloadIdx < 0 || dispatchIdx < 0 || reloadIdx > dispatchIdx {
		t.Fatal("cpu_ie64.go timer reload/interrupt order changed; review IE64 timer documentation")
	}
	for _, needle := range []string{
		"If the timer is still enabled, the count is reloaded from TIMER_PERIOD.",
		"If interrupts are enabled and the CPU is not already servicing an interrupt, the interrupt handler fires.",
		"handler that reads TIMER_COUNT after entry observes the reloaded value",
	} {
		if !strings.Contains(operation, needle) {
			t.Fatalf("IE64_ISA.md timer expiry order does not match source reload-before-dispatch semantics: %s", needle)
		}
	}
}

func TestSDKCompanionDocs_IE64FixedVectorTimerPatternIsNonProgrammable(t *testing.T) {
	doc := readAuditFile(t, "sdk/docs/IE64_ISA.md")
	source := readAuditFile(t, "cpu_ie64.go") + readAuditFile(t, "machine_bus_phys.go")
	if !strings.Contains(source, "cpu.interruptVector = 0") {
		t.Fatal("cpu_ie64.go no longer resets interruptVector to zero")
	}
	if !strings.Contains(source, "cpu.PC = cpu.interruptVector") {
		t.Fatal("cpu_ie64.go no longer jumps to interruptVector for fixed-vector timer delivery")
	}
	section := markdownSection(t, doc, "#### 9.5.2 Non-Programmable Fixed-Vector Model (MMU disabled)", "### 9.6 SEI/CLI Semantics")
	for _, needle := range []string{
		"fixed reset vector",
		"programmable interrupt-vector ABI",
		"On reset, `interruptVector` is zero",
		"there is no instruction that sets it",
		"Ordinary software cannot configure that vector",
		"Supervisor software that needs programmable IE64 timer interrupts should use the",
		"unified CR7/ERET model",
	} {
		if !strings.Contains(section, needle) {
			t.Fatalf("IE64_ISA.md fixed-vector timer section does not mark vector as non-programmable: %s", needle)
		}
	}
	for _, forbidden := range []string{
		"Host tests",
		"embedding code",
		"guest-visible instruction",
		"legacy",
		"compatibility path",
	} {
		if strings.Contains(section, forbidden) {
			t.Fatalf("IE64_ISA.md fixed-vector timer section still contains non-CPU-manual wording: %s", forbidden)
		}
	}
}

func TestSDKCompanionDocs_IE64ISADoesNotCarrySoftwareABI(t *testing.T) {
	doc := readAuditFile(t, "sdk/docs/IE64_ISA.md")
	for _, forbidden := range []string{
		"## Appendix C: IntuitionOS IE64 ABI v0",
		"| R1-R6 | Arguments and scratch | No |",
		"| R13-R15 | Callee-saved registers | Yes |",
		"| R31 / SP | Stack pointer | Yes |",
		"SP must be 16-byte aligned immediately before each `jsr`",
		"`SYSCALL #imm32` enters the kernel with the syscall number in the immediate field and arguments in R1-R6",
		"Current IExec syscall dispatch uses general-purpose registers internally",
		"Timer interrupts are different: current IExec timer preemption saves and restores the full user-visible GPR set",
		"TP (`CR6`) is currently zero at task startup",
		"Supervisor reads or writes to pages with `PTE_U=1` fault with `FAULT_SKAC` unless the privileged SUA latch is open",
	} {
		if strings.Contains(doc, forbidden) {
			t.Fatalf("IE64_ISA.md still carries software ABI material outside CPU ISA scope: %s", forbidden)
		}
	}
}

func TestSDKCompanionDocs_IE32OpcodeTableMatchesSource(t *testing.T) {
	source := readAuditFile(t, "cpu_ie32.go")
	doc := readAuditFile(t, "sdk/docs/IE32_ISA.md")
	sourceOps := parseIE32SourceOpcodes(t, source)
	docOps := parseMarkdownOpcodeRows(t, doc)

	for _, op := range sourceOps {
		if got, ok := docOps[op.name][op.value]; !ok {
			t.Fatalf("IE32 opcode %s=0x%02X missing from SDK table", op.name, op.value)
		} else if got != op.name {
			t.Fatalf("IE32 opcode 0x%02X mnemonic = %s, want %s", op.value, got, op.name)
		}
	}
}

func TestSDKCompanionDocs_IE32CompleteReferenceCoversINCDEC(t *testing.T) {
	doc := readAuditFile(t, "sdk/docs/IE32_ISA.md")
	section := markdownSection(t, doc, "### 4.3 Arithmetic", "### 4.4 Logical")
	for _, needle := range []string{
		"#### 40. INC - INC operand",
		"**Operation:** Increment register or memory target selected by operand.",
		"#### 41. DEC - DEC operand",
		"**Operation:** Decrement register or memory target selected by operand.",
		"`INC` and `DEC` use the operand mode as the destination selector.",
	} {
		if !strings.Contains(section, needle) {
			t.Fatalf("IE32 complete instruction reference missing INC/DEC detail: %s", needle)
		}
	}
}

func TestSDKCompanionDocs_IE32AddressSpaceDoesNotClaimFixedRAMSize(t *testing.T) {
	doc := readAuditFile(t, "sdk/docs/IE32_ISA.md")
	bootSource := readAuditFile(t, "boot_guest_ram.go")
	busSource := readAuditFile(t, "machine_bus.go")
	for _, needle := range []string{
		"case modeIE32, modeX86, modeM68KBare:",
		"return busMemMaxBytes, busMemMaxBytes",
		"if busTotalGuestRAM > busMemMaxBytes",
	} {
		if !strings.Contains(bootSource, needle) {
			t.Fatalf("boot_guest_ram.go IE32 runtime memory cap path changed: %s", needle)
		}
	}
	for _, needle := range []string{
		"const busMemMaxBytes uint64 = 0xFFFF0000",
		"NewMachineBus returns a MachineBus with the legacy 32 MiB bus.memory.",
	} {
		if !strings.Contains(busSource, needle) {
			t.Fatalf("machine_bus.go legacy/default memory wording changed: %s", needle)
		}
	}
	for _, needle := range []string{
		"32-bit effective addresses for instruction fetch",
		"CPU defines address formation and access width; it does not define a fixed",
		"Addresses outside the reset, stack, and interrupt-vector",
		"conventions above have no additional meaning in the IE32 CPU ISA.",
	} {
		if !strings.Contains(doc, needle) {
			t.Fatalf("IE32_ISA.md address-space wording missing CPU-only RAM-size boundary: %s", needle)
		}
	}
	for _, stale := range []string{
		"The default backing memory size is\n  `DEFAULT_MEMORY_SIZE`, currently 32 MiB.",
		"In the production runtime, IE32 uses",
		"current shared `MachineBus` memory sized at boot",
		"`DEFAULT_MEMORY_SIZE`",
	} {
		if strings.Contains(doc, stale) {
			t.Fatalf("IE32_ISA.md still presents runtime memory sizing as ISA contract: %s", stale)
		}
	}
}

func TestSDKCompanionDocs_IE32TimerMirrorsAreNotDocumentedAsGuestMMIO(t *testing.T) {
	doc := readAuditFile(t, "sdk/docs/IE32_ISA.md")
	source := readAuditFile(t, "cpu_ie32.go")
	asm := readAuditFile(t, "assembler/ie32asm.go")
	include := readAuditFile(t, "sdk/include/ie32.inc")
	for _, needle := range []string{
		"TIMER_COUNT  = IO_BASE + 0x04",
		"TIMER_PERIOD = IO_BASE + 0x08",
	} {
		if !strings.Contains(source, needle) {
			t.Fatalf("cpu_ie32.go timer runtime constant changed: %s", needle)
		}
	}
	if !strings.Contains(asm, "TIMER_COUNT  uint32 = 0xF804") {
		t.Fatal("assembler/ie32asm.go no longer contains the legacy TIMER_COUNT symbol expected by the compatibility note")
	}
	if !strings.Contains(include, ".equ TIMER_COUNT       0x00F804") {
		t.Fatal("sdk/include/ie32.inc no longer contains the legacy TIMER_COUNT symbol expected by the compatibility note")
	}
	for _, needle := range []string{
		"There is no ISA-level externally addressable timer-control register.",
		"Timer state",
		"CPU-integrated timer state and the instruction-level",
	} {
		if !strings.Contains(doc, needle) {
			t.Fatalf("IE32_ISA.md missing CPU-only timer boundary: %s", needle)
		}
	}
	for _, forbidden := range []string{
		"### 7.1 Internal Timer Mirrors vs Legacy Assembler Symbols",
		"`TIMER_COUNT = 0xF0804`",
		"`TIMER_PERIOD = 0xF0808`",
		"guest MMIO timer controls",
		"architectural bus/MMIO timer ABI",
		"`0xF0804`",
		"`0xF0808`",
		"`0x00F804`",
		"`0x00F808`",
	} {
		if strings.Contains(doc, forbidden) {
			t.Fatalf("IE32_ISA.md still documents timer mirror/platform addresses in the ISA manual: %s", forbidden)
		}
	}
	if strings.Contains(doc, "New IE32 code should use the documented runtime addresses") {
		t.Fatal("IE32_ISA.md still implies internal timer mirror offsets are guest ABI")
	}
}

func TestSDKCompanionDocs_ISAOpcodeAppendicesUseEquivalentTableShape(t *testing.T) {
	for _, path := range []string{"sdk/docs/IE64_ISA.md", "sdk/docs/IE32_ISA.md"} {
		doc := readAuditFile(t, path)
		if !strings.Contains(doc, "| Opcode | Operation | Syntax |") {
			t.Fatalf("%s instruction summary table does not use the Motorola-style Opcode/Operation/Syntax schema", path)
		}
		if strings.Contains(doc, "| Opcode | Hex") {
			t.Fatalf("%s duplicates opcode values in a Hex column; use one opcode column plus instruction format diagrams", path)
		}
		if !strings.Contains(doc, "### A.2 Machine Opcode Encoding Map") {
			t.Fatalf("%s is missing the separate machine opcode encoding map", path)
		}
		if !strings.Contains(doc, "### A.3 Opcode Ranges") {
			t.Fatalf("%s is missing the common opcode ranges table", path)
		}
	}
}

func TestSDKCompanionDocs_IEScriptFunctionCoverageMatchesBindings(t *testing.T) {
	source := readAuditFile(t, "script_engine.go")
	doc := readAuditFile(t, "sdk/docs/iescript.md")
	for _, module := range []string{"sys", "cpu", "mem", "term", "audio", "video", "rec", "repl", "dbg", "sym", "regions", "coproc", "media", "bit32"} {
		funcs := parseLuaModuleFunctions(t, source, module)
		for _, fn := range funcs {
			needle := module + "." + fn
			if !strings.Contains(doc, needle) {
				t.Fatalf("iescript.md does not document exported Lua function %s", needle)
			}
		}
		countRe := regexp.MustCompile(`(?m)^### ` + regexp.QuoteMeta(module) + ` \((\d+)\)$`)
		if m := countRe.FindStringSubmatch(doc); m != nil {
			got, _ := strconv.Atoi(m[1])
			if got != len(funcs) {
				t.Fatalf("iescript.md quick reference count for %s = %d, want %d", module, got, len(funcs))
			}
		}
		sectionHeading := "### " + module
		if countRe.MatchString(doc) {
			start := strings.Index(doc, sectionHeading)
			if start < 0 {
				t.Fatalf("iescript.md missing quick reference section %s", sectionHeading)
			}
			section := doc[start:]
			if end := strings.Index(section[len(sectionHeading):], "\n### "); end >= 0 {
				section = section[:len(sectionHeading)+end]
			}
			rowRe := regexp.MustCompile(`(?m)^\| ` + "`" + regexp.QuoteMeta(module) + `\.[^` + "`" + `]+` + "`" + ` \|`)
			if got := len(rowRe.FindAllString(section, -1)); got != len(funcs) {
				t.Fatalf("iescript.md quick reference rows for %s = %d, want %d", module, got, len(funcs))
			}
		}
	}
}

func TestSDKCompanionDocs_IEScriptRawMonitorFilterMatchesSource(t *testing.T) {
	source := readAuditFile(t, "script_engine.go")
	doc := readAuditFile(t, "sdk/docs/iescript.md")
	if !strings.Contains(source, `return fmt.Errorf("dbg.command cannot run trace file; use dbg.trace_file")`) {
		t.Fatal("script_engine.go no longer rejects trace file through dbg.command")
	}
	for _, needle := range []string{
		"`macro`, and `trace file`; `trace file off` is allowed",
		"Host-file-capable monitor commands are rejected (`save`, `load`, `ss`, `sl`, `script`, `macro`, and `trace file`; `trace file off` is allowed)",
	} {
		if !strings.Contains(doc, needle) {
			t.Fatalf("iescript.md does not document raw monitor filter detail: %s", needle)
		}
	}
}

func TestSDKCompanionDocs_IEScriptJITCPUListIncludesX86(t *testing.T) {
	source := readAuditFile(t, "script_engine.go")
	doc := readAuditFile(t, "sdk/docs/iescript.md")
	for _, needle := range []string{
		"case runtimeCPUX86:",
		"snap.x86.cpu.x86JitEnabled = enabled && x86JitAvailable",
		`L.RaiseError("x86 JIT unavailable on this platform")`,
	} {
		if !strings.Contains(source, needle) {
			t.Fatalf("script_engine.go no longer exposes x86 through script-controlled JIT path: %s", needle)
		}
	}
	for _, needle := range []string{
		"Supported for m68k, z80, x86, 6502, and ie64",
		"currently m68k, z80, x86, 6502, and ie64",
		"x86, m68k, z80, and 6502 JIT backends are amd64 host paths",
		"IE64 also has arm64 host paths as described in architecture.md",
	} {
		if !strings.Contains(doc, needle) {
			t.Fatalf("iescript.md JIT API docs missing x86/platform detail: %s", needle)
		}
	}
	if strings.Contains(doc, "currently only m68k, z80, 6502, and ie64 are supported") {
		t.Fatal("iescript.md still omits x86 from script-controlled JIT support")
	}
}

func TestSDKCompanionDocs_IEScriptStateSaveLoadIsCPULocal(t *testing.T) {
	source := readAuditFile(t, "script_engine.go")
	doc := readAuditFile(t, "sdk/docs/iescript.md")
	for _, needle := range []string{
		`mon.ExecuteCommandResult("ss " + quoteMonitorArg(validated))`,
		"RestoreSnapshot(cpu, snap)",
	} {
		if !strings.Contains(source, needle) {
			t.Fatalf("script_engine.go no longer uses CPU-local monitor snapshot path expected by docs: %s", needle)
		}
	}
	section := markdownSection(t, doc, "### State Save/Load", "### Multi-CPU")
	for _, needle := range []string{
		"`dbg.save_state(path)` - Save a CPU-local monitor snapshot",
		"by using IEMon's `ss` command",
		"does not save other CPUs, device/chip runtime state, audio/video state, timers, DMA, or monitor reverse-history state",
		"`dbg.load_state(path)` - Restore a CPU-local monitor snapshot",
		"does not restore whole-machine state",
		"Use `dbg.reverse_continue()` (`rg`) or `dbg.reverse_until(expr)` (`rt <expr>`) for IEMon's whole-machine reverse-history semantics",
	} {
		if !strings.Contains(section, needle) {
			t.Fatalf("iescript.md state save/load docs overstate or omit CPU-local scope: %s", needle)
		}
	}
	if strings.Contains(section, "Save the current machine state") || strings.Contains(section, "Restore machine state") {
		t.Fatal("iescript.md still describes dbg.save_state/load_state as whole-machine state")
	}
}

func TestSDKCompanionDocs_IEScriptDebugMemoryHelpersDistinguishCPUAndRawBus(t *testing.T) {
	source := readAuditFile(t, "script_engine.go")
	doc := readAuditFile(t, "sdk/docs/iescript.md")
	for _, needle := range []string{
		"func (se *ScriptEngine) luaDbgReadMem()",
		"cpu.ReadMemory(addr, n)",
		"func (se *ScriptEngine) luaDbgWriteMem()",
		"cpu.WriteMemory(addr, data)",
		"func (se *ScriptEngine) luaDbgFillMem()",
		"start := uint32(L.CheckInt64(1))",
		"se.bus.Write8(start+uint32(i), val)",
		"func (se *ScriptEngine) luaDbgHuntMem()",
		"if se.bus.Read8(start+uint32(i+j)) != pat[j]",
		"func (se *ScriptEngine) luaDbgCompareMem()",
		"dest := uint32(L.CheckInt64(3))",
		"func (se *ScriptEngine) luaDbgTransferMem()",
		"buf[i] = se.bus.Read8(start + uint32(i))",
		"se.bus.Write8(dest+uint32(i), b)",
	} {
		if !strings.Contains(source, needle) {
			t.Fatalf("script_engine.go memory helper path changed; docs gate expected: %s", needle)
		}
	}
	section := markdownSection(t, doc, "### Memory", "### Disassembly and Trace")
	for _, needle := range []string{
		"`dbg.read_mem(addr, len)` - Read `len` bytes from the focussed CPU's memory at `addr`.",
		"`dbg.write_mem(addr, data)` - Write raw byte string `data` to the focussed CPU's memory at `addr`.",
		"`dbg.fill_mem(addr, len, value)` - Raw 32-bit bus helper",
		"`dbg.hunt_mem(start, len, pattern)` - Raw 32-bit bus helper",
		"`dbg.compare_mem(start, len, dest)` - Raw 32-bit bus helper",
		"`dbg.transfer_mem(start, len, dest)` - Raw 32-bit bus helper",
		"Addresses above `0xFFFFFFFF` are truncated to their low 32 bits before access.",
		"`dbg.fill_mem`,",
		"`dbg.hunt_mem`, `dbg.compare_mem`, and `dbg.transfer_mem` use the raw shared",
		"bus path after converting their address arguments to `uint32`",
		"not for CPU-virtual or above-4GiB IE64",
	} {
		if !strings.Contains(section, needle) {
			t.Fatalf("iescript.md memory helper docs missing CPU-vs-raw-bus detail: %s", needle)
		}
	}
}

func TestSDKCompanionDocs_IEScriptDbgMonitorParityAPIsAreFullyDocumented(t *testing.T) {
	source := readAuditFile(t, "script_engine.go")
	doc := readAuditFile(t, "sdk/docs/iescript.md")
	for _, needle := range []string{
		"\"backtrace_frames\":",
		"se.luaDbgBacktraceFrames()",
		"\"tracering_on\":",
		"se.luaDbgTraceRingOn()",
		"\"tracering_off\":",
		"se.luaDbgTraceRingOff()",
		"\"tracering_show\":",
		"se.luaDbgTraceRingShow()",
		"\"source_at\":",
		"se.luaDbgSourceAt()",
		"\"history_horizon\":",
		"se.luaDbgHistoryHorizon()",
		"\"history_config\":",
		"se.luaDbgHistoryConfig()",
		"\"device_list\":",
		"se.luaDbgDeviceList()",
		"\"device_snapshot\":",
		"se.luaDbgDeviceSnapshot()",
		"\"device_diff\":",
		"se.luaDbgDeviceDiff()",
		"\"layout\":",
		"se.luaDbgLayout()",
		"\"bug_report\":",
		"se.luaDbgBugReport()",
		"\"help\":",
		"se.luaDbgHelp()",
	} {
		if !strings.Contains(source, needle) {
			t.Fatalf("script_engine.go dbg export changed; docs gate expected: %s", needle)
		}
	}
	for _, needle := range []string{
		"`dbg.backtrace_frames([depth])` - Return a structured call stack backtrace",
		"| `frame` | number | Zero-based frame index |",
		"| `sym` | string/nil | Symbol name when symbol resolution succeeds; `nil` otherwise |",
		"`dbg.tracering_on([size])` - Enable the focussed CPU trace ring.",
		"`dbg.tracering_show([count])` - Return the last `count` trace-ring entries",
		"| `cpu` | string | CPU name recorded with the trace entry |",
		"`dbg.source_at(addr)` - Resolve `addr` through the focussed CPU's loaded source map.",
		"Returns `nil` when no monitor or CPU is available or no source record covers the address.",
		"`dbg.history_horizon()` - Return whole-machine reverse-history retention state.",
		"| `checkpoint_interval` | number | Instructions between full checkpoints |",
		"`dbg.history_config([opts])` - Read or update whole-machine reverse-history configuration.",
		"`delta_interval`, `delta_mib`, `checkpoints`, and `snapshots`",
		"### Device Snapshots",
		"`dbg.device_list()` - Return the sorted names of devices registered with IEMon's versioned snapshot service.",
		"`dbg.device_snapshot(name)` - Capture one registered device snapshot.",
		"Returns `nil` when `name` is not registered.",
		"| `data` | string | Opaque byte string containing the device snapshot payload |",
		"`dbg.device_diff(a, b)` - Compare two device snapshot tables",
		"`dbg.layout(name)` - Run IEMon `layout <name>`",
		"`dbg.bug_report([trace_count])` - Run IEMon `bug <trace_count>`",
		"`dbg.help([topic])` - Run IEMon `help` or `help <topic>`",
	} {
		if !strings.Contains(doc, needle) {
			t.Fatalf("iescript.md missing full dbg API reference detail: %s", needle)
		}
	}
}

func TestSDKCompanionDocs_IEScriptMemModuleIsDocumentedAs32BitBusAPI(t *testing.T) {
	source := readAuditFile(t, "script_engine.go")
	doc := readAuditFile(t, "sdk/docs/iescript.md")
	arch := readAuditFile(t, "sdk/docs/architecture.md")
	for _, needle := range []string{
		"func (se *ScriptEngine) luaMemRead8()",
		"addr := uint32(L.CheckInt(1))",
		"L.Push(lua.LNumber(se.bus.Read8(addr)))",
		"func (se *ScriptEngine) luaMemReadBlock()",
		"out[i] = se.bus.Read8(addr + uint32(i))",
		"func (se *ScriptEngine) luaMemFill()",
		"se.bus.Write8(addr+uint32(i), val)",
	} {
		if !strings.Contains(source, needle) {
			t.Fatalf("script_engine.go mem.* 32-bit bus helper path changed: %s", needle)
		}
	}
	section := markdownSection(t, doc, "## `mem`", "## `term`")
	for _, needle := range []string{
		"The `mem.*` module is a raw 32-bit shared-bus API.",
		"Each address argument is",
		"converted to `uint32` before access and the functions call the shared bus",
		"Addresses above `0xFFFFFFFF` are",
		"truncated to their low 32 bits.",
		"not an above-4GiB IE64 RAM or CPU-virtual-address API",
		"`mem.read8(addr)` - Read one byte from bus address `uint32(addr)`.",
		"`mem.write32(addr, value)` - Write a 32-bit word `value` to bus address `uint32(addr)`.",
		"`mem.read_block(addr, len)` - Read `len` bytes starting at bus address `uint32(addr)`.",
		"`mem.fill(addr, len, value)` - Fill `len` bytes starting at bus address `uint32(addr)`",
	} {
		if !strings.Contains(section, needle) {
			t.Fatalf("iescript.md mem.* docs missing raw 32-bit bus detail: %s", needle)
		}
	}
	for _, needle := range []string{
		"shared 32-bit bus/MMIO surface plus",
		"Its `mem.*` helpers are raw 32-bit bus helpers",
		"an above-4GiB IE64 RAM or CPU-virtual-address API",
	} {
		if !strings.Contains(arch, needle) {
			t.Fatalf("architecture.md Lua scripting section still overstates script bus reach: %s", needle)
		}
	}
	if strings.Contains(arch, "provides host-side access to the entire bus") {
		t.Fatal("architecture.md still says Lua has access to the entire bus")
	}
}

func TestSDKCompanionDocs_IEMonCommandCoverageMatchesHelpRegistry(t *testing.T) {
	doc := readAuditFile(t, "sdk/docs/iemon.md")
	lowerDoc := strings.ToLower(doc)

	for _, entry := range monitorHelpRegistry() {
		if !markdownMentionsCommand(lowerDoc, entry.Name) {
			t.Fatalf("iemon.md does not document monitor command %q from help registry", entry.Name)
		}
		for _, syntax := range entry.Syntax {
			first := strings.Fields(syntax)
			if len(first) == 0 {
				continue
			}
			cmd := strings.Trim(first[0], "`")
			if strings.Contains(cmd, "|") {
				for _, alias := range strings.Split(cmd, "|") {
					alias = strings.Trim(alias, "`")
					if alias != "" && !markdownMentionsCommand(lowerDoc, alias) {
						t.Fatalf("iemon.md does not document monitor command alias %q from syntax %q", alias, syntax)
					}
				}
			}
		}
	}
	if !markdownMentionsCommand(lowerDoc, "?") {
		t.Fatal("iemon.md does not document ? help alias")
	}
}

func TestSDKCompanionDocs_IEMonDispatchAliasesAndIE64Cause11(t *testing.T) {
	doc := readAuditFile(t, "sdk/docs/iemon.md")
	source := readAuditFile(t, "debug_commands.go")
	cpuSource := readAuditFile(t, "cpu_ie64.go")
	if !strings.Contains(source, `case "wr", "wrw":`) {
		t.Fatal("debug_commands.go no longer exposes wr/wrw dispatch aliases")
	}
	if !strings.Contains(cpuSource, "FAULT_ILLEGAL_INSTRUCTION = 11") {
		t.Fatal("cpu_ie64.go no longer defines IE64 illegal-instruction cause 11")
	}
	for _, needle := range []string{
		"monitor command registry plus",
		"dispatch-level aliases",
		"| `wr` / `wrw` | Set a legacy one-byte read/write watchpoint |",
		"| 11    | `illegal`          | Opcode-level invariant or illegal-instruction trap, currently including `MTCR` to read-only `CR_RAM_SIZE_BYTES` |",
	} {
		if !strings.Contains(doc, needle) {
			t.Fatalf("iemon.md missing dispatch alias or IE64 cause-11 detail: %s", needle)
		}
	}
	if strings.Contains(doc, "The command surface below is enumerated from `monitorHelpRegistry()`") {
		t.Fatal("iemon.md still describes command coverage as help-registry-only")
	}
}

func TestSDKCompanionDocs_IEMonSaveArgumentParsingMatchesSource(t *testing.T) {
	doc := readAuditFile(t, "sdk/docs/iemon.md")
	source := readAuditFile(t, "debug_commands.go")
	for _, needle := range []string{
		"start, ok1 := m.evalAddress(cmd.Args[0], entry)",
		"end, ok2 := m.evalAddress(cmd.Args[1], entry)",
		`m.appendOutput("Usage: save <start> <end> <filename>", colorRed)`,
	} {
		if !strings.Contains(source, needle) {
			t.Fatalf("debug_commands.go save parsing no longer matches docs expectation: %s", needle)
		}
	}
	matrix := markdownSection(t, doc, "### Argument Parsing Matrix", "The matrix is intentionally command-facing")
	want := "| `save <start> <end> <file>` | Start and end address operands both accept register, symbol, `+`, and `-` terms | Filename is a host path argument, not an expression |"
	if !strings.Contains(matrix, want) {
		t.Fatalf("iemon.md argument parsing matrix missing corrected save row: %s", want)
	}
	if strings.Contains(matrix, "Filename and byte count are parsed separately") {
		t.Fatal("iemon.md still describes save as start+byte-count instead of start/end/file")
	}
}

func TestSDKCompanionDocs_ISAEncodingExamplesMatchSourceConstants(t *testing.T) {
	ie64 := readAuditFile(t, "sdk/docs/IE64_ISA.md")
	ie64Examples := map[string][]byte{
		"move.l r5, #$CAFEBABE": encodeIE64AuditInstr(OP_MOVE, 5, IE64_SIZE_L, 1, 0, 0, 0xCAFEBABE),
		"add.q r3, r1, r2":      encodeIE64AuditInstr(OP_ADD, 3, IE64_SIZE_Q, 0, 1, 2, 0),
		"beq r1, r2, target":    encodeIE64AuditInstr(OP_BEQ, 0, IE64_SIZE_Q, 0, 1, 2, 24),
		"store.b r7, 4(r10)":    encodeIE64AuditInstr(OP_STORE, 7, IE64_SIZE_B, 1, 10, 0, 4),
		"push r15":              encodeIE64AuditInstr(OP_PUSH64, 0, IE64_SIZE_Q, 0, 15, 0, 0),
		"jmp (r5)":              encodeIE64AuditInstr(OP_JMP, 0, 0, 0, 5, 0, 0),
		"jsr 16(r3)":            encodeIE64AuditInstr(OP_JSR_IND, 0, 0, 0, 3, 0, 16),
	}
	for asm, bytes := range ie64Examples {
		if !strings.Contains(ie64, formatAuditBytes(bytes)) {
			t.Fatalf("IE64 encoding example %q missing bytes %s", asm, formatAuditBytes(bytes))
		}
	}

	ie32 := readAuditFile(t, "sdk/docs/IE32_ISA.md")
	ie32Examples := map[string][]byte{
		"LDA #$12345678": encodeIE32AuditInstr(LDA, 0, ADDR_IMMEDIATE, 0x12345678),
		"LOAD X, A":      encodeIE32AuditInstr(LOAD, REG_X, ADDR_REGISTER, REG_A),
		"STA @0x5000":    encodeIE32AuditInstr(STA, 0, ADDR_DIRECT, 0x5000),
		"LDA [B+16]":     encodeIE32AuditInstr(LDA, 0, ADDR_REG_IND, 0x14),
		"JNZ A, loop":    encodeIE32AuditInstr(JNZ, REG_A, 0, 0x1020),
		"PUSH W":         encodeIE32AuditInstr(PUSH, REG_W, 0, 0),
	}
	for asm, bytes := range ie32Examples {
		if !strings.Contains(ie32, formatAuditBytes(bytes)) {
			t.Fatalf("IE32 encoding example %q missing bytes %s", asm, formatAuditBytes(bytes))
		}
	}
}

func TestSDKCompanionDocs_ISADocsExcludeAssemblerToolAndMonitorMaterial(t *testing.T) {
	for _, path := range []string{"sdk/docs/IE64_ISA.md", "sdk/docs/IE32_ISA.md"} {
		doc := readAuditFile(t, path)
		for _, forbidden := range []string{
			"Assembly Language Quick Reference",
			"Monitor Disassembly Contract",
			"`.libmanifest`",
			"ie32asm [-v]",
			"-Werror",
			"-Wno-category",
			"-I dir",
			"Data directives and `.incbin`",
			"Include resolution checks",
			"The monitor's IE64 disassembler",
			"IEMon disassembles IE32 memory",
		} {
			if strings.Contains(doc, forbidden) {
				t.Fatalf("%s still contains assembler-tool or monitor-display material outside CPU ISA scope: %s", path, forbidden)
			}
		}
		for _, required := range []string{
			"Instruction Encoding",
			"Addressing Modes",
			"Complete Instruction Reference",
			"Opcode Map",
		} {
			if !strings.Contains(doc, required) {
				t.Fatalf("%s lost ISA reference section while removing tool material: %s", path, required)
			}
		}
	}
}

func TestSDKCompanionDocs_ISADocsUsePhysicalCPUReferenceVoice(t *testing.T) {
	for _, tc := range []struct {
		path  string
		title string
	}{
		{"sdk/docs/IE64_ISA.md", "# IE64 Processor User's Manual"},
		{"sdk/docs/IE32_ISA.md", "# IE32 Processor User's Manual"},
	} {
		doc := readAuditFile(t, tc.path)
		if !strings.HasPrefix(doc, tc.title+"\n") {
			t.Fatalf("%s title should present the document as a processor manual, want %q", tc.path, tc.title)
		}
	}
	for _, path := range []string{"sdk/docs/IE64_ISA.md", "sdk/docs/IE32_ISA.md"} {
		doc := readAuditFile(t, path)
		for _, forbidden := range []string{
			"Instruction Set Architecture Reference",
			"Complete ISA Specification",
			"Intuition Engine 64-bit RISC CPU",
			"Intuition Engine 32-bit RISC-like CPU",
			"cpu_ie32.go",
			"cpu_ie64.go",
			"CPU.Execute",
			"CPU.StepOne",
			"Execute() loop",
			"current Go implementation",
			"current implementation",
			"Implementation note",
			"Host tests",
			"embedding code",
			"future revision",
			"may add",
			"ie64dis",
			"in-monitor",
			"debugger",
			"disassembler",
			"monitor-display",
			"runtime decodes",
			"assembler currently",
			"current assembler",
			"accepts labels",
			"accepts labels or equates",
			"assembler computes offset",
			"assembler sets X=1",
			"The assembler emits",
			"syntax still uses",
			"out-of-backing",
			"mapped backing",
			"source-level",
			"source syntax",
			"raw machine code",
			"Intuition Engine platform",
			"JIT",
			"interpreter",
			"guest-visible",
			"hosts with sufficient memory",
			"print an error message",
			"halt execution",
			"cycleCounter",
			"timerEnabled",
			"timerCount",
			"timerPeriod",
			"timerState",
			"interruptEnabled",
			"inInterrupt",
			"historical",
			"transpiled programs",
			"Intuition Engine machine architecture",
			"Runtime resolution",
			"SAMPLE_RATE",
			"normal execution loop",
			"prints a diagnostic",
			"running flag",
			"legacy 25-bit",
			"host-integration",
		} {
			if strings.Contains(doc, forbidden) {
				t.Fatalf("%s still reads like emulator/tool documentation instead of a physical CPU manual: %s", path, forbidden)
			}
		}
	}
}

func TestSDKCompanionDocs_IE64HALTDocumentsStoppedPCBehavior(t *testing.T) {
	doc := readAuditFile(t, "sdk/docs/IE64_ISA.md")
	source := readAuditFile(t, "cpu_ie64.go")
	for _, needle := range []string{
		"case OP_HALT64:",
		"cpu.running.Store(false)",
		"continue",
	} {
		if !strings.Contains(source, needle) {
			t.Fatalf("cpu_ie64.go HALT source changed; review IE64 HALT documentation: %s", needle)
		}
	}
	entry := markdownSection(t, doc, "#### 110. HALT - halt", "#### 111. SEI - sei")
	for _, required := range []string{
		"`HALT` enters the stopped processor state",
		"program counter is not advanced",
		"No trap is generated",
	} {
		if !strings.Contains(entry, required) {
			t.Fatalf("IE64 HALT entry missing source-backed stopped-PC behavior: %s", required)
		}
	}
	if strings.Contains(entry, "None specific to this entry") || strings.Contains(entry, "The instruction itself enters") {
		t.Fatal("IE64 HALT entry uses generic exception text instead of documenting stopped state")
	}
}

func TestSDKCompanionDocs_IE64PCOverviewAllowsInstructionSpecificPCBehavior(t *testing.T) {
	doc := readAuditFile(t, "sdk/docs/IE64_ISA.md")
	source := readAuditFile(t, "cpu_ie64.go")
	for _, needle := range []string{
		"case OP_HALT64:",
		"continue",
		"case OP_RTI64:",
		"cpu.PC = val",
		"case OP_ERET:",
		"cpu.PC = cpu.faultPC",
	} {
		if !strings.Contains(source, needle) {
			t.Fatalf("cpu_ie64.go PC-flow source changed; review IE64 PC overview: %s", needle)
		}
	}
	overview := markdownSection(t, doc, "**Program Counter (PC)**:", "**There is no integer flags register.**")
	normalizedOverview := strings.Join(strings.Fields(overview), " ")
	for _, required := range []string{
		"Most sequential instructions advance `PC` by 8 bytes.",
		"Control-transfer, trap, return, fault, interrupt, and stopped-state instructions use the PC behaviour defined by their individual instruction entries.",
	} {
		if !strings.Contains(normalizedOverview, required) {
			t.Fatalf("IE64 PC overview missing instruction-specific PC wording: %s", required)
		}
	}
	if strings.Contains(overview, "Advanced by 8 after each non-branch instruction.") {
		t.Fatal("IE64 PC overview still overstates non-branch PC advancement")
	}
}

func TestSDKCompanionDocs_IE64FixedFormInstructionsDocumentReservedBytes(t *testing.T) {
	doc := readAuditFile(t, "sdk/docs/IE64_ISA.md")
	source := readAuditFile(t, "cpu_ie64.go")
	for _, needle := range []string{
		"case OP_SEI64:",
		"case OP_CLI64:",
		"case OP_RTI64:",
		"case OP_WAIT64:",
		"if imm32 > 0",
	} {
		if !strings.Contains(source, needle) {
			t.Fatalf("cpu_ie64.go fixed-form system instruction source changed; review IE64 ISA documentation: %s", needle)
		}
	}
	for _, tc := range []struct {
		heading string
		next    string
		opcode  string
		want    string
	}{
		{"#### 109. NOP - nop", "#### 110. HALT - halt", "0xE0", "Bytes 1-7 are reserved by this instruction and ignored by the processor."},
		{"#### 110. HALT - halt", "#### 111. SEI - sei", "0xE1", "Bytes 1-7 are reserved by this instruction and ignored by the processor."},
		{"#### 111. SEI - sei", "#### 112. CLI - cli", "0xE2", "Bytes 1-7 are reserved by this instruction and ignored by the processor."},
		{"#### 112. CLI - cli", "#### 113. RTI - rti", "0xE3", "Bytes 1-7 are reserved by this instruction and ignored by the processor."},
		{"#### 113. RTI - rti", "#### 114. WAIT - wait #usec", "0xE4", "Bytes 1-7 are reserved by this instruction and ignored by the processor."},
		{"#### 114. WAIT - wait #usec", "### 4.10 MMU, Privilege, and Atomic Instructions", "0xE5", "Bytes 1-3 are reserved by this instruction and ignored by the processor. Bytes 4-7 hold unsigned `imm32`"},
	} {
		entry := markdownSection(t, doc, tc.heading, tc.next)
		want := "Byte 0 holds opcode `" + tc.opcode + "`. " + tc.want
		if !strings.Contains(entry, want) {
			t.Fatalf("%s does not document fixed-form reserved bytes", tc.heading)
		}
		if strings.Contains(entry, "select `Rd`") || strings.Contains(entry, "`X` immediate/register selector") {
			t.Fatalf("%s still uses generic operand-field prose for a fixed-form instruction", tc.heading)
		}
	}
}

func TestSDKCompanionDocs_IE64StackAndJSRReferenceText(t *testing.T) {
	doc := readAuditFile(t, "sdk/docs/IE64_ISA.md")
	source := readAuditFile(t, "cpu_ie64.go") + readAuditFile(t, "machine_bus_phys.go")
	for _, needle := range []string{
		"case OP_JSR64:",
		"case OP_RTS64:",
		"case OP_PUSH64:",
		"case OP_POP64:",
		"case OP_JSR_IND:",
		"case OP_RTI64:",
		"mmuStackWrite",
		"mmuStackRead",
		"WritePhys64WithFault",
		"ReadPhys64WithFault",
	} {
		if !strings.Contains(source, needle) {
			t.Fatalf("IE64 stack source changed; review stack exception documentation: %s", needle)
		}
	}
	if strings.Contains(doc, "absolute subroutine-call form uses opcode `0x50`") {
		t.Fatal("IE64 JSR summary still calls opcode 0x50 absolute instead of PC-relative")
	}
	if !strings.Contains(doc, "#### 108. JSR - `jsr (Rs)` / `jsr disp(Rs)`") {
		t.Fatal("IE64 indirect JSR heading has broken Markdown formatting")
	}
	jsrInd := markdownSection(t, doc, "#### 108. JSR - `jsr (Rs)` / `jsr disp(Rs)`", "### 4.9 System")
	if !strings.Contains(jsrInd, "Byte 1 is reserved by this instruction and ignored by the processor.") {
		t.Fatal("IE64 indirect JSR entry does not mark byte 1 as reserved/ignored")
	}
	for _, forbidden := range []string{
		"bits 2-1 encode quadword size",
		"bits 2-1 select quadword size",
	} {
		if strings.Contains(jsrInd, forbidden) {
			t.Fatalf("IE64 indirect JSR entry still assigns unsupported size bits: %s", forbidden)
		}
	}
	for _, tc := range []struct {
		heading string
		next    string
		access  string
	}{
		{"#### 104. JSR - jsr label", "#### 105. RTS - rts", "stack write"},
		{"#### 105. RTS - rts", "#### 106. PUSH - push Rs", "stack read"},
		{"#### 106. PUSH - push Rs", "#### 107. POP - pop Rd", "stack write"},
		{"#### 107. POP - pop Rd", "#### 108. JSR - `jsr (Rs)` / `jsr disp(Rs)`", "stack read"},
		{"#### 108. JSR - `jsr (Rs)` / `jsr disp(Rs)`", "### 4.9 System", "stack write"},
		{"#### 113. RTI - rti", "#### 114. WAIT - wait #usec", "stack read"},
	} {
		entry := markdownSection(t, doc, tc.heading, tc.next)
		if !strings.Contains(entry, tc.access+" can trap") {
			t.Fatalf("%s missing MMU %s trap wording", tc.heading, tc.access)
		}
		want := "A physical 8-byte " + tc.access + " outside implemented CPU-visible memory enters the stopped processor state and does not create a trap frame."
		if !strings.Contains(entry, want) {
			t.Fatalf("%s missing physical CPU stack stopped-state wording: %s", tc.heading, want)
		}
		for _, forbidden := range []string{
			"out-of-backing",
			"outside mapped backing raises cause 0",
		} {
			if strings.Contains(entry, forbidden) {
				t.Fatalf("%s still uses emulator/storage stack wording: %s", tc.heading, forbidden)
			}
		}
	}
}

func TestSDKCompanionDocs_IE64ControlRegisterSyntaxAndMMUText(t *testing.T) {
	doc := readAuditFile(t, "sdk/docs/IE64_ISA.md")
	asm := readAuditFile(t, "assembler/ie64asm.go")
	mmu := readAuditFile(t, "mmu_ie64.go")
	for _, needle := range []string{
		`case "cr15", "ram_size_bytes":`,
		"return encodeInstruction(OP64_MTCR, cr, 0, 0, rs, 0, 0)",
		"return encodeInstruction(OP64_MFCR, rd, 0, 0, cr, 0, 0)",
	} {
		if !strings.Contains(asm, needle) {
			t.Fatalf("IE64 assembler CR syntax source changed; review IE64 ISA docs: %s", needle)
		}
	}
	for _, needle := range []string{
		"ppn      uint64",
		"leafAddr uint64",
		"flags    byte",
	} {
		if !strings.Contains(mmu, needle) {
			t.Fatalf("IE64 TLB source changed; review TLB documentation: %s", needle)
		}
	}
	for _, required := range []string{
		"mtcr cr8, r1",
		"mtcr cr7, r1",
		"mtcr cr9, r1",
		"mtcr cr11, r1",
		"`MTCR RAM_SIZE_BYTES, Rs`",
		"Each entry stores the VPN tag, physical page number, leaf-PTE physical address, and translation permission flags.",
		"the cached translation and permission flags are used",
	} {
		if !strings.Contains(doc, required) {
			t.Fatalf("IE64_ISA.md missing source-backed CR/TLB wording: %s", required)
		}
	}
	for _, forbidden := range []string{
		"mtcr 8, r1",
		"mtcr 7, r1",
		"mtcr 9, r1",
		"mtcr 11, r1",
		"MTCR Rs, CR_RAM_SIZE_BYTES",
		"MTCR CR_RAM_SIZE_BYTES, Rs",
		"the full PTE",
		"cached PTE is used",
	} {
		if strings.Contains(doc, forbidden) {
			t.Fatalf("IE64_ISA.md still contains stale CR/TLB wording: %s", forbidden)
		}
	}
}

func TestSDKCompanionDocs_IE64JumpAndLOnlyQuickReferenceSyntax(t *testing.T) {
	doc := readAuditFile(t, "sdk/docs/IE64_ISA.md")
	asm := readAuditFile(t, "assembler/ie64asm.go")
	for _, needle := range []string{
		"instr, err = a.asmLOnlyALU2(OP64_CLZ",
		"instr, err = a.asmLOnlyALU2(OP64_CTZ",
		"instr, err = a.asmLOnlyALU2(OP64_POPCNT",
		"instr, err = a.asmLOnlyALU2(OP64_BSWAP",
	} {
		if !strings.Contains(asm, needle) {
			t.Fatalf("IE64 assembler .l-only source changed; review quick reference syntax: %s", needle)
		}
	}
	if !strings.Contains(doc, "#### 103. JMP - `jmp (Rs)` / `jmp disp(Rs)`") {
		t.Fatal("IE64 JMP heading has broken Markdown formatting")
	}
	quick := markdownSection(t, doc, "### A.1 Instruction Set Summary", "### A.2 Machine Opcode Encoding Map")
	for _, required := range []string{
		"| CLZ | Shift | `CLZ.l Rd, Rs` |",
		"| CTZ | Shift | `CTZ.l Rd, Rs` |",
		"| POPCNT | Shift | `POPCNT.l Rd, Rs` |",
		"| BSWAP | Shift | `BSWAP.l Rd, Rs` |",
	} {
		if !strings.Contains(quick, required) {
			t.Fatalf("IE64 quick reference missing .l-only syntax: %s", required)
		}
	}
	for _, forbidden := range []string{
		"| CLZ | Shift | `CLZ Rd, Rs` |",
		"| CTZ | Shift | `CTZ Rd, Rs` |",
		"| POPCNT | Shift | `POPCNT Rd, Rs` |",
		"| BSWAP | Shift | `BSWAP Rd, Rs` |",
	} {
		if strings.Contains(quick, forbidden) {
			t.Fatalf("IE64 quick reference contains suffixless .l-only syntax: %s", forbidden)
		}
	}
}

func TestSDKCompanionDocs_IE64LOnlyBitInstructionsDocumentIgnoredSizeField(t *testing.T) {
	doc := readAuditFile(t, "sdk/docs/IE64_ISA.md")
	source := readAuditFile(t, "cpu_ie64.go")
	for _, needle := range []string{
		"case OP_CLZ:",
		"bits.LeadingZeros32(uint32(cpu.regs[rs]))",
		"case OP_CTZ:",
		"bits.TrailingZeros32(uint32(cpu.regs[rs]))",
		"case OP_POPCNT:",
		"bits.OnesCount32(uint32(cpu.regs[rs]))",
		"case OP_BSWAP:",
		"bits.ReverseBytes32(uint32(cpu.regs[rs]))",
	} {
		if !strings.Contains(source, needle) {
			t.Fatalf("cpu_ie64.go low-32-bit instruction source changed; review IE64 field documentation: %s", needle)
		}
	}
	for _, tc := range []struct {
		heading string
		next    string
	}{
		{"#### 41. CLZ - clz.l Rd, Rs", "#### 42. SEXT - sext.s Rd, Rs"},
		{"#### 45. CTZ - ctz.l Rd, Rs", "#### 46. POPCNT - popcnt.l Rd, Rs"},
		{"#### 46. POPCNT - popcnt.l Rd, Rs", "#### 47. BSWAP - bswap.l Rd, Rs"},
		{"#### 47. BSWAP - bswap.l Rd, Rs", "### 4.6 Floating Point (FPU)"},
	} {
		entry := markdownSection(t, doc, tc.heading, tc.next)
		required := "Byte 1 bits 2-1 are reserved by this instruction and ignored by the processor; the operation always uses the low 32 bits of `Rs`."
		if !strings.Contains(entry, required) {
			t.Fatalf("%s does not document ignored size bits for fixed low-32-bit execution", tc.heading)
		}
		if strings.Contains(entry, "bits 2-1 select size (L)") {
			t.Fatalf("%s still assigns architectural meaning to ignored size bits", tc.heading)
		}
	}
}

func TestSDKCompanionDocs_IE64MMUPrivilegeInstructionsUseFullSchema(t *testing.T) {
	doc := readAuditFile(t, "sdk/docs/IE64_ISA.md")
	source := readAuditFile(t, "cpu_ie64.go")
	for _, needle := range []string{
		"case OP_MTCR:",
		"case OP_MFCR:",
		"case OP_ERET:",
		"case OP_TLBFLUSH:",
		"case OP_TLBINVAL:",
		"case OP_SYSCALL:",
		"case OP_SMODE:",
		"case OP_SUAEN:",
		"case OP_SUADIS:",
		"cpu.trapFault(FAULT_ILLEGAL_INSTRUCTION, 0)",
		"vpn := cpu.regs[rs] >> MMU_PAGE_SHIFT",
		"cpu.trapSyscall(imm32)",
	} {
		if !strings.Contains(source, needle) {
			t.Fatalf("cpu_ie64.go MMU/privilege source changed; review IE64 ISA docs: %s", needle)
		}
	}
	section := markdownSection(t, doc, "### 4.10 MMU, Privilege, and Atomic Instructions", "## 5. Architectural Instruction Idioms")
	if strings.Contains(section, "| Mnemonic | Opcode | Syntax | Operation | Privilege |") ||
		strings.Contains(section, "MTCR:     [0xE6]") {
		t.Fatal("IE64 MMU/privilege instructions are documented as a summary table instead of full instruction entries")
	}
	for _, tc := range []struct {
		heading string
		next    string
		want    []string
	}{
		{
			"#### 115. MTCR - mtcr CRn, Rs",
			"#### 116. MFCR - mfcr Rd, CRn",
			[]string{"**Operation:**", "**Assembler Syntax:**", "**Attributes:**", "**Description:**", "assigned control registers are listed in section 8.1.1", "Supervisor-mode writes to unassigned control-register numbers have no architectural effect.", "**Condition Codes:**", "**Instruction Format:**", "**Instruction Fields:**", "**Exceptions:** User-mode execution raises `FAULT_PRIV` (cause 5). Writing `CR15` (`RAM_SIZE_BYTES`) raises `FAULT_ILLEGAL_INSTRUCTION` (cause 11).", "Encodings `CR16` through `CR31` are reserved; `MTCR` to those encodings is ignored after the privilege check succeeds.", "**Notes:**"},
		},
		{
			"#### 116. MFCR - mfcr Rd, CRn",
			"#### 117. ERET - eret",
			[]string{"**Operation:**", "Reading `CR6` is permitted in user mode.", "Byte 2 bits 7-3 hold the control-register number `CRn`", "Supervisor-mode reads of unassigned control-register numbers return zero.", "Encodings `CR16` through `CR31` are reserved; `MFCR` from those encodings returns zero after the privilege check succeeds."},
		},
		{
			"#### 117. ERET - eret",
			"#### 118. TLBFLUSH - tlbflush",
			[]string{"**Operation:** `PC = CR3`; restore the saved privilege and trap-frame state.", "Bytes 1-7 are reserved by this instruction and ignored by the processor.", "User-mode execution raises `FAULT_PRIV` (cause 5)."},
		},
		{
			"#### 119. TLBINVAL - tlbinval Rs",
			"#### 120. SYSCALL - syscall #imm32",
			[]string{"**Operation:** Invalidate the TLB entry selected by `Rs >> 12`.", "Byte 2 bits 7-3 select source register `Rs`", "`Rs` contains an address within the affected virtual page"},
		},
		{
			"#### 120. SYSCALL - syscall #imm32",
			"#### 121. SMODE - smode Rd",
			[]string{"**Operation:** `CR1 = imm32`; `CR2 = 6`; `CR3 = PC + 8`; `PC = CR4`.", "Bytes 4-7 hold unsigned `imm32` in little-endian order.", "`SYSCALL` is itself a trap source and records `FAULT_SYSCALL` (cause 6)."},
		},
		{
			"#### 128. SUAEN - suaen",
			"#### 129. SUADIS - suadis",
			[]string{"**Operation:** `SUA = 1`.", "Bytes 1-7 are reserved by this instruction and ignored by the processor.", "User-mode execution raises `FAULT_PRIV` (cause 5)."},
		},
		{
			"#### 129. SUADIS - suadis",
			"",
			[]string{"**Operation:** `SUA = 0`.", "Clearing an already-clear latch leaves architectural state unchanged.", "User-mode execution raises `FAULT_PRIV` (cause 5)."},
		},
	} {
		entry := markdownSection(t, section, tc.heading, tc.next)
		for _, want := range tc.want {
			if !strings.Contains(entry, want) {
				t.Fatalf("%s missing source-backed schema text: %s", tc.heading, want)
			}
		}
	}
}

func TestSDKCompanionDocs_IE64ReservedControlRegisterEncodings(t *testing.T) {
	doc := readAuditFile(t, "sdk/docs/IE64_ISA.md")
	source := readAuditFile(t, "cpu_ie64.go")
	for _, needle := range []string{
		"switch crIdx",
		"case CR_RAM_SIZE_BYTES:",
		"cpu.trapFault(FAULT_ILLEGAL_INSTRUCTION, 0)",
		"var val uint64",
		"if rd != 0 {",
	} {
		if !strings.Contains(source, needle) {
			t.Fatalf("cpu_ie64.go control-register source changed; review reserved CR docs: %s", needle)
		}
	}
	table := markdownSection(t, doc, "### 8.1.1 Control Register Numbers", "### 8.2 Initial State After Reset")
	normalizedTable := strings.Join(strings.Fields(table), " ")
	for _, needle := range []string{
		"| Reserved | CR16-CR31 |",
	} {
		if !strings.Contains(table, needle) {
			t.Fatalf("IE64_ISA.md missing reserved control-register encoding rule: %s", needle)
		}
	}
	for _, needle := range []string{
		"`MFCR` from a reserved encoding returns zero",
		"`MTCR` to a reserved encoding has no effect",
		"only `MFCR CR6` is user-readable",
	} {
		if !strings.Contains(normalizedTable, needle) {
			t.Fatalf("IE64_ISA.md missing reserved control-register encoding rule: %s", needle)
		}
	}
}

func TestSDKCompanionDocs_IE64MOVTFieldsMatchSource(t *testing.T) {
	doc := readAuditFile(t, "sdk/docs/IE64_ISA.md")
	source := readAuditFile(t, "cpu_ie64.go")
	for _, needle := range []string{
		"case OP_MOVT:",
		"cpu.regs[rd] = (cpu.regs[rd] & 0x00000000FFFFFFFF) | (uint64(imm32) << 32)",
	} {
		if !strings.Contains(source, needle) {
			t.Fatalf("cpu_ie64.go MOVT source changed; review IE64 ISA field documentation: %s", needle)
		}
	}
	movt := markdownSection(t, doc, "#### 3. MOVT - movt Rd, #imm", "#### 4. MOVEQ - moveq Rd, #imm")
	for _, required := range []string{
		"Byte 0 holds opcode `0x02`.",
		"Byte 1 bits 7-3 select destination register `Rd`",
		"byte 1 bits 2-0 are reserved by this instruction and ignored by the processor",
		"Bytes 2-3 are reserved by this instruction and ignored by the processor",
		"Bytes 4-7 hold unsigned `imm32` in little-endian order",
		"this field supplies the new high 32 bits of `Rd`",
	} {
		if !strings.Contains(movt, required) {
			t.Fatalf("IE64 MOVT entry missing source-backed field detail: %s", required)
		}
	}
	for _, forbidden := range []string{
		"immediate/register selector",
		"`Rs`",
		"`Rt`",
		"displacement",
		"where applicable",
	} {
		if strings.Contains(movt, forbidden) {
			t.Fatalf("IE64 MOVT entry still contains generic or misleading field prose: %q", forbidden)
		}
	}
}

func TestSDKCompanionDocs_IE64DIVSDocumentsFullWidthSignedOperands(t *testing.T) {
	doc := readAuditFile(t, "sdk/docs/IE64_ISA.md")
	source := readAuditFile(t, "cpu_ie64.go")
	for _, needle := range []string{
		"maskToSize(uint64(int64(cpu.regs[rs])/int64(operand3)), size)",
		"a := signExtendToInt64(cpu.regs[rs], size)",
		"b := signExtendToInt64(operand3, size)",
	} {
		if !strings.Contains(source, needle) {
			t.Fatalf("cpu_ie64.go DIVS/MODS source changed; review signed division docs: %s", needle)
		}
	}
	divs := markdownSection(t, doc, "#### 20. DIVS - divs.s Rd, Rs, Rt", "#### 21. DIVS - divs.s Rd, Rs, #imm")
	divsImm := markdownSection(t, doc, "#### 21. DIVS - divs.s Rd, Rs, #imm", "#### 22. MOD - mod.s Rd, Rs, Rt")
	for heading, entry := range map[string]string{
		"DIVS register":  divs,
		"DIVS immediate": divsImm,
	} {
		if !strings.Contains(entry, "For `.b`, `.w`, and `.l`, the operands are not sign-extended from the selected width before division.") {
			t.Fatalf("%s entry does not document full-width signed division before result masking", heading)
		}
	}
	if !strings.Contains(divsImm, "`MODS` differs: it sign-extends both operands to the selected width before taking the remainder.") {
		t.Fatal("DIVS immediate note does not distinguish MODS selected-width sign extension")
	}
}

func TestSDKCompanionDocs_IE64IntegerDivisionDocumentsZeroDivisorResult(t *testing.T) {
	doc := readAuditFile(t, "sdk/docs/IE64_ISA.md")
	source := readAuditFile(t, "cpu_ie64.go")
	for _, needle := range []string{
		"case OP_DIVU:",
		"case OP_DIVS:",
		"case OP_MOD64:",
		"case OP_MODS:",
		"if operand3 == 0 {",
		"if b == 0 {",
		"cpu.regs[rd] = 0",
	} {
		if !strings.Contains(source, needle) {
			t.Fatalf("cpu_ie64.go integer division/remainder source changed; review zero-divisor docs: %s", needle)
		}
	}
	entries := map[string]string{
		"DIVU register":  markdownSection(t, doc, "#### 18. DIVU - divu.s Rd, Rs, Rt", "#### 19. DIVU - divu.s Rd, Rs, #imm"),
		"DIVU immediate": markdownSection(t, doc, "#### 19. DIVU - divu.s Rd, Rs, #imm", "#### 20. DIVS - divs.s Rd, Rs, Rt"),
		"DIVS register":  markdownSection(t, doc, "#### 20. DIVS - divs.s Rd, Rs, Rt", "#### 21. DIVS - divs.s Rd, Rs, #imm"),
		"DIVS immediate": markdownSection(t, doc, "#### 21. DIVS - divs.s Rd, Rs, #imm", "#### 22. MOD - mod.s Rd, Rs, Rt"),
		"MOD register":   markdownSection(t, doc, "#### 22. MOD - mod.s Rd, Rs, Rt", "#### 23. MOD - mod.s Rd, Rs, #imm"),
		"MOD immediate":  markdownSection(t, doc, "#### 23. MOD - mod.s Rd, Rs, #imm", "#### 24. NEG - neg.s Rd, Rs"),
		"MODS reg/imm":   markdownSection(t, doc, "#### 25. MODS - mods.s Rd, Rs, Rt/#imm", "#### 26. MULHU - mulhu Rd, Rs, Rt/#imm"),
	}
	if got := strings.Count(doc, "If the divisor is zero and `Rd` is not `R0`, `Rd` receives zero."); got != len(entries) {
		t.Fatalf("IE64 zero-divisor note appears %d times, want exactly %d division/remainder entries", got, len(entries))
	}
	for heading, entry := range entries {
		if !strings.Contains(entry, "If the divisor is zero and `Rd` is not `R0`, `Rd` receives zero.") {
			t.Fatalf("%s entry does not document source-backed zero-divisor result", heading)
		}
		if !strings.Contains(entry, "**Operation:**") || !strings.Contains(entry, "If ") || !strings.Contains(entry, "`Rd = 0`") {
			t.Fatalf("%s Operation field is not self-contained about the zero-divisor result", heading)
		}
		if !strings.Contains(entry, "**Exceptions:** None.") {
			t.Fatalf("%s entry should keep source-backed no-exception wording for zero divisors", heading)
		}
	}
}

func TestSDKCompanionDocs_IE64MULSImmediateDocumentsZeroExtendedOperand(t *testing.T) {
	doc := readAuditFile(t, "sdk/docs/IE64_ISA.md")
	source := readAuditFile(t, "cpu_ie64.go")
	for _, needle := range []string{
		"operand3 = uint64(imm32)",
		"case OP_MULS:",
		"int64(cpu.regs[rs])*int64(operand3)",
	} {
		if !strings.Contains(source, needle) {
			t.Fatalf("cpu_ie64.go MULS immediate source changed; review IE64 ISA wording: %s", needle)
		}
	}
	entry := markdownSection(t, doc, "#### 17. MULS - muls.s Rd, Rs, #imm", "#### 18. DIVU - divu.s Rd, Rs, Rt")
	for _, required := range []string{
		"zero-extended `imm32` converted to `int64`",
		"selected size only when masking the product written to `Rd`",
	} {
		if !strings.Contains(entry, required) {
			t.Fatalf("MULS immediate entry omits source-backed signedness detail: %s", required)
		}
	}
	if strings.Contains(entry, "by the immediate as signed integers") {
		t.Fatal("MULS immediate entry still implies the imm32 field is sign-extended before multiplication")
	}
}

func TestSDKCompanionDocs_IE64MULHSImmediateDocumentsZeroExtendedOperand(t *testing.T) {
	doc := readAuditFile(t, "sdk/docs/IE64_ISA.md")
	source := readAuditFile(t, "cpu_ie64.go")
	for _, needle := range []string{
		"operand3 = uint64(imm32)",
		"case OP_MULHS:",
		"mulHighSigned(int64(cpu.regs[rs]), int64(operand3))",
	} {
		if !strings.Contains(source, needle) {
			t.Fatalf("cpu_ie64.go MULHS immediate source changed; review IE64 ISA wording: %s", needle)
		}
	}
	entry := markdownSection(t, doc, "#### 27. MULHS - mulhs Rd, Rs, Rt/#imm", "**Immediate operands**")
	for _, required := range []string{
		"Register operands use the full 64-bit value in `Rt`",
		"immediate operands use zero-extended `imm32` converted to `int64`",
		"upper 64 bits of the signed 128-bit product",
	} {
		if !strings.Contains(entry, required) {
			t.Fatalf("MULHS entry omits source-backed immediate operand detail: %s", required)
		}
	}
	if strings.Contains(entry, "by the third operand as signed integers") {
		t.Fatal("MULHS entry still hides the zero-extended imm32 signed conversion rule")
	}
}

func TestSDKCompanionDocs_IEMonMemoryMapUsesSharedMachineMemoryWording(t *testing.T) {
	doc := readAuditFile(t, "sdk/docs/iemon.md")
	source := readAuditFile(t, "cpu_ie32.go")
	bus := readAuditFile(t, "machine_bus.go")
	for _, needle := range []string{
		"memory []byte       // Cached reference to bus.memory (shared, not private)",
		"memory:     bus.GetMemory(), // Use shared bus memory",
	} {
		if !strings.Contains(source, needle) {
			t.Fatalf("cpu_ie32.go shared-memory source changed; review IEMon memory-map wording: %s", needle)
		}
	}
	if !strings.Contains(bus, "return bus.memory") {
		t.Fatal("machine_bus.go GetMemory source changed; review IEMon memory-map wording")
	}
	if strings.Contains(doc, "host-visible RAM") {
		t.Fatal("iemon.md still describes CPU regions as host-visible RAM instead of shared machine RAM")
	}
	for _, needle := range []string{
		"Standard shared machine RAM, stack, VRAM, and `0xF0000-0xFFFFF` MMIO regions",
		"The regions are views of the shared MachineBus memory map",
	} {
		if !strings.Contains(doc, needle) {
			t.Fatalf("iemon.md missing shared-memory wording: %s", needle)
		}
	}
}

func TestSDKCompanionDocs_ISAInstructionReferencesUsePerInstructionSchema(t *testing.T) {
	for _, path := range []string{"sdk/docs/IE64_ISA.md", "sdk/docs/IE32_ISA.md"} {
		doc := readAuditFile(t, path)
		section := markdownSection(t, doc, "## 4. Complete Instruction Reference", "## 5.")
		if !strings.Contains(section, "### 4.0 Instruction Entry Schema") {
			t.Fatalf("%s instruction reference no longer declares the per-instruction entry schema", path)
		}
		for _, required := range []string{
			"**Operation:**",
			"**Assembler Syntax:**",
			"**Attributes:**",
			"**Description:**",
			"**Condition Codes:**",
			"**Instruction Format:**",
			"**Instruction Fields:**",
			"**Exceptions:**",
			"**Notes:**",
		} {
			if !strings.Contains(section, required) {
				t.Fatalf("%s instruction reference is missing schema field %s", path, required)
			}
		}
		for _, forbidden := range []string{
			"compact grouped opcode tables",
			"| Mnemonic | Opcode | Syntax",
			"| Instruction | Opcode | Syntax",
			"**Instruction Fields:** opcode =",
			"syntax = `",
		} {
			if strings.Contains(section, forbidden) {
				t.Fatalf("%s section 4 still uses compact opcode-table form: %s", path, forbidden)
			}
		}
	}
}

func TestSDKCompanionDocs_IE64MemoryEntryExceptionsAreDirectionSpecific(t *testing.T) {
	doc := readAuditFile(t, "sdk/docs/IE64_ISA.md")
	source := readAuditFile(t, "mmu_ie64.go")
	for _, needle := range []string{
		"FAULT_READ_DENIED",
		"FAULT_WRITE_DENIED",
		"FAULT_NOT_PRESENT",
		"FAULT_SKAC",
	} {
		if !strings.Contains(source, needle) {
			t.Fatalf("mmu_ie64.go fault source changed; review IE64 memory exception documentation: %s", needle)
		}
	}
	for _, tc := range []struct {
		heading string
		next    string
		want    []string
		forbid  []string
	}{
		{
			"#### 6. LOAD - load.s Rd, (Rs)",
			"#### 7. LOAD - load.s Rd, disp(Rs)",
			[]string{"reads can trap with cause 0", "cause 1 (`FAULT_READ_DENIED`)", "After optional MMU translation, physical backing is checked; a physical read outside implemented CPU-visible memory raises cause 0."},
			[]string{"writes can trap", "FAULT_WRITE_DENIED", "translated physical"},
		},
		{
			"#### 7. LOAD - load.s Rd, disp(Rs)",
			"#### 8. STORE - store.s Rd, (Rs)",
			[]string{"reads can trap with cause 0", "cause 1 (`FAULT_READ_DENIED`)", "After optional MMU translation, physical backing is checked; a physical read outside implemented CPU-visible memory raises cause 0."},
			[]string{"writes can trap", "FAULT_WRITE_DENIED", "translated physical"},
		},
		{
			"#### 8. STORE - store.s Rd, (Rs)",
			"#### 9. STORE - store.s Rd, disp(Rs)",
			[]string{"writes can trap with cause 0", "cause 2 (`FAULT_WRITE_DENIED`)", "After optional MMU translation, physical backing is checked; a physical write outside implemented CPU-visible memory raises cause 0."},
			[]string{"reads can trap", "FAULT_READ_DENIED", "translated physical"},
		},
		{
			"#### 9. STORE - store.s Rd, disp(Rs)",
			"### 4.3 Arithmetic",
			[]string{"writes can trap with cause 0", "cause 2 (`FAULT_WRITE_DENIED`)", "After optional MMU translation, physical backing is checked; a physical write outside implemented CPU-visible memory raises cause 0."},
			[]string{"reads can trap", "FAULT_READ_DENIED", "translated physical"},
		},
	} {
		entry := markdownSection(t, doc, tc.heading, tc.next)
		for _, want := range tc.want {
			if !strings.Contains(entry, want) {
				t.Fatalf("IE64 memory entry %s missing direction-specific exception text: %s", tc.heading, want)
			}
		}
		for _, forbid := range tc.forbid {
			if strings.Contains(entry, forbid) {
				t.Fatalf("IE64 memory entry %s contains wrong-direction exception text: %s", tc.heading, forbid)
			}
		}
	}
}

func TestSDKCompanionDocs_ISAInstructionEntriesRejectGeneratedPlaceholders(t *testing.T) {
	for _, path := range []string{"sdk/docs/IE64_ISA.md", "sdk/docs/IE32_ISA.md"} {
		doc := readAuditFile(t, path)
		section := markdownSection(t, doc, "## 4. Complete Instruction Reference", "## 5.")
		for _, forbidden := range []string{
			"See section 3 for field definitions",
			"See instruction format and fields",
			"according to the operand rules for this instruction class",
			"See the surrounding subsection for shared semantics",
			"**Operation:** MOVE (reg).",
			"**Operation:** MOVE (imm).",
			"**Operation:** ADD.",
			"**Operation:** BRA.",
			"**Description:** Executes `--`",
			"**Description:** Executes `",
			"**Description:** Performs `",
			"**Description:** Architectural effect:",
			"The processor updates architectural state as specified by",
			"The processor applies `",
			"None specific to this entry beyond the architectural faults described for its instruction class",
			"using the operands and fields named in this entry",
			"where applicable",
			"when the form uses",
			"first encoded register field",
			"operand32 or branch target",
		} {
			if strings.Contains(section, forbidden) {
				t.Fatalf("%s still contains generated placeholder or boilerplate entry text: %s", path, forbidden)
			}
		}
	}
}

func TestSDKCompanionDocs_ISAInstructionFieldProseIsConcrete(t *testing.T) {
	for _, path := range []string{"sdk/docs/IE64_ISA.md", "sdk/docs/IE32_ISA.md"} {
		doc := readAuditFile(t, path)
		for lineNo, line := range strings.Split(doc, "\n") {
			if !strings.Contains(line, "**Instruction Fields:**") {
				continue
			}
			for _, forbidden := range []string{
				"where applicable",
				"when the form uses",
				"first encoded register field",
				"operand32 or branch target",
				"immediate/register selector",
			} {
				if strings.Contains(line, forbidden) {
					t.Fatalf("%s:%d contains generic instruction-field prose: %s", path, lineNo+1, forbidden)
				}
			}
		}
	}
}

func TestSDKCompanionDocs_IE32TimerDividerIsArchitecturalValue(t *testing.T) {
	doc := readAuditFile(t, "sdk/docs/IE32_ISA.md")
	source := readAuditFile(t, "audio_chip.go")
	if !strings.Contains(source, "SAMPLE_RATE = 44100") {
		t.Fatal("audio_chip.go SAMPLE_RATE changed; review IE32 architectural timer divider documentation")
	}
	section := markdownSection(t, doc, "## 9. Timer and Interrupt Model", "## 10. Architectural Caveats")
	if strings.Contains(section, "SAMPLE_RATE") {
		t.Fatal("IE32_ISA.md leaks the implementation SAMPLE_RATE symbol instead of documenting the architectural divider value")
	}
	for _, needle := range []string{
		"`IE32_TIMER_DIVIDER` | 44,100",
		"decoded instruction step, after the instruction word and operand fields have",
		"prescaler reaches `IE32_TIMER_DIVIDER`",
		"Timer prescaler reaches `IE32_TIMER_DIVIDER` and countdown is non-zero",
	} {
		if !strings.Contains(section, needle) {
			t.Fatalf("IE32_ISA.md does not document the architectural timer divider constant: %s", needle)
		}
	}
	if strings.Contains(section, "Executed-instruction divider") ||
		strings.Contains(section, "once per\nexecuted instruction") ||
		strings.Contains(section, "Enabled timer executes one instruction") {
		t.Fatal("IE32_ISA.md timer section uses over-strong executed-instruction wording")
	}
}

func TestSDKCompanionDocs_IE32StackLimitPredicatesMatchSource(t *testing.T) {
	doc := readAuditFile(t, "sdk/docs/IE32_ISA.md")
	source := readAuditFile(t, "cpu_ie32.go")
	for _, needle := range []string{
		"func (cpu *CPU) Push(value uint32) bool",
		"if cpu.SP <= STACK_BOTTOM",
		"if cpu.SP < STACK_BOTTOM+WORD_SIZE",
		"cpu.handleInterrupt()",
	} {
		if !strings.Contains(source, needle) {
			t.Fatalf("cpu_ie32.go stack-bound source changed; review IE32 stack documentation: %s", needle)
		}
	}
	stack := markdownSection(t, doc, "## 8. Stack", "## 9. Timer and Interrupt Model")
	normalizedStack := strings.Join(strings.Fields(stack), " ")
	for _, required := range []string{
		"| Inlined `PUSH` and `JSR` instruction execution | `SP < STACK_BOTTOM + 4` before decrement |",
		"| Interrupt entry through the CPU push helper | `SP <= STACK_BOTTOM` before decrement |",
		"CPU stack operations maintain 32-bit word alignment for `SP`; with an aligned `SP`, the inlined instruction and interrupt-entry predicates reject the same lower boundary.",
	} {
		if !strings.Contains(normalizedStack, required) {
			t.Fatalf("IE32_ISA.md missing source-backed stack predicate wording: %s", required)
		}
	}
	if strings.Contains(stack, "| Push / JSR / interrupt entry | `SP < STACK_BOTTOM + 4` before decrement |") {
		t.Fatal("IE32_ISA.md still collapses distinct stack overflow predicates")
	}
}

func TestSDKCompanionDocs_IE32DoesNotInventMemoryFaults(t *testing.T) {
	doc := readAuditFile(t, "sdk/docs/IE32_ISA.md")
	source := readAuditFile(t, "cpu_ie32.go")
	for _, needle := range []string{
		"if addr < IO_REGION_START",
		"unsafe.Pointer(uintptr(cpu.memBase) + uintptr(addr))",
		"cpu.bus.Read32(addr)",
		"cpu.bus.Write32(addr, value)",
		"Division by zero error",
		"Invalid opcode",
	} {
		if !strings.Contains(source, needle) {
			t.Fatalf("cpu_ie32.go memory/stop-condition source changed; review IE32 ISA exception documentation: %s", needle)
		}
	}
	if strings.Contains(doc, "May raise address, permission, alignment, or bus faults") {
		t.Fatal("IE32_ISA.md invents address/permission/alignment/bus fault behaviour not present in source")
	}
	if strings.Contains(doc, "No address, permission, alignment, or bus fault is architecturally raised by this instruction.") {
		t.Fatal("IE32_ISA.md uses awkward negative fault taxonomy instead of omitting non-existent fault classes")
	}
	for _, tc := range []struct {
		heading string
		next    string
		want    string
		forbid  string
	}{
		{"#### 1. LOAD - LOAD R, operand", "#### 2. LDA - LDA operand", "**Exceptions:** None.", "resolved divisor is zero"},
		{"#### 2. LDA - LDA operand", "#### 3. LDX - LDX operand", "**Exceptions:** None.", "resolved divisor is zero"},
		{"#### 3. LDX - LDX operand", "#### 4. LDY - LDY operand", "**Exceptions:** None.", "resolved divisor is zero"},
		{"#### 4. LDY - LDY operand", "#### 5. LDZ - LDZ operand", "**Exceptions:** None.", "resolved divisor is zero"},
		{"#### 38. DIV - DIV R, operand", "#### 39. MOD - MOD R, operand", "**Exceptions:** If the resolved divisor is zero, the CPU enters the stopped processor state.", ""},
		{"#### 39. MOD - MOD R, operand", "#### 40. INC - INC operand", "**Exceptions:** If the resolved divisor is zero, the CPU enters the stopped processor state.", ""},
	} {
		entry := markdownSection(t, doc, tc.heading, tc.next)
		if !strings.Contains(entry, tc.want) {
			t.Fatalf("IE32_ISA.md %s has wrong exception text; want %q", tc.heading, tc.want)
		}
		if tc.forbid != "" && strings.Contains(entry, tc.forbid) {
			t.Fatalf("IE32_ISA.md %s inherited unrelated divisor exception text", tc.heading)
		}
	}
	for _, needle := range []string{
		"**Exceptions:** If the resolved divisor is zero, the CPU enters the stopped processor state.",
		"**Exceptions:** Stack overflow enters the stopped processor state.",
		"**Exceptions:** Stack underflow enters the stopped processor state.",
		"Opcodes not listed above are reserved. Executing a reserved opcode enters the",
	} {
		if !strings.Contains(doc, needle) {
			t.Fatalf("IE32_ISA.md missing source-backed stop condition: %s", needle)
		}
	}
}

func TestSDKCompanionDocs_IE32ArithmeticFieldsDoNotMentionBranchTargets(t *testing.T) {
	doc := readAuditFile(t, "sdk/docs/IE32_ISA.md")
	source := readAuditFile(t, "cpu_ie32.go")
	for _, needle := range []string{
		"case DIV:",
		"case MOD:",
		"if resolvedOperand == 0",
		"*cpu.regs[reg&REG_INDEX_MASK] /= resolvedOperand",
		"*cpu.regs[reg&REG_INDEX_MASK] %= resolvedOperand",
	} {
		if !strings.Contains(source, needle) {
			t.Fatalf("cpu_ie32.go arithmetic source changed; review IE32 ISA field documentation: %s", needle)
		}
	}
	for _, tc := range []struct {
		name   string
		start  string
		end    string
		opcode string
	}{
		{"DIV", "#### 38. DIV - DIV R, operand", "#### 39. MOD - MOD R, operand", "0x15"},
		{"MOD", "#### 39. MOD - MOD R, operand", "#### 40. INC - INC operand", "0x16"},
	} {
		section := markdownSection(t, doc, tc.start, tc.end)
		for _, required := range []string{
			"Byte 0 holds opcode `" + tc.opcode + "`.",
			"Byte 1 selects the destination/dividend register `R`.",
			"Byte 2 selects the addressing mode used to resolve the divisor operand.",
			"Byte 3 is reserved by this instruction and ignored by the processor.",
			"Bytes 4-7 hold `operand32` in little-endian order; `operand32` is interpreted according to byte 2.",
		} {
			if !strings.Contains(section, required) {
				t.Fatalf("IE32 %s entry missing concrete field detail: %s", tc.name, required)
			}
		}
		if strings.Contains(section, "branch target") || strings.Contains(section, "when the instruction names a register") {
			t.Fatalf("IE32 %s entry still contains generic field prose", tc.name)
		}
	}
}

func TestSDKCompanionDocs_IE32FixedFormInstructionsDocumentReservedBytes(t *testing.T) {
	doc := readAuditFile(t, "sdk/docs/IE32_ISA.md")
	source := readAuditFile(t, "cpu_ie32.go")
	for _, needle := range []string{
		"case NOP:",
		"cpu.PC += INSTRUCTION_SIZE",
		"case HALT:",
		"cpu.running.Store(false)",
	} {
		if !strings.Contains(source, needle) {
			t.Fatalf("cpu_ie32.go NOP/HALT source changed; review IE32 ISA documentation: %s", needle)
		}
	}
	for _, tc := range []struct {
		heading string
		next    string
		opcode  string
	}{
		{"#### 63. NOP - NOP", "#### 64. HALT - HALT", "0xEE"},
		{"#### 64. HALT - HALT", "---\n## 5. Addressing Modes", "0xFF"},
	} {
		entry := markdownSection(t, doc, tc.heading, tc.next)
		want := "Byte 0 holds opcode `" + tc.opcode + "`. Bytes 1-7 are reserved by this instruction and ignored by the processor."
		if !strings.Contains(entry, want) {
			t.Fatalf("%s does not document fixed-form reserved bytes", tc.heading)
		}
		if strings.Contains(entry, "register field when the instruction names a register") ||
			strings.Contains(entry, "addressing-mode code when the instruction has an operand") {
			t.Fatalf("%s still uses generic operand-field prose for a fixed-form instruction", tc.heading)
		}
	}
}

func TestSDKCompanionDocs_IE32MemoryIndirectStoreSemantics(t *testing.T) {
	doc := readAuditFile(t, "sdk/docs/IE32_ISA.md")
	source := readAuditFile(t, "cpu_ie32.go")
	for _, needle := range []string{
		"case ADDR_MEM_IND:",
		"return cpu.Read32(operand)",
		"} else if addrMode == ADDR_MEM_IND {",
		"addr := cpu.Read32(operand)",
		"cpu.Write32(addr, value)",
	} {
		if !strings.Contains(source, needle) {
			t.Fatalf("cpu_ie32.go memory-indirect addressing source changed; review IE32 ISA documentation: %s", needle)
		}
	}
	intro := markdownSection(t, doc, "| Condition Codes |", "### 4.1 Data Movement")
	normalizedIntro := strings.Join(strings.Fields(intro), " ")
	for _, needle := range []string{
		"For memory-indirect store encodings, the CPU first reads a 32-bit pointer from `operand32`, then writes to the address contained in that pointer",
	} {
		if !strings.Contains(normalizedIntro, needle) {
			t.Fatalf("IE32_ISA.md missing source-backed memory-indirect store semantics: %s", needle)
		}
	}
	if strings.Contains(intro, "section 10.3") {
		t.Fatal("IE32_ISA.md still points memory-indirect store semantics at the wrong section")
	}
}

func TestSDKCompanionDocs_IE32ISADoesNotDefineLoaderContract(t *testing.T) {
	doc := readAuditFile(t, "sdk/docs/IE32_ISA.md")
	for _, forbidden := range []string{
		"Programme loading",
		"programme loading",
		"programme-load",
		"program loading",
		"program-load",
		"loader contract",
		"load window",
	} {
		if strings.Contains(doc, forbidden) {
			t.Fatalf("IE32_ISA.md contains loader/tooling contract language: %q", forbidden)
		}
	}
	for _, required := range []string{
		"`0x01000` | `PROG_START`. Reset value loaded into `PC`.",
		"`0x9F000` | `STACK_START`. Initial `SP`.",
		"CPU load, store, stack, and interrupt-vector accesses transfer 32-bit",
	} {
		if !strings.Contains(doc, required) {
			t.Fatalf("IE32_ISA.md missing CPU architectural convention: %s", required)
		}
	}
}

func TestSDKCompanionDocs_ArchitectureCompositorAlphaMatchesSource(t *testing.T) {
	doc := readAuditFile(t, "sdk/docs/architecture.md")
	source := readAuditFile(t, "video_compositor.go")
	regression := readAuditFile(t, "video_compositor_test.go")
	for _, needle := range []string{
		"if srcPixel&0xFF000000 != 0",
		"if srcPixel&0x00FFFFFF != 0",
		"return srcPixel | 0xFF000000, true",
	} {
		if !strings.Contains(source, needle) {
			t.Fatalf("video_compositor.go opacity rule changed; review architecture.md compositor alpha text: %s", needle)
		}
	}
	for _, needle := range []string{
		"TestCompositorOpaquePixelTreatsRGBWithZeroAlphaAsOpaque",
		"0x00332211",
		"0xFF332211",
		"0x00000000",
	} {
		if !strings.Contains(regression, needle) {
			t.Fatalf("video compositor regression test changed; review architecture.md compositor alpha text: %s", needle)
		}
	}
	section := markdownSection(t, doc, "### Video Compositor", "### Triple-Buffer Protocol")
	for _, required := range []string{
		"All-zero frame pixels are transparent",
		"any nonzero alpha or RGB value is opaque",
		"zero-alpha nonzero-RGB pixels are promoted to opaque `0xFFRRGGBB`",
	} {
		if !strings.Contains(section, required) {
			t.Fatalf("architecture.md compositor alpha rule omits source-backed behavior: %s", required)
		}
	}
	if strings.Contains(section, "alpha 0 is transparent") {
		t.Fatal("architecture.md still treats alpha 0 alone as transparent")
	}
}

func TestSDKCompanionDocs_ArchitectureCompositorScaleModeMatchesSource(t *testing.T) {
	doc := readAuditFile(t, "sdk/docs/architecture.md")
	source := readAuditFile(t, "video_compositor.go")
	regression := readAuditFile(t, "video_compositor_test.go")
	for _, needle := range []string{
		"scaleMode:   ScaleStretchFill",
		"if c.scaleMode == ScaleStretchFill",
		"c.scaleMode = ScaleAspectFit",
	} {
		if !strings.Contains(source, needle) {
			t.Fatalf("video_compositor.go scale-mode source changed; review architecture.md compositor scale text: %s", needle)
		}
	}
	for _, needle := range []string{
		"default scale mode = %v, want stretch fill",
		"scale mode = %v, want aspect fit",
	} {
		if !strings.Contains(regression, needle) {
			t.Fatalf("video compositor scale regression test changed; review architecture.md compositor scale text: %s", needle)
		}
	}
	section := markdownSection(t, doc, "### Video Compositor", "### Triple-Buffer Protocol")
	for _, required := range []string{
		"Video compositor default scale mode is stretch-fill; F11 toggles non-16:9 sources to aspect-fit.",
		"Shift+F11` toggles fullscreen/windowed mode",
	} {
		if !strings.Contains(section, required) {
			t.Fatalf("architecture.md compositor scale contract omits source-backed behavior: %s", required)
		}
	}
	for _, forbidden := range []string{
		"Non-16:9 sources are aspect-fit by default",
		"can be stretch-filled with `F11`",
	} {
		if strings.Contains(section, forbidden) {
			t.Fatalf("architecture.md still reverses compositor scale behavior: %s", forbidden)
		}
	}
}

func TestSDKCompanionDocs_IE64AtomicFaultsMatchSource(t *testing.T) {
	doc := readAuditFile(t, "sdk/docs/IE64_ISA.md")
	source := readAuditFile(t, "cpu_ie64.go")
	if !strings.Contains(source, "vaddr&7 != 0") ||
		!strings.Contains(source, "uint32(vaddr) >= IO_REGION_START") ||
		!strings.Contains(source, "FAULT_MISALIGNED") ||
		!strings.Contains(source, "memLen := uint64(len(cpu.memory))") ||
		!strings.Contains(source, "physWide > memLen-8") ||
		!strings.Contains(source, "uintptr(cpu.memBase) + uintptr(addr)") ||
		!strings.Contains(source, "FAULT_NOT_PRESENT") {
		t.Fatal("cpu_ie64.go atomic fault checks changed; review IE64 ISA atomic fault documentation")
	}
	section := markdownSection(t, doc, "### 11.13 Atomic Memory Operations", "### 11.14 Trap-Frame Stack")
	normalizedSection := strings.Join(strings.Fields(section), " ")
	for _, needle := range []string{
		"An unaligned effective address or an address in a non-atomic CPU address aperture raises `FAULT_MISALIGNED` (cause 7).",
		"After optional MMU translation, the physical 8-byte word must lie entirely inside the processor's atomic RAM aperture.",
		"A translated address outside that aperture raises `FAULT_NOT_PRESENT` (cause 0).",
	} {
		if !strings.Contains(normalizedSection, needle) {
			t.Fatalf("IE64_ISA.md atomic fault contract does not match source: %s", needle)
		}
	}
	for _, forbidden := range []string{
		"low I/O-region",
		"ordinary CPU RAM",
		"ordinary CPU RAM domain",
		"CPU-visible writable memory",
		"outside CPU-visible writable memory",
	} {
		if strings.Contains(section, forbidden) {
			t.Fatalf("IE64_ISA.md atomic section still uses platform/bus terminology: %s", forbidden)
		}
	}
}

func TestSDKCompanionDocs_IE64FaultCauseZeroCoversPhysicalAndAtomicBacking(t *testing.T) {
	doc := readAuditFile(t, "sdk/docs/IE64_ISA.md")
	source := readAuditFile(t, "cpu_ie64.go")
	for _, needle := range []string{
		"FAULT_NOT_PRESENT  = 0",
		"page is not present, physical memory is unavailable, or atomic backing is unavailable",
	} {
		if !strings.Contains(source, needle) {
			t.Fatalf("cpu_ie64.go FAULT_NOT_PRESENT source/comment changed; review fault cause table: %s", needle)
		}
	}
	if got := strings.Count(source, "cpu.trapFault(FAULT_NOT_PRESENT, vaddr)"); got < 3 {
		t.Fatalf("expected FAULT_NOT_PRESENT to cover PTE/physical/atomic paths, found %d vaddr trap sites", got)
	}
	section := markdownSection(t, doc, "### 11.8 Fault Cause Codes", "### 11.9 Translation Lookaside Buffer (TLB)")
	for _, required := range []string{
		"Absent PTE mapping or unavailable physical/atomic backing.",
		"Access to a page with P=0",
		"physical memory backing",
		"atomic RAM aperture",
	} {
		if !strings.Contains(section, required) {
			t.Fatalf("IE64 fault cause 0 table omits source-backed not-present condition: %s", required)
		}
	}
	if strings.Contains(section, "| 0 | Page Not Present | Access to a page with P=0. |") {
		t.Fatal("IE64 fault cause 0 row is still too narrow")
	}
}

func TestSDKCompanionDocs_IE64TrapStackResetUsesProcessorManualVoice(t *testing.T) {
	doc := readAuditFile(t, "sdk/docs/IE64_ISA.md")
	source := readAuditFile(t, "cpu_ie64.go")
	for _, needle := range []string{
		"func (cpu *CPU64) Reset()",
		"cpu.trapDepth = 0",
		"cpu.trapStack[i] = trapFrame{}",
	} {
		if !strings.Contains(source, needle) {
			t.Fatalf("cpu_ie64.go reset trap-stack behavior changed; review IE64 ISA reset text: %s", needle)
		}
	}
	section := markdownSection(t, doc, "### 11.14 Trap-Frame Stack", "---")
	for _, required := range []string{
		"Processor reset clears the trap stack to depth 0",
		"clears all trap-frame slots",
		"After reset, no saved trap frame is architecturally visible.",
	} {
		if !strings.Contains(section, required) {
			t.Fatalf("IE64 trap-stack reset text omits processor-manual wording: %s", required)
		}
	}
	for _, forbidden := range []string{
		"Reset()",
		"reused CPU",
		"previous run",
	} {
		if strings.Contains(section, forbidden) {
			t.Fatalf("IE64 trap-stack reset text still leaks implementation lifecycle wording: %s", forbidden)
		}
	}
}

func TestSDKCompanionDocs_IEMonAccessInstrumentationFailsClosed(t *testing.T) {
	doc := readAuditFile(t, "sdk/docs/iemon.md")
	source := readAuditFile(t, "debug_commands.go")
	for _, needle := range []string{
		"if !m.access.Instrumented()",
		"Page guards require CPU/bus access instrumentation; this build has not enabled it yet",
		"Access log requires CPU/bus access instrumentation; this build has not enabled it yet",
		"bfirst requires CPU/bus access instrumentation; this build has not enabled it yet",
	} {
		if !strings.Contains(source, needle) {
			t.Fatalf("debug_commands.go instrumentation fail-closed path changed; review iemon.md: %s", needle)
		}
	}
	start := strings.Index(doc, "## Common Pitfalls")
	if start < 0 {
		t.Fatal("iemon.md missing Common Pitfalls section")
	}
	section := doc[start:]
	for _, required := range []string{
		"When access instrumentation is not enabled",
		"access-backed commands fail closed",
		"instead of advertising partial behaviour",
	} {
		if !strings.Contains(section, required) {
			t.Fatalf("iemon.md access instrumentation note omits current source-backed behavior: %s", required)
		}
	}
	if strings.Contains(section, "future build disables") {
		t.Fatal("iemon.md still describes instrumentation as a speculative future-build condition")
	}
}

func TestSDKCompanionDocs_IE64AtomicsUseInstructionEntrySchema(t *testing.T) {
	doc := readAuditFile(t, "sdk/docs/IE64_ISA.md")
	section := markdownSection(t, doc, "### 4.10 MMU, Privilege, and Atomic Instructions", "## 5. Architectural Instruction Idioms")
	for _, heading := range []string{
		"#### 122. CAS - cas Rd, disp(Rs), Rt",
		"#### 123. XCHG - xchg Rd, disp(Rs), Rt",
		"#### 124. FAA - faa Rd, disp(Rs), Rt",
		"#### 125. FAND - fand Rd, disp(Rs), Rt",
		"#### 126. FOR - for Rd, disp(Rs), Rt",
		"#### 127. FXOR - fxor Rd, disp(Rs), Rt",
	} {
		entry := markdownSection(t, section, heading, nextAtomicHeading(heading))
		for _, required := range []string{
			"**Operation:**",
			"**Assembler Syntax:**",
			"**Attributes:**",
			"**Description:**",
			"**Condition Codes:**",
			"**Instruction Format:**",
			"**Instruction Fields:**",
			"**Exceptions:**",
			"**Notes:**",
		} {
			if !strings.Contains(entry, required) {
				t.Fatalf("IE64 atomic entry %q missing schema field %s", heading, required)
			}
		}
	}
	if strings.Contains(section, "| Mnemonic | Opcode | Syntax | Operation |") {
		t.Fatal("IE64 atomic section still uses grouped instruction table instead of per-instruction entries")
	}
}

func TestSDKCompanionDocs_NoReferencesToUnshippedBooks(t *testing.T) {
	allowed := map[string]bool{
		"IE64_ISA.md":     true,
		"IE32_ISA.md":     true,
		"iemon.md":        true,
		"iescript.md":     true,
		"architecture.md": true,
	}
	for _, path := range sdkAuditDocs {
		doc := readAuditFile(t, path)
		for lineNo, line := range strings.Split(doc, "\n") {
			if strings.Contains(line, ".md") {
				for _, m := range regexp.MustCompile(`[A-Za-z0-9_./-]+\.md`).FindAllString(line, -1) {
					base := m[strings.LastIndex(m, "/")+1:]
					if !allowed[base] {
						t.Fatalf("%s:%d references unshipped document %q", path, lineNo+1, m)
					}
				}
			}
			for _, forbidden := range []string{
				"README",
				"Cookbook",
				"COOKBOOK",
				"PLAN_",
				"MC68000",
			} {
				if strings.Contains(line, forbidden) {
					t.Fatalf("%s:%d references unshipped book/doc marker %q: %s", path, lineNo+1, forbidden, line)
				}
			}
		}
	}
}

func stripMarkdownInlineCode(line string) string {
	var b strings.Builder
	inCode := false
	for i := 0; i < len(line); i++ {
		if line[i] == '`' {
			inCode = !inCode
			continue
		}
		if !inCode {
			b.WriteByte(line[i])
		}
	}
	return b.String()
}

func markdownMentionsCommand(lowerDoc, command string) bool {
	command = strings.ToLower(command)
	if strings.Contains(lowerDoc, "`"+command+"`") {
		return true
	}
	if strings.Contains(lowerDoc, "`"+command+" ") {
		return true
	}
	if strings.Contains(lowerDoc, " "+command+"|") || strings.Contains(lowerDoc, "|"+command+" ") {
		return true
	}
	return false
}

func encodeIE64AuditInstr(opcode byte, rd, size, xbit, rs, rt byte, imm32 uint32) []byte {
	return []byte{
		opcode,
		(rd << 3) | (size << 1) | xbit,
		rs << 3,
		rt << 3,
		byte(imm32),
		byte(imm32 >> 8),
		byte(imm32 >> 16),
		byte(imm32 >> 24),
	}
}

func encodeIE32AuditInstr(opcode, reg, mode byte, operand uint32) []byte {
	return []byte{
		opcode,
		reg,
		mode,
		0,
		byte(operand),
		byte(operand >> 8),
		byte(operand >> 16),
		byte(operand >> 24),
	}
}

func formatAuditBytes(bytes []byte) string {
	parts := make([]string, len(bytes))
	for i, b := range bytes {
		parts[i] = fmt.Sprintf("%02X", b)
	}
	return strings.Join(parts, " ")
}

type auditOpcode struct {
	name  string
	value int
}

func readAuditFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func markdownSection(t *testing.T, doc, startHeading, endHeading string) string {
	t.Helper()
	start := strings.Index(doc, startHeading)
	if start < 0 {
		t.Fatalf("missing Markdown section %q", startHeading)
	}
	if endHeading == "" {
		return doc[start:]
	}
	end := strings.Index(doc[start+len(startHeading):], endHeading)
	if end < 0 {
		t.Fatalf("missing Markdown section terminator %q after %q", endHeading, startHeading)
	}
	return doc[start : start+len(startHeading)+end]
}

func nextAtomicHeading(heading string) string {
	switch heading {
	case "#### 122. CAS - cas Rd, disp(Rs), Rt":
		return "#### 123. XCHG - xchg Rd, disp(Rs), Rt"
	case "#### 123. XCHG - xchg Rd, disp(Rs), Rt":
		return "#### 124. FAA - faa Rd, disp(Rs), Rt"
	case "#### 124. FAA - faa Rd, disp(Rs), Rt":
		return "#### 125. FAND - fand Rd, disp(Rs), Rt"
	case "#### 125. FAND - fand Rd, disp(Rs), Rt":
		return "#### 126. FOR - for Rd, disp(Rs), Rt"
	case "#### 126. FOR - for Rd, disp(Rs), Rt":
		return "#### 127. FXOR - fxor Rd, disp(Rs), Rt"
	default:
		return "#### 128. SUAEN - suaen"
	}
}

func parseIE64SourceOpcodes(t *testing.T, source string) []auditOpcode {
	t.Helper()
	re := regexp.MustCompile(`(?m)^\s*(OP_[A-Z0-9_]+)\s*=\s*(0x[0-9A-Fa-f]+)`)
	var ops []auditOpcode
	inBlock := false
	for _, line := range strings.Split(source, "\n") {
		if strings.Contains(line, "OP_MOVE") {
			inBlock = true
		}
		if !inBlock {
			continue
		}
		if m := re.FindStringSubmatch(line); m != nil {
			value, _ := strconv.ParseInt(m[2], 0, 32)
			ops = append(ops, auditOpcode{name: m[1], value: int(value)})
		}
		if strings.Contains(line, "OP_SUADIS") {
			break
		}
	}
	if len(ops) == 0 {
		t.Fatal("no IE64 opcodes parsed")
	}
	return ops
}

func parseIE32SourceOpcodes(t *testing.T, source string) []auditOpcode {
	t.Helper()
	re := regexp.MustCompile(`(?m)^\s*([A-Z][A-Z0-9_]*)\s*=\s*(0x[0-9A-Fa-f]+)`)
	allowed := map[string]bool{
		"LOAD": true, "STORE": true, "ADD": true, "SUB": true, "MUL": true, "DIV": true, "MOD": true,
		"AND": true, "OR": true, "XOR": true, "NOT": true, "SHL": true, "SHR": true,
		"JMP": true, "JNZ": true, "JZ": true, "JGT": true, "JGE": true, "JLT": true, "JLE": true,
		"PUSH": true, "POP": true, "JSR": true, "RTS": true, "SEI": true, "CLI": true, "RTI": true,
		"WAIT": true, "NOP": true, "HALT": true,
		"LDA": true, "LDX": true, "LDY": true, "LDZ": true, "STA": true, "STX": true, "STY": true, "STZ": true,
		"INC": true, "DEC": true, "LDB": true, "LDC": true, "LDD": true, "LDE": true, "LDF": true,
		"LDG": true, "LDU": true, "LDV": true, "LDW": true, "LDH": true, "LDS": true, "LDT": true,
		"STB": true, "STC": true, "STD": true, "STE": true, "STF": true, "STG": true,
		"STU": true, "STV": true, "STW": true, "STH": true, "STS": true, "STT": true,
	}
	var ops []auditOpcode
	for _, m := range re.FindAllStringSubmatch(source, -1) {
		if !allowed[m[1]] {
			continue
		}
		value, _ := strconv.ParseInt(m[2], 0, 32)
		ops = append(ops, auditOpcode{name: m[1], value: int(value)})
	}
	if len(ops) == 0 {
		t.Fatal("no IE32 opcodes parsed")
	}
	return ops
}

func parseMarkdownOpcodeRows(t *testing.T, doc string) map[string]map[int]string {
	t.Helper()
	out := make(map[string]map[int]string)
	hexRe := regexp.MustCompile(`(?i)(0x[0-9a-f]{2}|\$[0-9a-f]{2})`)
	for _, line := range strings.Split(doc, "\n") {
		if !strings.HasPrefix(line, "|") || strings.Contains(line, "---") {
			continue
		}
		cols := strings.Split(line, "|")
		var clean []string
		for _, col := range cols {
			col = strings.TrimSpace(strings.Trim(col, "`"))
			if col != "" {
				clean = append(clean, col)
			}
		}
		if len(clean) < 2 || !hexRe.MatchString(clean[0]) {
			continue
		}
		hexText := strings.ToLower(hexRe.FindString(clean[0]))
		var value64 int64
		if strings.HasPrefix(hexText, "$") {
			value64, _ = strconv.ParseInt(strings.TrimPrefix(hexText, "$"), 16, 32)
		} else {
			value64, _ = strconv.ParseInt(hexText, 0, 32)
		}
		value := int(value64)
		mnemonicCol := 1
		if len(clean) > 2 && hexRe.MatchString(clean[1]) {
			mnemonicCol = 2
		}
		mnemonic := clean[mnemonicCol]
		if out[mnemonic] == nil {
			out[mnemonic] = make(map[int]string)
		}
		out[mnemonic][value] = mnemonic
	}
	if len(out) == 0 {
		t.Fatal("no opcode rows parsed from Markdown")
	}
	return out
}

func ie64DocMnemonic(name string) string {
	mnemonic := strings.TrimPrefix(name, "OP_")
	mnemonic = strings.TrimSuffix(mnemonic, "64")
	switch mnemonic {
	case "AND64":
		mnemonic = "AND"
	case "OR64":
		mnemonic = "OR"
	case "NOT64":
		mnemonic = "NOT"
	case "MOD64":
		mnemonic = "MOD"
	case "JSR_IND":
		mnemonic = "JSR"
	}
	return mnemonic
}

func parseLuaModuleFunctions(t *testing.T, source, module string) []string {
	t.Helper()
	pattern := fmt.Sprintf(`(?s)\b%s\s*:=\s*L\.SetFuncs\(L\.NewTable\(\),\s*map\[string\]lua\.LGFunction\{(.*?)\n\s*\}\)`, regexp.QuoteMeta(module))
	blockRe := regexp.MustCompile(pattern)
	m := blockRe.FindStringSubmatch(source)
	if m == nil {
		t.Fatalf("Lua module %s not found", module)
	}
	keyRe := regexp.MustCompile(`"([A-Za-z0-9_]+)"\s*:`)
	var funcs []string
	for _, km := range keyRe.FindAllStringSubmatch(m[1], -1) {
		funcs = append(funcs, km[1])
	}
	if len(funcs) == 0 {
		t.Fatalf("Lua module %s has no parsed functions", module)
	}
	return funcs
}
