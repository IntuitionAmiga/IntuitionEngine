package main

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

type sdkISAFact struct {
	CPU        string
	Kind       string
	Value      int
	Symbol     string
	Evidence   string
	ManualText string
}

func TestSDKISAInventoryGoldenMatchesExecutableSource(t *testing.T) {
	expected := renderSDKISASourceAudit(t)
	if os.Getenv("UPDATE_SDK_ISA_SOURCE_AUDIT") == "1" {
		if err := os.WriteFile("sdk/docs/verify/SDK_ISA_SOURCE_AUDIT.md", []byte(expected), 0o644); err != nil {
			t.Fatalf("update SDK ISA source audit table: %v", err)
		}
	}
	gotBytes, err := os.ReadFile("sdk/docs/verify/SDK_ISA_SOURCE_AUDIT.md")
	if err != nil {
		t.Fatalf("SDK ISA source audit table is missing or unreadable: %v\n--- want\n%s", err, expected)
	}
	got := string(gotBytes)
	if got != expected {
		t.Fatalf("SDK ISA source audit table drifted from executable source facts\n--- got\n%s\n--- want\n%s", got, expected)
	}
}

func TestSDKISAInventoryManualCoverageMatchesSourceFacts(t *testing.T) {
	ie64Doc := readAuditFile(t, "sdk/docs/IE64_ISA.md")
	ie32Doc := readAuditFile(t, "sdk/docs/IE32_ISA.md")
	ie64OpcodeRows := parseMarkdownOpcodeRows(t, ie64Doc)
	ie32OpcodeRows := parseMarkdownOpcodeRows(t, ie32Doc)

	for _, fact := range sdkISAFactsFromExecutableSource(t) {
		switch {
		case fact.CPU == "IE64" && fact.Kind == "opcode":
			mnemonic := ie64DocMnemonic(fact.Symbol)
			requireISAInstructionHeading(t, "sdk/docs/IE64_ISA.md", ie64Doc, mnemonic)
			if _, ok := ie64OpcodeRows[mnemonic][fact.Value]; !ok {
				t.Fatalf("IE64 manual opcode table missing source opcode %s = 0x%02X", fact.Symbol, fact.Value)
			}
		case fact.CPU == "IE32" && fact.Kind == "opcode":
			requireISAInstructionHeading(t, "sdk/docs/IE32_ISA.md", ie32Doc, fact.Symbol)
			if _, ok := ie32OpcodeRows[fact.Symbol][fact.Value]; !ok {
				t.Fatalf("IE32 manual opcode table missing source opcode %s = 0x%02X", fact.Symbol, fact.Value)
			}
		case fact.CPU == "IE64" && fact.Kind == "control register":
			if !strings.Contains(ie64Doc, "`"+fact.Symbol+"`") {
				t.Fatalf("IE64 manual missing source control register %s", fact.Symbol)
			}
			if !strings.Contains(ie64Doc, fmt.Sprintf("CR%d", fact.Value)) {
				t.Fatalf("IE64 manual missing CR number for %s = CR%d", fact.Symbol, fact.Value)
			}
		case fact.CPU == "IE64" && fact.Kind == "fpu side effect":
			mnemonic := ie64DocMnemonic(fact.Symbol)
			entry := sdkISAInstructionEntryByMnemonic(t, ie64Doc, mnemonic)
			want := fact.ManualText
			if !strings.Contains(entry, want) {
				t.Fatalf("IE64 manual %s entry missing source-backed FPU side-effect text: %s", mnemonic, want)
			}
		}
	}
}

