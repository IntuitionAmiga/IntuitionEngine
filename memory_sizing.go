// memory_sizing.go - Autodetected guest RAM sizing for the Intuition Engine
// appliance build. Computes total guest RAM and active CPU/profile visible
// RAM from host /proc/meminfo, a per-platform host reserve, and an optional
// hidden override hook used for deterministic tests and diagnostics.
//
// PLAN_MAX_RAM.md slice 1.

package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
)

// MIN_GUEST_RAM is the smallest total/active guest RAM size the appliance
// will accept after reserve and ceiling clamping. Matches the historical
// 32 MiB low-memory layout floor.
const MIN_GUEST_RAM uint64 = 32 * 1024 * 1024

// PlatformClass identifies the appliance hardware target. Reserve policy
// is chosen per class.
type PlatformClass int

const (
	PlatformUnknown PlatformClass = iota
	PlatformRaspberryPi64
	PlatformX64PC
	PlatformAppleSiliconLinux
)

func (p PlatformClass) String() string {
	switch p {
	case PlatformRaspberryPi64:
		return "raspberry-pi-64"
	case PlatformX64PC:
		return "x64-pc"
	case PlatformAppleSiliconLinux:
		return "apple-silicon-linux"
	default:
		return "unknown"
	}
}

var (
	ErrInsufficientHostRAM  = errors.New("insufficient host RAM after reserve")
	ErrGuestRAMBelowMinimum = errors.New("guest RAM below minimum")
	ErrUnsupportedPlatform  = errors.New("unsupported appliance platform")
	ErrInvalidSizeArg       = errors.New("invalid size argument")
	ErrMeminfoUnusable      = errors.New("meminfo missing usable fields")
	ErrInvalidOverride      = errors.New("invalid sizing override")
)

// SizingOverrides holds hidden test/diagnostic override hooks. None of these
// are documented as user-facing tuning controls. Zero values mean "use
// autodetected behavior".
type SizingOverrides struct {
	// Platform forces the platform class. PlatformUnknown means autodetect.
	Platform PlatformClass

	// SkipPlatformCheck allows PlatformUnknown (developer/test only).
	// HostReserveExplicit must be set so a reserve can be derived.
	SkipPlatformCheck bool

	// DetectedUsableRAM, when non-zero, replaces /proc/meminfo discovery.
	DetectedUsableRAM uint64

	// HostReserveBytes overrides the per-platform reserve when
	// HostReserveExplicit is true. The explicit flag is needed so a reserve
	// of zero is distinguishable from "unset".
	HostReserveBytes    uint64
	HostReserveExplicit bool

	// TotalGuestRAM overrides total guest RAM after reserve subtraction.
	TotalGuestRAM uint64

	// ActiveVisibleRAM overrides the active visible RAM. Still clamped to
	// total guest RAM unless AllowImpossibleState is set.
	ActiveVisibleRAM uint64

	// AllowImpossibleState lets active > total. Used only by tests that wire
	// up an explicit fake/sparse memory backend.
	AllowImpossibleState bool
}

// MemorySizing is the resolved sizing decision for a single boot.
type MemorySizing struct {
	Platform          PlatformClass
	DetectedUsableRAM uint64
	HostReserve       uint64
	VisibleCeiling    uint64
	TotalGuestRAM     uint64
	ActiveVisibleRAM  uint64
	MeminfoSource     string // "MemAvailable" | "fallback" | "override"
}

// pageAlignDown floors v to the nearest MMU_PAGE_SIZE multiple.
func pageAlignDown(v uint64) uint64 {
	return v &^ uint64(MMU_PAGE_SIZE-1)
}

