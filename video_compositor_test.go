// video_compositor_test.go - Tests and benchmarks for video compositor

package main

import (
	"testing"
)

// BenchmarkFrameClear_Loop benchmarks the old loop-based frame clear
func BenchmarkFrameClear_Loop(b *testing.B) {
	// 640x480x4 = 1,228,800 bytes
	frame := make([]byte, 640*480*4)
	// Pre-fill with some data
	for i := range frame {
		frame[i] = 0xFF
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		for j := range frame {
			frame[j] = 0
		}
	}
}

// BenchmarkFrameClear_Copy benchmarks the optimized copy-based frame clear
func BenchmarkFrameClear_Copy(b *testing.B) {
	// 640x480x4 = 1,228,800 bytes
	frameSize := 640 * 480 * 4
	frame := make([]byte, frameSize)
	zeroFrame := make([]byte, frameSize)
	// Pre-fill with some data
	for i := range frame {
		frame[i] = 0xFF
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		copy(frame, zeroFrame)
	}
}

// BenchmarkFrameClear_Copy_1080p benchmarks copy for 1920x1080 resolution
func BenchmarkFrameClear_Copy_1080p(b *testing.B) {
	frameSize := 1920 * 1080 * 4
	frame := make([]byte, frameSize)
	zeroFrame := make([]byte, frameSize)
	for i := range frame {
		frame[i] = 0xFF
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		copy(frame, zeroFrame)
	}
}