func TestSDKISAAuditLedgerRequiresEmpiricalSourceOnlyInventory(t *testing.T) {
	ledger := readAuditFile(t, "sdk/docs/verify/SDK_DOC_AUDIT_LEDGER.md")
	ledgerText := strings.Join(strings.Fields(ledger), " ")
	for _, needle := range []string{
		"SDK_ISA_SOURCE_AUDIT.md",
		"empirically provable from executable source code on disk",
		"Source comments are not canonical",
		"Incorrect source comments must be fixed",
		"must not contain statuses",
		"assembler, disassembler, monitor, device, MMIO, and tooling claims are out of scope",
	} {
		if !strings.Contains(ledgerText, needle) {
			t.Fatalf("SDK doc audit ledger is missing empirical ISA inventory rule %q", needle)
		}
	}
}

func TestSDKISAInventoryRejectsStaleSourceComments(t *testing.T) {
	for _, tc := range []struct {
		path      string
		forbidden []string
		required  []string
	}{
		{
			path: "cpu_ie64.go",
			forbidden: []string{
				"Timer reload period (instruction cycles)",
				"decrement TIMER_COUNT every instruction cycle",
				"When it reaches 0, fire interrupt and reload from TIMER_PERIOD",
			},
			required: []string{
				"Timer reload period in decoded-instruction timer steps",
				"decrement TIMER_COUNT once per decoded instruction",
				"reload from TIMER_PERIOD before dispatching",
			},
		},
		{
			path: "cpu_ie32.go",
			forbidden: []string{
				"WAIT = 0x17 // Wait specified cycles",
				"Allocates main memory",
				"Addressing mode (IMMEDIATE/REGISTER/REG_IND/MEM_IND)",
				"Checks I/O region access (IO_BASE to IO_LIMIT)",
				"Protected by memory read mutex during resolution.",
			},
			required: []string{
				"WAIT = 0x17 // Wait specified microseconds",
				"Uses the supplied bus's shared memory slice",
				"resolveOperand applies read-style IE32 operand resolution",
				"Direct: memory at operand address",
				"Reserved addressing-mode bytes resolve to zero on read-style operands.",
			},
		},
		{
			path: "ted_constants.go",
			forbidden: []string{
				"0xF0F20-0xF0F5F",
			},
			required: []string{
				"0xF0F20-0xF0F6B",
			},
		},
		{
			path: "ted_video_constants.go",
			forbidden: []string{
				"0xF0F20-0xF0F3F",
				"0xD620-0xD65F",
				"0x20-0x2F select TED video",
			},
			required: []string{
				"0xF0F20-0xF0F6B",
				"0xD620-0xD632",
				"TED_V_INDEX_BASE = 0x20",
				"TED_V_INDEX_END  = 0x32",
				"0x20-0x32 select TED video",
			},
		},
		{
			path: "cpu_x86_runner.go",
			forbidden: []string{
				"video registers 0x20-0x2F",
				"0xF0F20-0xF0F5F",
			},
			required: []string{
				"video registers 0x20-0x32",
				"0xF0F20-0xF0F6B",
				"X86_TED_V_INDEX_BASE = TED_V_INDEX_BASE",
				"b.tedRegSelect >= X86_TED_V_INDEX_BASE && b.tedRegSelect <= X86_TED_V_INDEX_END",
			},
		},
		{
			path: "cpu_z80_runner.go",
			forbidden: []string{
				"0x20-0x2F = TED video",
				"TED video registers (0x20-0x2F)",
				"Enable Z80 JIT compiler (x86-64/ARM64)",
			},
			required: []string{
				"0x20-0x32 = TED video",
				"TED video registers (0x20-0x32)",
				"Enable Z80 JIT compiler on supported amd64 hosts",
			},
		},
		{
			path: "sdk/include/ie65.inc",
			forbidden: []string{
				"$D620-$D62F",
				"$F0F20-$F0F5F",
			},
			required: []string{
				"$D620-$D632",
				"$F0F20-$F0F6B",
				"TED_V_RASTER_CMP_LO",
				"TED_V_RASTER_STATUS",
			},
		},
		{
			path: "sdk/include/ie80.inc",
			forbidden: []string{
				"0x20-0x2F",
				"0xF0F20-0xF0F5F",
			},
			required: []string{
				"0x20-0x32",
				"0xF0F20-0xF0F6B",
				"TED_V_RASTER_CMP_LO",
				"TED_V_RASTER_STATUS",
			},
		},
		{
			path: "registers.go",
			forbidden: []string{
				"0xF0F00-0xF0F5F     96B     TED",
				"TED (0xF0F00-0xF0F5F)",
				"Video: TED_V_* (0xF0F20-0xF0F5F)",
				"video 0xF0F20-0xF0F5F",
				"TED_REGION_END  = 0xF0F5F",
				"Timer registers at IO_BASE+0x04, IO_BASE+0x08",
			},
			required: []string{
				"0xF0F00-0xF0F6B     108B    TED",
				"TED (0xF0F00-0xF0F6B)",
				"Video: TED_V_* (0xF0F20-0xF0F6B)",
				"video 0xF0F20-0xF0F6B",
				"TED_REGION_END  = 0xF0F6B",
				"Timer state is CPU-integrated; IO_BASE timer mirrors are not a stable bus ABI",
			},
		},
		{
			path: "machine_bus.go",
			forbidden: []string{
				"future IE64 CR_RAM_SIZE_BYTES",
			},
			required: []string{
				"IE64 CR_RAM_SIZE_BYTES",
			},
		},
		{
			path: "ahx_constants.go",
			forbidden: []string{
				"0xF0B80-0xF0B94",
				"0xF0B84-0xF0B94",
				"AHX Engine registers (memory-mapped at 0xF0B80-0xF0B91)",
			},
			required: []string{
				"0xF0B80-0xF0B91",
				"0xF0B84-0xF0B91",
				"AHX engine/player register constants",
				"AHX engine/control register (memory-mapped at 0xF0B80)",
				"AHX engine/player register block spans 0xF0B80-0xF0B91.",
			},
		},
		{
			path: "sid_constants.go",
			forbidden: []string{
				"SID_POT_X = 0xF0E19",
				"Potentiometer X (not implemented)",
				"Read-only registers (on real SID, we can emulate these)",
			},
			required: []string{
				"SID_PLUS_CTRL = 0xF0E19",
				"Offset 0x19 is SID+ control state in Intuition Engine",
				"SID_POT_Y = 0xF0E1A // Potentiometer Y backing register",
			},
		},
		{
			path: "cpu_ie64.go",
			forbidden: []string{
				"Kernel handlers that can take a nested synchronous trap must MFCR CR_SAVED_SUA into a GPR",
				"skipping it loses the outer value on the next nested trap",
				"mirrors FAULT_PC for save/restore discipline",
			},
			required: []string{
				"CR_SAVED_SUA    = 14 // Saved SUA latch for the active trap frame; restored by ERET on supervisor return",
				"Nested trap preservation is therefore architectural",
				"kernel handlers do not need to save and restore CR_SAVED_SUA or",
				"CR_FAULT_PC manually to survive nested synchronous traps",
			},
		},
	} {
		source := readAuditFile(t, tc.path)
		for _, needle := range tc.forbidden {
			if strings.Contains(source, needle) {
				t.Fatalf("%s contains stale source comment %q", tc.path, needle)
			}
		}
		for _, needle := range tc.required {
			if !strings.Contains(source, needle) {
				t.Fatalf("%s missing corrected source comment %q", tc.path, needle)
			}
		}
	}
}

