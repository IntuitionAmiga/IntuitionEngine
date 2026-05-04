package main

import "testing"

func FuzzLHA(f *testing.F) {
	f.Add(buildLHALevel0("-lh0-", []byte("seed"), []byte("seed")))
	f.Add([]byte{0x20, 0x00, '-', 'l', 'h', '5', '-'})
	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = parseLHAHeader(data)
		_, _ = DecompressLHAData(data)
	})
}

func FuzzLH5(f *testing.F) {
	f.Add(testCompressLH5([]byte("seed")), 4)
	f.Add([]byte{0, 0}, 1)
	f.Fuzz(func(t *testing.T, data []byte, size int) {
		if size < 1 || size > 4096 {
			size = 1
		}
		_, _ = decompressLH5(data, size)
	})
}

func FuzzLH1(f *testing.F) {
	f.Add(testCompressLH1([]byte("seed")), 4)
	f.Add([]byte{0}, 1)
	f.Fuzz(func(t *testing.T, data []byte, size int) {
		if size < 1 || size > 4096 {
			size = 1
		}
		_, _ = decompressLH1(data, size)
	})
}

func FuzzICE(f *testing.F) {
	f.Add([]byte{
		0x49, 0x43, 0x45, 0x21,
		0x00, 0x00, 0x00, 0x0d,
		0x00, 0x00, 0x00, 0x01,
		0x00,
	})
	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = UnpackICE(data)
	})
}
