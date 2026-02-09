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