func renderSDKISASourceAudit(t *testing.T) string {
	t.Helper()
	var b strings.Builder
	b.WriteString("# SDK ISA Source Audit\n\n")
	b.WriteString("| CPU | Kind | Value | Source symbol | Executable evidence |\n")
	b.WriteString("|-----|------|-------|---------------|---------------------|\n")
	for _, fact := range sdkISAFactsFromExecutableSource(t) {
		b.WriteString(fmt.Sprintf("| %s | %s | 0x%02X | `%s` | %s |\n", fact.CPU, fact.Kind, fact.Value, fact.Symbol, fact.Evidence))
	}
	return b.String()
}

func sdkISAFactsFromExecutableSource(t *testing.T) []sdkISAFact {
	t.Helper()
	ie64OpcodeSource := readAuditFile(t, "cpu_ie64_opcodes_gen.go")
	ie64Source := readAuditFile(t, "cpu_ie64.go")
	ie32Source := readAuditFile(t, "cpu_ie32.go")
	fpu64Source := readAuditFile(t, "fpu_ie64.go")
	var facts []sdkISAFact

	for _, op := range parseIE64SourceOpcodes(t, ie64OpcodeSource) {
		if countSwitchCases(ie64Source, op.name) < 2 {
			t.Fatalf("IE64 opcode %s lacks execute and step switch evidence", op.name)
		}
		facts = append(facts, sdkISAFact{
			CPU:      "IE64",
			Kind:     "opcode",
			Value:    op.value,
			Symbol:   op.name,
			Evidence: fmt.Sprintf("`cpu_ie64_opcodes_gen.go` const `%s`; `cpu_ie64.go` execute switch case `%s`; step switch case `%s`", op.name, op.name, op.name),
		})
	}

	for _, op := range parseIE32InstructionConstBlockOpcodes(t, ie32Source) {
		if countSwitchCases(ie32Source, op.name) < 2 {
			t.Fatalf("IE32 opcode %s lacks execute and step switch evidence", op.name)
		}
		facts = append(facts, sdkISAFact{
			CPU:      "IE32",
			Kind:     "opcode",
			Value:    op.value,
			Symbol:   op.name,
			Evidence: fmt.Sprintf("`cpu_ie32.go` const `%s`; execute switch case `%s`; step switch case `%s`", op.name, op.name, op.name),
		})
	}

	for _, cr := range parseIE64ControlRegisterConstBlock(t, ie64Source) {
		if cr.name == "CR_COUNT" {
			continue
		}
		evidence := fmt.Sprintf("`cpu_ie64.go` const `%s`; MFCR switch case `%s`", cr.name, cr.name)
		if !sourceContainsCRCaseInBlocks(ie64Source, "case OP_MFCR:", "case OP_ERET:", cr.name) {
			t.Fatalf("IE64 control register %s lacks MFCR switch evidence", cr.name)
		}
		if sourceContainsCRCaseInBlocks(ie64Source, "case OP_MTCR:", "case OP_MFCR:", cr.name) {
			evidence += fmt.Sprintf("; MTCR switch case `%s`", cr.name)
		} else if cr.name == "CR_RAM_SIZE_BYTES" && strings.Contains(ie64Source, "crIdx == CR_RAM_SIZE_BYTES") && strings.Contains(ie64Source, "rd == CR_RAM_SIZE_BYTES") {
			evidence += "; MTCR illegal-instruction check `CR_RAM_SIZE_BYTES`"
		} else if cr.name == "CR_PREV_MODE" {
			evidence += "; no MTCR switch case for `CR_PREV_MODE`"
		} else {
			t.Fatalf("IE64 control register %s lacks MTCR evidence or explicit source-backed read-only evidence", cr.name)
		}
		facts = append(facts, sdkISAFact{
			CPU:      "IE64",
			Kind:     "control register",
			Value:    cr.value,
			Symbol:   cr.name,
			Evidence: evidence,
		})
	}

	facts = append(facts, sdkIE64FPUSideEffectFacts(t, ie64Source, fpu64Source)...)

	sort.Slice(facts, func(i, j int) bool {
		if facts[i].CPU != facts[j].CPU {
			return facts[i].CPU > facts[j].CPU
		}
		if facts[i].Kind != facts[j].Kind {
			return facts[i].Kind > facts[j].Kind
		}
		if facts[i].Value != facts[j].Value {
			return facts[i].Value < facts[j].Value
		}
		return facts[i].Symbol < facts[j].Symbol
	})
	return facts
}

