package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// programmeProductionEnvVars is the explicit allowlist for the performance
// programme's environment-variable drift guard. It lists exactly the production
// runtime switches the programme introduced or changed, each of which must be
// documented in the architecture.md Build Profiles and Observable Runtime
// section and must be read somewhere in the production (non-test) sources.
//
// The allowlist is deliberately narrow. It excludes, on purpose:
//   - CPU and JIT controls, which are outside the programme's scope;
//   - test-only and benchmark gates (IE_SWAP_HASH, IE_BENCH_GATE, IE_GPU_BENCH,
//     IE_PROFILE and similar), which never gate guest-visible behaviour;
//   - diagnostic and tracing switches.
//
// A pre-existing undocumented production variable is added here only after it is
// verified against the code, never wholesale.
var programmeProductionEnvVars = []string{
	"IE_CPUPROFILE",           // T0: boot-time CPU profile capture
	"IE_VIDEO_TILE_COMPOSITE", // T1: tile-based retained composition
	"IE_VIDEO_PARTIAL_UPLOAD", // T1: regional texture upload
	"IE_VIDEO_GPU_CONVERT",    // T1b: GPU-side format conversion
	"IE_VOODOO_TILE_RASTER",   // T1b: tiled software Voodoo rasteriser
	"IE_VOODOO_WORKERS",       // T1b: software Voodoo worker-count override
	"IE_AUDIO_BLOCK",          // T2: universal block audio
	"IE_AUDIO_EVENT_RING",     // T2: audio event ring
	"IE_BUS_SPANS",            // T3: bulk bus spans
	"IE_PAGE_DIRTY",           // T3: epoch page dirty tracking
	"IE_MON_EPOCH_HISTORY",    // T5: epoch-driven reverse history
	"IE_SCRIPT_COMPILE_CACHE", // T5: IEScript compile cache
}

// excludedFromProgrammeAllowlist are switches that must never appear in the
// programme allowlist, so a careless addition is caught: they are either out of
// scope (CPU/JIT) or test-only gates.
var excludedFromProgrammeAllowlist = []string{
	"IE_SWAP_HASH",
	"IE_BENCH_GATE",
	"IE_GPU_BENCH",
	"IE_PROFILE",
	"IE_JIT_DISPATCH_CACHE",
	"IE_JIT_SMC_RANGE",
	"IE64_JIT_REGIONS",
	"IE_SIMD",
}

// productionGoSources returns the package's non-test .go sources in the repo
// root, the surface the drift guard scans for env-var reads.
func productionGoSources(t *testing.T) string {
	t.Helper()
	matches, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	var sb strings.Builder
	for _, path := range matches {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		sb.Write(data)
		sb.WriteByte('\n')
	}
	return sb.String()
}

// TestBuildProfiles_DocumentedMatchesGetenv is the programme's env-var drift
// guard. Every allowlisted production switch must be both read by the code and
// documented in the architecture.md Build Profiles section, so the documented
// runtime surface cannot silently diverge from what the binary actually reads.
func TestBuildProfiles_DocumentedMatchesGetenv(t *testing.T) {
	code := productionGoSources(t)
	docBytes, err := os.ReadFile("sdk/docs/architecture.md")
	if err != nil {
		t.Fatal(err)
	}
	doc := string(docBytes)

	// The Build Profiles section is the authoritative surface; a switch merely
	// mentioned elsewhere in the file does not count as documented.
	const heading = "## Build Profiles and Observable Runtime"
	start := strings.Index(doc, heading)
	if start < 0 {
		t.Fatalf("architecture.md missing %q section", heading)
	}
	profiles := doc[start:]

	seen := map[string]bool{}
	for _, name := range programmeProductionEnvVars {
		if seen[name] {
			t.Fatalf("duplicate allowlist entry %s", name)
		}
		seen[name] = true

		// The read may be direct (os.Getenv("IE_...")) or indirect through a
		// named constant, so require the name to appear as a string literal in
		// production code rather than pinning a single call form.
		if !strings.Contains(code, `"`+name+`"`) {
			t.Errorf("%s is on the programme allowlist but no production source references it", name)
		}
		if !strings.Contains(profiles, name) {
			t.Errorf("%s is on the programme allowlist but is not documented in the Build Profiles section", name)
		}
	}

	for _, name := range excludedFromProgrammeAllowlist {
		if seen[name] {
			t.Errorf("%s is a test-only or out-of-scope switch and must not be on the programme allowlist", name)
		}
	}
}