// ParseMeminfo extracts usable host RAM in bytes from /proc/meminfo text.
// Prefers MemAvailable; falls back to MemFree+Buffers+Cached-Shmem clamped
// at zero so the fallback can never wrap.
func ParseMeminfo(text string) (uint64, string, error) {
	fields := map[string]uint64{}
	consumed := map[string]bool{
		"MemAvailable": true,
		"MemFree":      true,
		"Buffers":      true,
		"Cached":       true,
		"Shmem":        true,
	}
	scanner := bufio.NewScanner(strings.NewReader(text))
	for scanner.Scan() {
		line := scanner.Text()
		colon := strings.IndexByte(line, ':')
		if colon < 0 {
			continue
		}
		key := strings.TrimSpace(line[:colon])
		rest := strings.TrimSpace(line[colon+1:])
		// Format: "<num> kB" (kibibytes despite kB label) or just "<num>".
		parts := strings.Fields(rest)
		if len(parts) == 0 {
			continue
		}
		val, err := strconv.ParseUint(parts[0], 10, 64)
		if err != nil {
			continue
		}
		if !consumed[key] {
			continue
		}
		mult := uint64(1)
		if len(parts) >= 2 {
			switch strings.ToLower(parts[1]) {
			case "kb":
				mult = 1024
			case "mb":
				mult = 1024 * 1024
			case "gb":
				mult = 1024 * 1024 * 1024
			case "b", "":
				mult = 1
			default:
				return 0, "", fmt.Errorf("%w: unknown unit %q on %s", ErrMeminfoUnusable, parts[1], key)
			}
		}
		fields[key] = val * mult
	}
	if err := scanner.Err(); err != nil {
		return 0, "", err
	}

	if v, ok := fields["MemAvailable"]; ok {
		return v, "MemAvailable", nil
	}
	free, fOK := fields["MemFree"]
	buf, bOK := fields["Buffers"]
	cached, cOK := fields["Cached"]
	if !fOK || !bOK || !cOK {
		return 0, "", ErrMeminfoUnusable
	}
	shmem := fields["Shmem"] // optional; 0 if absent
	sum := free + buf + cached
	var usable uint64
	if shmem >= sum {
		usable = 0
	} else {
		usable = sum - shmem
	}
	return usable, "fallback", nil
}

// ParseSizeArg parses a byte size with optional KiB/MiB/GiB suffix
// (case-insensitive). Plain numbers are bytes.
func ParseSizeArg(s string) (uint64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("%w: empty", ErrInvalidSizeArg)
	}
	// Split number and suffix.
	idx := len(s)
	for i, r := range s {
		if (r < '0' || r > '9') && r != '.' && r != '-' && r != '+' {
			idx = i
			break
		}
	}
	numPart := strings.TrimSpace(s[:idx])
	suf := strings.ToLower(strings.TrimSpace(s[idx:]))

	v, err := strconv.ParseUint(numPart, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%w: %v", ErrInvalidSizeArg, err)
	}
	switch suf {
	case "":
		return v, nil
	case "b":
		return v, nil
	case "kib", "k":
		return v * 1024, nil
	case "mib", "m":
		return v * 1024 * 1024, nil
	case "gib", "g":
		return v * 1024 * 1024 * 1024, nil
	default:
		return 0, fmt.Errorf("%w: unknown suffix %q", ErrInvalidSizeArg, suf)
	}
}

// ReserveFor returns the host/system reserve for a platform given detected
// usable RAM, applying the documented per-platform max(floor, percent) policy.
func ReserveFor(p PlatformClass, usable uint64) (uint64, error) {
	type policy struct {
		minBytes uint64
		percent  uint64 // out of 100
	}
	var pol policy
	switch p {
	case PlatformRaspberryPi64:
		pol = policy{minBytes: 768 * 1024 * 1024, percent: 25}
	case PlatformX64PC:
		pol = policy{minBytes: 1 * 1024 * 1024 * 1024, percent: 20}
	case PlatformAppleSiliconLinux:
		pol = policy{minBytes: (3 * 1024 * 1024 * 1024) / 2, percent: 25}
	default:
		return 0, fmt.Errorf("%w: %s", ErrUnsupportedPlatform, p)
	}
	// Order: multiply first to keep precision. usable * 25 fits comfortably
	// in uint64 for any plausible host RAM size.
	pct := usable * pol.percent / 100
	if pct < pol.minBytes {
		return pol.minBytes, nil
	}
	return pct, nil
}

// DetectPlatform inspects the running Linux host to pick a PlatformClass.
// Returns PlatformUnknown when the host is not one of the supported
// appliance targets.
func DetectPlatform() PlatformClass {
	if runtime.GOOS != "linux" {
		return PlatformUnknown
	}
	switch runtime.GOARCH {
	case "amd64":
		return PlatformX64PC
	case "arm64":
		model := readDeviceTreeModel()
		lower := strings.ToLower(model)
		switch {
		case strings.Contains(lower, "raspberry pi"):
			return PlatformRaspberryPi64
		case strings.Contains(lower, "apple"):
			return PlatformAppleSiliconLinux
		}
	}
	return PlatformUnknown
}