func sdkIE64FPUSideEffectFacts(t *testing.T, cpuSource, fpuSource string) []sdkISAFact {
	t.Helper()
	opcodeSource := readAuditFile(t, "cpu_ie64_opcodes_gen.go")
	opcodes := make(map[string]int)
	for _, op := range parseIE64SourceOpcodes(t, opcodeSource) {
		opcodes[op.name] = op.value
	}

	tests := []struct {
		symbol        string
		functionName  string
		conditionCall string
		inlineCase    bool
		readsFPCR     bool
		writesSticky  bool
		manualText    string
	}{
		{"OP_FADD", "FADD", "setConditionCodesBits", false, false, true, "writes FPSR condition-code bits from the result and may set FPSR sticky exception flags; FPCR is not read"},
		{"OP_FSUB", "FSUB", "setConditionCodesBits", false, false, true, "writes FPSR condition-code bits from the result and may set FPSR sticky exception flags; FPCR is not read"},
		{"OP_FMUL", "FMUL", "setConditionCodesBits", false, false, true, "writes FPSR condition-code bits from the result and may set FPSR sticky exception flags; FPCR is not read"},
		{"OP_FDIV", "FDIV", "setConditionCodesBits", false, false, true, "writes FPSR condition-code bits from the result and may set FPSR sticky exception flags; FPCR is not read"},
		{"OP_FMOD", "FMOD", "setConditionCodesBits", false, false, true, "writes FPSR condition-code bits from the result and may set FPSR sticky exception flags; FPCR is not read"},
		{"OP_FABS", "FABS", "setConditionCodesBits", true, false, false, "writes FPSR condition-code bits from the result and does not set FPSR sticky exception flags; FPCR is not read"},
		{"OP_FNEG", "FNEG", "setConditionCodesBits", true, false, false, "writes FPSR condition-code bits from the result and does not set FPSR sticky exception flags; FPCR is not read"},
		{"OP_FSQRT", "FSQRT", "setConditionCodesBits", false, false, true, "writes FPSR condition-code bits from the result and may set FPSR sticky exception flags; FPCR is not read"},
		{"OP_FINT", "FINT", "setConditionCodes", false, true, false, "reads FPCR rounding bits, writes FPSR condition-code bits from the rounded result, and does not set FPSR sticky exception flags"},
		{"OP_FSIN", "FSIN", "setConditionCodes", false, false, false, "writes FPSR condition-code bits from the result and does not set FPSR sticky exception flags; FPCR is not read"},
		{"OP_FCOS", "FCOS", "setConditionCodes", false, false, false, "writes FPSR condition-code bits from the result and does not set FPSR sticky exception flags; FPCR is not read"},
		{"OP_FTAN", "FTAN", "setConditionCodes", false, false, false, "writes FPSR condition-code bits from the result and does not set FPSR sticky exception flags; FPCR is not read"},
		{"OP_FATAN", "FATAN", "setConditionCodes", false, false, false, "writes FPSR condition-code bits from the result and does not set FPSR sticky exception flags; FPCR is not read"},
		{"OP_FLOG", "FLOG", "setConditionCodes", false, false, true, "writes FPSR condition-code bits from the result and may set FPSR sticky exception flags; FPCR is not read"},
		{"OP_FEXP", "FEXP", "setConditionCodesBits", false, false, true, "writes FPSR condition-code bits from the result and may set FPSR sticky exception flags; FPCR is not read"},
		{"OP_FPOW", "FPOW", "setConditionCodesBits", false, false, true, "writes FPSR condition-code bits from the result and may set FPSR sticky exception flags; FPCR is not read"},
		{"OP_DADD", "DADD", "setConditionCodesBits64", false, false, true, "writes FPSR condition-code bits from the result and may set FPSR sticky exception flags; FPCR is not read"},
		{"OP_DSUB", "DSUB", "setConditionCodesBits64", false, false, true, "writes FPSR condition-code bits from the result and may set FPSR sticky exception flags; FPCR is not read"},
		{"OP_DMUL", "DMUL", "setConditionCodesBits64", false, false, true, "writes FPSR condition-code bits from the result and may set FPSR sticky exception flags; FPCR is not read"},
		{"OP_DDIV", "DDIV", "setConditionCodesBits64", false, false, true, "writes FPSR condition-code bits from the result and may set FPSR sticky exception flags; FPCR is not read"},
		{"OP_DMOD", "DMOD", "setConditionCodesBits64", false, false, true, "writes FPSR condition-code bits from the result and may set FPSR sticky exception flags; FPCR is not read"},
		{"OP_DABS", "DABS", "setConditionCodesBits64", false, false, false, "writes FPSR condition-code bits from the result and does not set FPSR sticky exception flags; FPCR is not read"},
		{"OP_DNEG", "DNEG", "setConditionCodesBits64", false, false, false, "writes FPSR condition-code bits from the result and does not set FPSR sticky exception flags; FPCR is not read"},
		{"OP_DSQRT", "DSQRT", "setConditionCodesBits64", false, false, true, "writes FPSR condition-code bits from the result and may set FPSR sticky exception flags; FPCR is not read"},
		{"OP_DINT", "DINT", "setConditionCodesBits64", false, true, false, "reads FPCR rounding bits, writes FPSR condition-code bits from the rounded result, and does not set FPSR sticky exception flags"},
		{"OP_DSIN", "DSIN", "setConditionCodesBits64", false, false, false, "writes FPSR condition-code bits from the result and does not set FPSR sticky exception flags; FPCR is not read"},
		{"OP_DCOS", "DCOS", "setConditionCodesBits64", false, false, false, "writes FPSR condition-code bits from the result and does not set FPSR sticky exception flags; FPCR is not read"},
		{"OP_DTAN", "DTAN", "setConditionCodesBits64", false, false, false, "writes FPSR condition-code bits from the result and does not set FPSR sticky exception flags; FPCR is not read"},
		{"OP_DATAN", "DATAN", "setConditionCodesBits64", false, false, false, "writes FPSR condition-code bits from the result and does not set FPSR sticky exception flags; FPCR is not read"},
		{"OP_DLOG", "DLOG", "setConditionCodesBits64", false, false, true, "writes FPSR condition-code bits from the result and may set FPSR sticky exception flags; FPCR is not read"},
		{"OP_DEXP", "DEXP", "setConditionCodesBits64", false, false, true, "writes FPSR condition-code bits from the result and may set FPSR sticky exception flags; FPCR is not read"},
		{"OP_DPOW", "DPOW", "setConditionCodesBits64", false, false, true, "writes FPSR condition-code bits from the result and may set FPSR sticky exception flags; FPCR is not read"},
	}

	var facts []sdkISAFact
	for _, tc := range tests {
		value, ok := opcodes[tc.symbol]
		if !ok {
			t.Fatalf("IE64 FPU side-effect opcode not found in source: %s", tc.symbol)
		}
		caseBodies := sdkISASwitchCaseBodiesForSymbol(cpuSource, tc.symbol)
		functionBody := sdkISAFPUFunctionBody(t, fpuSource, tc.functionName)
		if !strings.Contains(functionBody, tc.conditionCall) {
			t.Fatalf("fpu_ie64.go %s no longer writes FPSR condition codes with %s", tc.functionName, tc.conditionCall)
		}
		if tc.readsFPCR && !strings.Contains(functionBody, "GetRoundingMode()") {
			t.Fatalf("fpu_ie64.go %s no longer reads FPCR rounding mode", tc.functionName)
		}
		if !tc.readsFPCR && strings.Contains(functionBody, "GetRoundingMode()") {
			t.Fatalf("fpu_ie64.go %s now reads FPCR rounding mode; review IE64 FPU docs", tc.functionName)
		}
		if tc.writesSticky && !strings.Contains(functionBody, "setExceptionFlag") {
			t.Fatalf("fpu_ie64.go %s no longer writes sticky exception flags; review IE64 FPU docs", tc.functionName)
		}
		if !tc.writesSticky && strings.Contains(functionBody, "setExceptionFlag") {
			t.Fatalf("fpu_ie64.go %s now writes sticky exception flags; review IE64 FPU docs", tc.functionName)
		}
		if tc.inlineCase {
			var executableBodies []string
			for _, body := range caseBodies {
				if strings.Contains(body, tc.conditionCall) {
					executableBodies = append(executableBodies, body)
				}
			}
			if len(executableBodies) < 2 {
				t.Fatalf("%s lacks execute and step switch cases that write FPSR condition codes with %s", tc.symbol, tc.conditionCall)
			}
			for _, body := range executableBodies {
				if strings.Contains(body, "setExceptionFlag") {
					t.Fatalf("cpu_ie64.go %s switch case now writes sticky exception flags; review IE64 FPU docs", tc.symbol)
				}
			}
		} else {
			var executableBodies []string
			for _, body := range caseBodies {
				if strings.Contains(body, "cpu.FPU."+tc.functionName+"(") {
					executableBodies = append(executableBodies, body)
				}
			}
			if len(executableBodies) < 2 {
				t.Fatalf("%s lacks execute and step switch cases that call %s", tc.symbol, tc.functionName)
			}
		}
		facts = append(facts, sdkISAFact{
			CPU:    "IE64",
			Kind:   "fpu side effect",
			Value:  value,
			Symbol: tc.symbol,
			Evidence: fmt.Sprintf(
				"`cpu_ie64.go` execute and step cases for `%s`; `fpu_ie64.go` `%s` side effects: %s",
				tc.symbol,
				tc.functionName,
				tc.manualText,
			),
			ManualText: tc.manualText,
		})
	}
	return facts
}

