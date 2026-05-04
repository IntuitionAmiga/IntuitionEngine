package main

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

func writeRawSnapshotForTest(t *testing.T, regCount, memLen uint32) string {
	t.Helper()

	var buf bytes.Buffer
	buf.WriteString(snapshotMagic)
	_ = binary.Write(&buf, binary.LittleEndian, uint32(snapshotVersion))
	buf.WriteByte(3)
	buf.WriteString("CPU")
	_ = binary.Write(&buf, binary.LittleEndian, regCount)
	for i := uint32(0); i < regCount && i < 4; i++ {
		buf.WriteByte(1)
		buf.WriteByte(byte('A' + i))
		_ = binary.Write(&buf, binary.LittleEndian, uint64(i))
		_ = binary.Write(&buf, binary.LittleEndian, uint32(32))
	}
	_ = binary.Write(&buf, binary.LittleEndian, memLen)
	gz := gzip.NewWriter(&buf)
	_, _ = gz.Write(nil)
	_ = gz.Close()

	path := filepath.Join(t.TempDir(), "snapshot.iems")
	if err := os.WriteFile(path, buf.Bytes(), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadSnapshotRejectsOversizeRegisterCount(t *testing.T) {
	path := writeRawSnapshotForTest(t, snapshotMaxRegisters+1, 0)
	if _, err := LoadSnapshotFromFile(path); err == nil {
		t.Fatal("expected oversize register count error")
	}
}

func TestLoadSnapshotRejectsOversizeMemoryLength(t *testing.T) {
	path := writeRawSnapshotForTest(t, 0, snapshotMaxMemory+1)
	if _, err := LoadSnapshotFromFile(path); err == nil {
		t.Fatal("expected oversize memory length error")
	}
}
