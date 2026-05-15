//go:build headless

package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type frameStats struct {
	Len         int
	NonBlackRGB int
	OpaqueAlpha int
	ZeroAlpha   int
	Hash        uint32
}

func collectFrameStats(frame []byte) frameStats {
	stats := frameStats{Len: len(frame), Hash: frameHash(frame)}
	for i := 0; i+3 < len(frame); i += 4 {
		if frame[i] != 0 || frame[i+1] != 0 || frame[i+2] != 0 {
			stats.NonBlackRGB++
		}
		if frame[i+3] == 0 {
			stats.ZeroAlpha++
		} else {
			stats.OpaqueAlpha++
		}
	}
	return stats
}

func requireAROSDriveRoot(t *testing.T) string {
	t.Helper()

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() failed: %v", err)
	}
	driveRoot, driveErr := resolveAROSDrivePath("", filepath.Join(wd, "IntuitionEngine"))
	if driveErr != nil || !isAROSDrivePath(driveRoot) {
		t.Skip("AROS drive tree not available")
	}
	return driveRoot
}

func newBootedAROSVisualEnvironment(t *testing.T) (*AROSInterpreterBootEnvironment, *VideoCompositor, AROSBootResult) {
	t.Helper()

	rom, err := os.ReadFile("sdk/roms/aros-ie-m68k.rom")
	if err != nil {
		t.Skipf("AROS ROM not available: %v", err)
	}
	env, err := NewAROSInterpreterBootEnvironment(rom, requireAROSDriveRoot(t))
	if err != nil {
		t.Fatalf("NewAROSInterpreterBootEnvironment() failed: %v", err)
	}

	compositor := NewVideoCompositor(nil)
	compositor.RegisterSource(env.Video)
	compositor.SetFrameCallback(func() {})
	if err := compositor.Start(); err != nil {
		env.Close()
		t.Fatalf("compositor.Start() failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	result, err := env.BootAndWait(ctx)
	if err != nil {
		compositor.Stop()
		env.Close()
		t.Fatalf("BootAndWait() failed: %v", err)
	}
	if result.TimedOut || !result.Ready.Ready || len(result.Faults) != 0 {
		compositor.Stop()
		env.Close()
		t.Fatalf("AROS boot did not reach clean ready state: ready=%+v timedOut=%v faults=%+v",
			result.Ready, result.TimedOut, result.Faults)
	}

	return env, compositor, result
}

func waitForNonBlackFrame(frameFn func() []byte, timeout time.Duration) ([]byte, frameStats) {
	deadline := time.Now().Add(timeout)
	var last []byte
	var lastStats frameStats
	for time.Now().Before(deadline) {
		last = frameFn()
		lastStats = collectFrameStats(last)
		if lastStats.NonBlackRGB > 0 {
			return last, lastStats
		}
		time.Sleep(20 * time.Millisecond)
	}
	return last, lastStats
}

func TestAROSRuntimeBootProducesVisibleWorkbenchFrame(t *testing.T) {
	env, compositor, _ := newBootedAROSVisualEnvironment(t)
	defer env.Close()
	defer compositor.Stop()

	rawFrame, rawStats := waitForNonBlackFrame(env.Video.GetFrame, 5*time.Second)
	_, compStats := waitForNonBlackFrame(compositor.GetCurrentFrame, 2*time.Second)

	ctrl := env.Bus.Read32(VIDEO_CTRL)
	mode := env.Bus.Read32(VIDEO_MODE)
	status := env.Bus.Read32(VIDEO_STATUS)
	fbBase := env.Bus.Read32(VIDEO_FB_BASE)

	var failures []string
	if ctrl == 0 {
		failures = append(failures, "VIDEO_CTRL disabled")
	}
	if mode != MODE_1920x1080 {
		failures = append(failures, fmt.Sprintf("VIDEO_MODE=0x%X, want MODE_1920x1080", mode))
	}
	if fbBase < arosDirectVRAMBase || uint64(fbBase) >= uint64(arosDirectVRAMBase)+uint64(arosDirectVRAMSize) {
		failures = append(failures, fmt.Sprintf("VIDEO_FB_BASE=0x%X outside AROS direct VRAM 0x%X..0x%X",
			fbBase, arosDirectVRAMBase, arosDirectVRAMBase+arosDirectVRAMSize))
	}
	if status&videoStatusFramebufferErr != 0 {
		failures = append(failures, fmt.Sprintf("VIDEO_STATUS=0x%X has framebuffer error", status))
	}
	if len(rawFrame) == 0 || rawStats.NonBlackRGB == 0 {
		failures = append(failures, fmt.Sprintf("AROS raw IEVideo frame stayed black/empty: %+v", rawStats))
	}
	if compStats.NonBlackRGB == 0 {
		failures = append(failures, fmt.Sprintf("AROS compositor output stayed black: raw=%+v compositor=%+v",
			rawStats, compStats))
	}
	if len(failures) > 0 {
		t.Fatalf("AROS visual boot failed: ctrl=0x%X mode=0x%X status=0x%X fb=0x%X raw=%+v compositor=%+v failures=%v",
			ctrl, mode, status, fbBase, rawStats, compStats, failures)
	}
}

func TestAROSIEScriptBlackScreenDiagnostic(t *testing.T) {
	env, compositor, _ := newBootedAROSVisualEnvironment(t)
	defer env.Close()
	defer compositor.Stop()

	mon := NewMachineMonitor(env.Bus)
	mon.RegisterCPU("M68K", NewDebugM68K(env.CPU, env.Runner))

	se := NewScriptEngine(env.Bus, compositor, env.Terminal)
	se.SetMonitor(mon)
	compositor.SetFrameCallback(se.onFrameComplete)
	runtimeStatus.setCPUs(runtimeCPUM68K, nil, nil, env.Runner, nil, nil, nil)
	runtimeStatus.setChips(env.Video, nil, nil, nil, nil, nil, env.Sound, nil, nil, nil, nil, nil, nil, nil)
	defer runtimeStatus.setCPUs(runtimeCPUNone, nil, nil, nil, nil, nil, nil)
	defer runtimeStatus.setChips(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)

	scriptDir := t.TempDir()
	reportName := "aros_black_screen_report.txt"
	reportPath := filepath.Join(scriptDir, reportName)
	script := fmt.Sprintf(`
local deadline = sys.time_ms() + 5000
local nonblack = 0
local hash = 0
while sys.time_ms() < deadline do
	hash = video.frame_hash()
	local w,h = video.get_dimensions()
	for _,p in ipairs({{0,0}, {math.floor(w/2), math.floor(h/2)}, {w-1,h-1}, {64,64}, {320,240}}) do
		local r,g,b,a = video.get_pixel(p[1], p[2])
		if r ~= 0 or g ~= 0 or b ~= 0 then nonblack = nonblack + 1 end
	end
	if nonblack > 0 then break end
	sys.wait_ms(50)
end

local ctrl = video.read_reg(0xF0000)
local mode = video.read_reg(0xF0004)
local status = video.read_reg(0xF0008)
local fb = video.read_reg(0xF0084)
local color = video.read_reg(0xF0080)

local report = string.format("ctrl=0x%%X mode=0x%%X status=0x%%X fb=0x%%X color=0x%%X hash=0x%%X nonblack=%%d\n",
	ctrl, mode, status, fb, color, hash, nonblack)
sys.write_file(%q, report)
if ctrl == 0 or mode ~= 0x06 or fb < 0x1E00000 or fb >= 0x5E00000 or bit32.band(status, 0x04) ~= 0 or nonblack == 0 then
	dbg.open()
	local bug = dbg.bug_report(32)
	dbg.close()
	sys.write_file("aros_black_screen_bug.txt", bug)
	error(report)
end
`, reportName)

	if err := se.RunString(script, filepath.Join(scriptDir, "diag_aros_black_screen.ies")); err != nil {
		t.Fatalf("AROS black-screen diagnostic script failed to start: %v", err)
	}
	waitScriptStoppedWithin(t, se, 8*time.Second)
	if err := se.LastError(); err != nil {
		report, _ := os.ReadFile(reportPath)
		t.Fatalf("AROS black-screen diagnostic failed: %v\n%s", err, report)
	}
}