func readDeviceTreeModel() string {
	for _, p := range []string{
		"/sys/firmware/devicetree/base/model",
		"/proc/device-tree/model",
	} {
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		return strings.TrimRight(string(b), "\x00\n ")
	}
	return ""
}

// ComputeMemorySizing applies overrides + autodetect + reserve + clamp +
// page-alignment + minimum-RAM checks and returns the resolved sizing.
//
// visibleCeiling is the active CPU/profile visible-RAM ceiling in bytes
// (e.g. 4 GiB for IE32/x86/M68K, larger for IE64).
func ComputeMemorySizing(visibleCeiling uint64, ov SizingOverrides) (MemorySizing, error) {
	ms := MemorySizing{}

	// Page-align the ceiling down before any clamp uses it.
	visibleCeiling = pageAlignDown(visibleCeiling)
	if visibleCeiling < MIN_GUEST_RAM {
		return ms, fmt.Errorf("%w: ceiling %d below minimum", ErrGuestRAMBelowMinimum, visibleCeiling)
	}
	ms.VisibleCeiling = visibleCeiling

	// Resolve platform. Suppress autodetect when a test has injected a
	// DetectedUsableRAM override -- autodetect is a production-only path.
	plat := ov.Platform
	if plat == PlatformUnknown && ov.DetectedUsableRAM == 0 {
		plat = DetectPlatform()
	}
	if plat == PlatformUnknown && !ov.SkipPlatformCheck {
		return ms, fmt.Errorf("%w: cannot classify host", ErrUnsupportedPlatform)
	}
	ms.Platform = plat

	// Detected usable RAM.
	usable := ov.DetectedUsableRAM
	source := "override"
	if usable == 0 {
		text, err := os.ReadFile("/proc/meminfo")
		if err != nil {
			return ms, fmt.Errorf("read /proc/meminfo: %w", err)
		}
		u, src, err := ParseMeminfo(string(text))
		if err != nil {
			return ms, err
		}
		usable = u
		source = src
	}
	ms.DetectedUsableRAM = usable
	ms.MeminfoSource = source

	// Resolve reserve.
	var reserve uint64
	if ov.HostReserveExplicit {
		reserve = ov.HostReserveBytes
	} else if plat == PlatformUnknown {
		// SkipPlatformCheck without explicit reserve is ambiguous.
		return ms, fmt.Errorf("%w: SkipPlatformCheck requires HostReserveExplicit", ErrInvalidOverride)
	} else {
		r, err := ReserveFor(plat, usable)
		if err != nil {
			return ms, err
		}
		reserve = r
	}
	ms.HostReserve = reserve

	// Total guest RAM = usable - reserve, with explicit override.
	var total uint64
	if ov.TotalGuestRAM != 0 {
		total = ov.TotalGuestRAM
	} else {
		if reserve >= usable {
			return ms, fmt.Errorf("%w: usable=%d reserve=%d", ErrInsufficientHostRAM, usable, reserve)
		}
		total = usable - reserve
	}
	total = pageAlignDown(total)
	if total < MIN_GUEST_RAM {
		return ms, fmt.Errorf("%w: total=%d", ErrGuestRAMBelowMinimum, total)
	}
	ms.TotalGuestRAM = total

	// Active visible RAM. Override-or-derive, then clamp to the CPU/profile
	// visible ceiling unless an explicit fake/sparse impossible-state
	// backend has been requested. The ceiling clamp is part of the sizing
	// contract; only AllowImpossibleState may bypass it.
	var active uint64
	if ov.ActiveVisibleRAM != 0 {
		active = ov.ActiveVisibleRAM
	} else {
		active = total
	}
	if !ov.AllowImpossibleState && active > visibleCeiling {
		active = visibleCeiling
	}
	active = pageAlignDown(active)

	// Default policy: active <= total. Tests may bypass with AllowImpossibleState.
	if !ov.AllowImpossibleState && active > total {
		return ms, fmt.Errorf("%w: active=%d > total=%d", ErrInvalidOverride, active, total)
	}
	if active < MIN_GUEST_RAM {
		return ms, fmt.Errorf("%w: active=%d", ErrGuestRAMBelowMinimum, active)
	}
	ms.ActiveVisibleRAM = active

	return ms, nil
}
