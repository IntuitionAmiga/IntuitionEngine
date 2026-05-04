package main

import (
	"fmt"
	"os"
	"path/filepath"
)

const (
	arosDirectVRAMBase = 0x1E00000
	arosDirectVRAMSize = 0x200000
)

func resolveAROSDrivePath(explicit, exePath string) (string, error) {
	if explicit != "" {
		absPath, err := filepath.Abs(explicit)
		if err != nil || !isAROSDrivePath(absPath) {
			return "", fmt.Errorf("invalid -aros-drive %q: not a valid AROS system tree", explicit)
		}
		return absPath, nil
	}

	for _, candidate := range arosDriveCandidates(exePath) {
		absCandidate, err := filepath.Abs(candidate)
		if err != nil {
			continue
		}
		if isAROSDrivePath(absCandidate) {
			return absCandidate, nil
		}
	}

	return "", fmt.Errorf("AROS system tree not found; use -aros-drive <path> or install bundled AROS/")
}

func arosDriveCandidates(exePath string) []string {
	candidates := []string{
		"AROS/bin/ie-m68k/bin/ie-m68k/AROS",
		"../AROS/bin/ie-m68k/bin/ie-m68k/AROS",
		"AROS",
	}
	if exePath == "" {
		return candidates
	}

	exeDir := filepath.Dir(exePath)
	for _, base := range []string{
		exeDir,
		filepath.Dir(exeDir),
		filepath.Dir(filepath.Dir(exeDir)),
	} {
		candidates = append(candidates,
			filepath.Join(base, "AROS"),
			filepath.Join(base, "AROS", "bin", "ie-m68k", "bin", "ie-m68k", "AROS"),
		)
	}
	return candidates
}

func isAROSDrivePath(path string) bool {
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return false
	}

	startup := filepath.Join(path, "S", "Startup-Sequence")
	startupInfo, err := os.Stat(startup)
	return err == nil && !startupInfo.IsDir()
}

func configureArosVRAM(sysBus *MachineBus, videoChip *VideoChip) []byte {
	sysBus.UnmapIO(VRAM_START, VRAM_START+VRAM_SIZE-1)
	videoChip.SetBusMemory(sysBus.memory)
	videoChip.SetBigEndianMode(true)
	directVRAM := sysBus.memory[arosDirectVRAMBase : arosDirectVRAMBase+arosDirectVRAMSize]
	videoChip.SetDirectVRAM(directVRAM)
	return directVRAM
}