func sdkISASwitchCaseBodiesForSymbol(source, symbol string) []string {
	caseRe := regexp.MustCompile(`(?m)^\s*case\s+([^:\n]+):`)
	matches := caseRe.FindAllStringSubmatchIndex(source, -1)
	var bodies []string
	for i, m := range matches {
		for _, part := range strings.Split(source[m[2]:m[3]], ",") {
			if strings.TrimSpace(part) != symbol {
				continue
			}
			end := len(source)
			if i+1 < len(matches) {
				end = matches[i+1][0]
			}
			bodies = append(bodies, source[m[1]:end])
		}
	}
	return bodies
}

func sdkISAFPUFunctionBody(t *testing.T, source, name string) string {
	t.Helper()
	marker := "func (fpu *IE64FPU) " + name + "("
	start := strings.Index(source, marker)
	if start < 0 {
		t.Fatalf("fpu_ie64.go missing %s", marker)
	}
	next := strings.Index(source[start+len(marker):], "\nfunc ")
	if next < 0 {
		return source[start:]
	}
	return source[start : start+len(marker)+next]
}

func parseIE32InstructionConstBlockOpcodes(t *testing.T, source string) []auditOpcode {
	t.Helper()
	block := sourceBlockFromConstSymbols(t, source, "LOAD", "STT")
	re := regexp.MustCompile(`(?m)^\s*([A-Z][A-Z0-9_]*)\s*=\s*(0x[0-9A-Fa-f]+)`)
	var ops []auditOpcode
	for _, m := range re.FindAllStringSubmatch(block, -1) {
		value, err := strconv.ParseInt(m[2], 0, 32)
		if err != nil {
			t.Fatalf("invalid IE32 opcode literal %s for %s: %v", m[2], m[1], err)
		}
		ops = append(ops, auditOpcode{name: m[1], value: int(value)})
	}
	if len(ops) == 0 {
		t.Fatal("no IE32 instruction opcodes parsed from bounded const block")
	}
	return ops
}

