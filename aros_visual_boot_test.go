//go:build headless

package main

import (
	"os"
	"path/filepath"
	"testing"
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

