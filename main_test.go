package main

import "testing"

func TestCLIModeFromExtension_AcceptsTypedBinariesAndScripts(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"program.iex", "ie32"},
		{"program.ie32", "ie32"},
		{"program.ie64", "ie64"},
		{"program.ie65", "6502"},
		{"program.ie68", "m68k"},
		{"program.ie80", "z80"},
		{"program.ie86", "x86"},
		{"demo.ies", "script"},
		{"PROGRAM.IE68", "m68k"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got, err := cliModeFromExtension(tt.path)
			if err != nil {
				t.Fatalf("cliModeFromExtension(%q): %v", tt.path, err)
			}
			if got != tt.want {
				t.Fatalf("cliModeFromExtension(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestCLIModeFromExtension_RejectsNonTypedLaunchFiles(t *testing.T) {
	for _, path := range []string{"program.bin", "program", "program.txt", "emutos.tos", "emutos.img", "music.sid", "music.mod"} {
		t.Run(path, func(t *testing.T) {
			if got, err := cliModeFromExtension(path); err == nil {
				t.Fatalf("cliModeFromExtension(%q) = %q, want error", path, got)
			}
		})
	}
}

func TestExtractScriptFlag_OrderIndependent(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantArgs   []string
		wantScript string
	}{
		{
			name:       "before_binary",
			args:       []string{"-script", "demo.ies", "program.ie64"},
			wantArgs:   []string{"program.ie64"},
			wantScript: "demo.ies",
		},
		{
			name:       "after_binary",
			args:       []string{"program.ie64", "-script", "demo.ies"},
			wantArgs:   []string{"program.ie64"},
			wantScript: "demo.ies",
		},
		{
			name:       "equals_after_binary",
			args:       []string{"program.ie64", "--script=demo.ies"},
			wantArgs:   []string{"program.ie64"},
			wantScript: "demo.ies",
		},
		{
			name:       "explicit_mode",
			args:       []string{"-ie64", "-script", "demo.ies", "program.ie64"},
			wantArgs:   []string{"-ie64", "program.ie64"},
			wantScript: "demo.ies",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotArgs, gotScript, err := extractScriptFlag(tt.args)
			if err != nil {
				t.Fatalf("extractScriptFlag(%v): %v", tt.args, err)
			}
			if gotScript != tt.wantScript {
				t.Fatalf("script = %q, want %q", gotScript, tt.wantScript)
			}
			if len(gotArgs) != len(tt.wantArgs) {
				t.Fatalf("args = %v, want %v", gotArgs, tt.wantArgs)
			}
			for i := range gotArgs {
				if gotArgs[i] != tt.wantArgs[i] {
					t.Fatalf("args = %v, want %v", gotArgs, tt.wantArgs)
				}
			}
		})
	}
}

func TestExtractScriptFlag_MissingValue(t *testing.T) {
	if _, _, err := extractScriptFlag([]string{"program.ie64", "-script"}); err == nil {
		t.Fatal("expected missing -script value error")
	}
}

func TestValidateResolutionOverride_BothSet(t *testing.T) {
	w, h, ok := validateResolutionOverride(800, 600)
	if !ok {
		t.Fatal("expected override to be accepted")
	}
	if w != 800 || h != 600 {
		t.Fatalf("expected (800,600), got (%d,%d)", w, h)
	}
}

func TestValidateResolutionOverride_NeitherSet(t *testing.T) {
	w, h, ok := validateResolutionOverride(0, 0)
	if ok {
		t.Fatal("expected override to be disabled")
	}
	if w != 0 || h != 0 {
		t.Fatalf("expected (0,0), got (%d,%d)", w, h)
	}
}

func TestValidateResolutionOverride_OnlyWidth(t *testing.T) {
	w, h, ok := validateResolutionOverride(800, 0)
	if ok {
		t.Fatal("expected partial override to be rejected")
	}
	if w != 0 || h != 0 {
		t.Fatalf("expected (0,0), got (%d,%d)", w, h)
	}
}

func TestValidateResolutionOverride_OnlyHeight(t *testing.T) {
	w, h, ok := validateResolutionOverride(0, 600)
	if ok {
		t.Fatal("expected partial override to be rejected")
	}
	if w != 0 || h != 0 {
		t.Fatalf("expected (0,0), got (%d,%d)", w, h)
	}
}
