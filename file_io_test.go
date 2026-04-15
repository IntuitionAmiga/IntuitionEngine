package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestFileIO_ReadFile(t *testing.T) {
	bus := NewMachineBus()
	tmpDir, err := os.MkdirTemp("", "fileio_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	content := []byte("Hello, Intuition Engine!")
	err = os.WriteFile(filepath.Join(tmpDir, "test.txt"), content, 0644)
	if err != nil {
		t.Fatal(err)
	}

	fio := NewFileIODevice(bus, tmpDir)

	// Setup MMIO registers
	fileNameAddr := uint32(0x1000)
	dataBufAddr := uint32(0x2000)

	// Write filename to bus
	for i, b := range []byte("test.txt\x00") {
		bus.Write8(fileNameAddr+uint32(i), b)
	}

	fio.HandleWrite(FILE_NAME_PTR, fileNameAddr)
	fio.HandleWrite(FILE_DATA_PTR, dataBufAddr)
	fio.HandleWrite(FILE_CTRL, FILE_OP_READ)

	if fio.HandleRead(FILE_STATUS) != 0 {
		t.Errorf("expected status 0, got %d", fio.HandleRead(FILE_STATUS))
	}
	if fio.HandleRead(FILE_RESULT_LEN) != uint32(len(content)) {
		t.Errorf("expected result len %d, got %d", len(content), fio.HandleRead(FILE_RESULT_LEN))
	}

	// Verify data in bus
	got := make([]byte, len(content))
	for i := range got {
		got[i] = bus.Read8(dataBufAddr + uint32(i))
	}
	if !bytes.Equal(got, content) {
		t.Errorf("expected %q, got %q", content, got)
	}
}

func TestFileIO_WriteFile(t *testing.T) {
	bus := NewMachineBus()
	tmpDir, err := os.MkdirTemp("", "fileio_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	fio := NewFileIODevice(bus, tmpDir)

	content := []byte("Writing from VM!")
	fileNameAddr := uint32(0x1000)
	dataBufAddr := uint32(0x2000)

	// Write filename and data to bus
	for i, b := range []byte("out.txt\x00") {
		bus.Write8(fileNameAddr+uint32(i), b)
	}
	for i, b := range content {
		bus.Write8(dataBufAddr+uint32(i), b)
	}

	fio.HandleWrite(FILE_NAME_PTR, fileNameAddr)
	fio.HandleWrite(FILE_DATA_PTR, dataBufAddr)
	fio.HandleWrite(FILE_DATA_LEN, uint32(len(content)))
	fio.HandleWrite(FILE_CTRL, FILE_OP_WRITE)

	if fio.HandleRead(FILE_STATUS) != 0 {
		t.Errorf("expected status 0, got %d", fio.HandleRead(FILE_STATUS))
	}

	// Verify file on disk
	got, err := os.ReadFile(filepath.Join(tmpDir, "out.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, content) {
		t.Errorf("expected %q, got %q", content, got)
	}
}

func TestFileIO_ReadNotFound(t *testing.T) {
	bus := NewMachineBus()
	tmpDir, err := os.MkdirTemp("", "fileio_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	fio := NewFileIODevice(bus, tmpDir)

	fileNameAddr := uint32(0x1000)
	for i, b := range []byte("missing.txt\x00") {
		bus.Write8(fileNameAddr+uint32(i), b)
	}

	fio.HandleWrite(FILE_NAME_PTR, fileNameAddr)
	fio.HandleWrite(FILE_CTRL, FILE_OP_READ)

	if fio.HandleRead(FILE_STATUS) != 1 {
		t.Errorf("expected status 1, got %d", fio.HandleRead(FILE_STATUS))
	}
	if fio.HandleRead(FILE_ERROR_CODE) != FILE_ERR_NOT_FOUND {
		t.Errorf("expected error code %d, got %d", FILE_ERR_NOT_FOUND, fio.HandleRead(FILE_ERROR_CODE))
	}
}

func TestFileIO_PathTraversal(t *testing.T) {
	bus := NewMachineBus()
	tmpDir, err := os.MkdirTemp("", "fileio_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	fio := NewFileIODevice(bus, tmpDir)

	badPaths := []string{
		"../test.txt",
		"/etc/passwd",
		"subdir/../../test.txt",
	}

	for _, path := range badPaths {
		fileNameAddr := uint32(0x1000)
		for i, b := range append([]byte(path), 0) {
			bus.Write8(fileNameAddr+uint32(i), b)
		}

		fio.HandleWrite(FILE_NAME_PTR, fileNameAddr)
		fio.HandleWrite(FILE_CTRL, FILE_OP_READ)

		if fio.HandleRead(FILE_STATUS) != 1 {
			t.Errorf("path %q: expected status 1, got %d", path, fio.HandleRead(FILE_STATUS))
		}
		if fio.HandleRead(FILE_ERROR_CODE) != FILE_ERR_PATH_TRAVERSAL {
			t.Errorf("path %q: expected error code %d, got %d", path, FILE_ERR_PATH_TRAVERSAL, fio.HandleRead(FILE_ERROR_CODE))
		}
	}
}

func TestFileIODevice_HandleWrite8(t *testing.T) {
	bus := NewMachineBus()
	tmpDir, err := os.MkdirTemp("", "fileio_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	content := []byte("Byte-level File I/O works!")
	err = os.WriteFile(filepath.Join(tmpDir, "byte8.txt"), content, 0644)
	if err != nil {
		t.Fatal(err)
	}

	fio := NewFileIODevice(bus, tmpDir)

	// Write filename to bus memory
	fileNameAddr := uint32(0x1000)
	for i, b := range []byte("byte8.txt\x00") {
		bus.Write8(fileNameAddr+uint32(i), b)
	}
	dataBufAddr := uint32(0x2000)

	// Write FILE_NAME_PTR byte-by-byte (little-endian)
	fio.HandleWrite8(FILE_NAME_PTR+0, uint8(fileNameAddr&0xFF))
	fio.HandleWrite8(FILE_NAME_PTR+1, uint8((fileNameAddr>>8)&0xFF))
	fio.HandleWrite8(FILE_NAME_PTR+2, uint8((fileNameAddr>>16)&0xFF))
	fio.HandleWrite8(FILE_NAME_PTR+3, uint8((fileNameAddr>>24)&0xFF))

	// Verify the assembled 32-bit value
	if fio.HandleRead(FILE_NAME_PTR) != fileNameAddr {
		t.Fatalf("FILE_NAME_PTR: expected 0x%08X, got 0x%08X", fileNameAddr, fio.HandleRead(FILE_NAME_PTR))
	}

	// Write FILE_DATA_PTR byte-by-byte
	fio.HandleWrite8(FILE_DATA_PTR+0, uint8(dataBufAddr&0xFF))
	fio.HandleWrite8(FILE_DATA_PTR+1, uint8((dataBufAddr>>8)&0xFF))
	fio.HandleWrite8(FILE_DATA_PTR+2, uint8((dataBufAddr>>16)&0xFF))
	fio.HandleWrite8(FILE_DATA_PTR+3, uint8((dataBufAddr>>24)&0xFF))

	if fio.HandleRead(FILE_DATA_PTR) != dataBufAddr {
		t.Fatalf("FILE_DATA_PTR: expected 0x%08X, got 0x%08X", dataBufAddr, fio.HandleRead(FILE_DATA_PTR))
	}

	// Write FILE_CTRL byte 0 = FILE_OP_READ to trigger the read
	fio.HandleWrite8(FILE_CTRL, FILE_OP_READ)

	if fio.HandleRead(FILE_STATUS) != 0 {
		t.Fatalf("expected status 0, got %d (error code %d)", fio.HandleRead(FILE_STATUS), fio.HandleRead(FILE_ERROR_CODE))
	}
	if fio.HandleRead(FILE_RESULT_LEN) != uint32(len(content)) {
		t.Fatalf("expected result len %d, got %d", len(content), fio.HandleRead(FILE_RESULT_LEN))
	}

	// Verify data in bus
	got := make([]byte, len(content))
	for i := range got {
		got[i] = bus.Read8(dataBufAddr + uint32(i))
	}
	if !bytes.Equal(got, content) {
		t.Fatalf("expected %q, got %q", content, got)
	}

	// Test partial byte writes accumulate correctly
	// Write 0xDEADBEEF to FILE_DATA_LEN byte-by-byte
	fio.HandleWrite8(FILE_DATA_LEN+0, 0xEF)
	fio.HandleWrite8(FILE_DATA_LEN+1, 0xBE)
	fio.HandleWrite8(FILE_DATA_LEN+2, 0xAD)
	fio.HandleWrite8(FILE_DATA_LEN+3, 0xDE)
	if fio.HandleRead(FILE_DATA_LEN) != 0xDEADBEEF {
		t.Fatalf("FILE_DATA_LEN: expected 0xDEADBEEF, got 0x%08X", fio.HandleRead(FILE_DATA_LEN))
	}

	// Test that writing upper bytes of FILE_CTRL does NOT trigger an operation
	fio.HandleWrite8(FILE_CTRL+1, 0xFF) // Should be a no-op
	fio.HandleWrite8(FILE_CTRL+2, 0xFF) // Should be a no-op
	fio.HandleWrite8(FILE_CTRL+3, 0xFF) // Should be a no-op
	// Status should still be 0 from previous successful read
	if fio.HandleRead(FILE_STATUS) != 0 {
		t.Fatalf("upper CTRL byte write should not trigger operation, got status %d", fio.HandleRead(FILE_STATUS))
	}
}

func TestFileIO_MachineBusByteWritesUseHandleWrite8(t *testing.T) {
	bus := NewMachineBus()
	tmpDir, err := os.MkdirTemp("", "fileio_bus_byte_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	content := []byte("banked byte writes")
	if err := os.WriteFile(filepath.Join(tmpDir, "bytebus.txt"), content, 0644); err != nil {
		t.Fatal(err)
	}

	fio := NewFileIODevice(bus, tmpDir)
	bus.MapIO(FILE_IO_BASE, FILE_IO_END, fio.HandleRead, fio.HandleWrite)
	bus.MapIOByte(FILE_IO_BASE, FILE_IO_END, fio.HandleWrite8)

	fileNameAddr := uint32(0x1000)
	dataBufAddr := uint32(0x600000)
	for i, b := range []byte("bytebus.txt\x00") {
		bus.Write8(fileNameAddr+uint32(i), b)
	}

	for i := uint32(0); i < 4; i++ {
		bus.Write8(FILE_NAME_PTR+i, uint8(fileNameAddr>>(i*8)))
		bus.Write8(FILE_DATA_PTR+i, uint8(dataBufAddr>>(i*8)))
	}
	bus.Write8(FILE_CTRL, FILE_OP_READ)

	if got := fio.HandleRead(FILE_NAME_PTR); got != fileNameAddr {
		t.Fatalf("FILE_NAME_PTR: got 0x%08X, want 0x%08X", got, fileNameAddr)
	}
	if got := fio.HandleRead(FILE_DATA_PTR); got != dataBufAddr {
		t.Fatalf("FILE_DATA_PTR: got 0x%08X, want 0x%08X", got, dataBufAddr)
	}
	if got := fio.HandleRead(FILE_STATUS); got != 0 {
		t.Fatalf("status: got %d, want 0 (error code %d)", got, fio.HandleRead(FILE_ERROR_CODE))
	}
	if got := fio.HandleRead(FILE_RESULT_LEN); got != uint32(len(content)) {
		t.Fatalf("result len: got %d, want %d", got, len(content))
	}
	got := make([]byte, len(content))
	for i := range got {
		got[i] = bus.Read8(dataBufAddr + uint32(i))
	}
	if !bytes.Equal(got, content) {
		t.Fatalf("content mismatch: got %q want %q", got, content)
	}
}

func TestFileIO_ReadEmptyFile(t *testing.T) {
	bus := NewMachineBus()
	tmpDir, err := os.MkdirTemp("", "fileio_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	err = os.WriteFile(filepath.Join(tmpDir, "empty.txt"), []byte{}, 0644)
	if err != nil {
		t.Fatal(err)
	}

	fio := NewFileIODevice(bus, tmpDir)

	fileNameAddr := uint32(0x1000)
	for i, b := range []byte("empty.txt\x00") {
		bus.Write8(fileNameAddr+uint32(i), b)
	}

	fio.HandleWrite(FILE_NAME_PTR, fileNameAddr)
	fio.HandleWrite(FILE_CTRL, FILE_OP_READ)

	if fio.HandleRead(FILE_STATUS) != 0 {
		t.Errorf("expected status 0, got %d", fio.HandleRead(FILE_STATUS))
	}
	if fio.HandleRead(FILE_RESULT_LEN) != 0 {
		t.Errorf("expected result len 0, got %d", fio.HandleRead(FILE_RESULT_LEN))
	}
}