func parseIE64ControlRegisterConstBlock(t *testing.T, source string) []auditOpcode {
	t.Helper()
	block := sourceBlockFromConstSymbols(t, source, "CR_PTBR", "CR_COUNT")
	re := regexp.MustCompile(`(?m)^\s*(CR_[A-Z0-9_]+)\s*=\s*([0-9]+)`)
	var regs []auditOpcode
	for _, m := range re.FindAllStringSubmatch(block, -1) {
		value, err := strconv.Atoi(m[2])
		if err != nil {
			t.Fatalf("invalid IE64 control register literal %s for %s: %v", m[2], m[1], err)
		}
		regs = append(regs, auditOpcode{name: m[1], value: value})
	}
	if len(regs) == 0 {
		t.Fatal("no IE64 control registers parsed from bounded const block")
	}
	return regs
}

func sourceBlockFromConstSymbols(t *testing.T, source, firstSymbol, lastSymbol string) string {
	t.Helper()
	startRe := regexp.MustCompile(`(?m)^\s*` + regexp.QuoteMeta(firstSymbol) + `\s*=`)
	startMatch := startRe.FindStringIndex(source)
	if startMatch == nil {
		t.Fatalf("const block start symbol not found: %s", firstSymbol)
	}
	endRe := regexp.MustCompile(`(?m)^\s*` + regexp.QuoteMeta(lastSymbol) + `\s*=.*$`)
	endMatches := endRe.FindAllStringIndex(source, -1)
	if len(endMatches) == 0 {
		t.Fatalf("const block end symbol not found: %s", lastSymbol)
	}
	endMatch := endMatches[len(endMatches)-1]
	if endMatch[0] < startMatch[0] {
		t.Fatalf("const block end symbol %s appears before start symbol %s", lastSymbol, firstSymbol)
	}
	return source[startMatch[0]:endMatch[1]]
}

func countSwitchCases(source, symbol string) int {
	caseRe := regexp.MustCompile(`(?m)^\s*case\s+([^:\n]+):`)
	count := 0
	for _, m := range caseRe.FindAllStringSubmatch(source, -1) {
		for _, part := range strings.Split(m[1], ",") {
			if strings.TrimSpace(part) == symbol {
				count++
			}
		}
	}
	return count
}

func sourceContainsCRCaseInBlocks(source, startMarker, endMarker, symbol string) bool {
	start := 0
	for {
		startIdx := strings.Index(source[start:], startMarker)
		if startIdx < 0 {
			return false
		}
		startIdx += start
		endIdx := strings.Index(source[startIdx+len(startMarker):], endMarker)
		if endIdx < 0 {
			return false
		}
		endIdx += startIdx + len(startMarker)
		if countSwitchCases(source[startIdx:endIdx], symbol) > 0 {
			return true
		}
		start = endIdx + len(endMarker)
	}
}

func requireISAInstructionHeading(t *testing.T, path, doc, mnemonic string) {
	t.Helper()
	re := regexp.MustCompile(`(?m)^#### [0-9]+\. ` + regexp.QuoteMeta(mnemonic) + `\b`)
	if !re.MatchString(doc) {
		t.Fatalf("%s missing complete instruction reference entry for %s", path, mnemonic)
	}
}

func sdkISAInstructionEntryByMnemonic(t *testing.T, doc, mnemonic string) string {
	t.Helper()
	startRe := regexp.MustCompile(`(?m)^#### [0-9]+\. ` + regexp.QuoteMeta(mnemonic) + `\b.*$`)
	start := startRe.FindStringIndex(doc)
	if start == nil {
		t.Fatalf("IE64 manual missing complete instruction reference entry for %s", mnemonic)
	}
	nextRe := regexp.MustCompile(`(?m)^#### [0-9]+\. `)
	rest := doc[start[1]:]
	next := nextRe.FindStringIndex(rest)
	if next == nil {
		return doc[start[0]:]
	}
	return doc[start[0] : start[1]+next[0]]
}
