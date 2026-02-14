// debug_snapshot.go - Machine state snapshot for save/load and backstep

package main

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"fmt"
	"io"
	"os"
)

const (
	snapshotMagic   = "IEMS"
	snapshotVersion = 1
)

// MachineSnapshot captures CPU registers and memory for save/load and backstep.
type MachineSnapshot struct {
	CPUType   string
	Registers []RegisterInfo
	Memory    []byte // full memory (compressed on disk, raw in memory for backstep)
}

// memSizeFromWidth returns the memory size in bytes for a given address width.
func memSizeFromWidth(width int) int {
	switch {
	case width <= 16:
		return 64 * 1024 // 64KB
	default:
		return 32 * 1024 * 1024 // 32MB
	}
}

// TakeSnapshot captures the current CPU registers and full memory.
func TakeSnapshot(cpu DebuggableCPU) *MachineSnapshot {
	regs := cpu.GetRegisters()
	memSize := memSizeFromWidth(cpu.AddressWidth())
	mem := cpu.ReadMemory(0, memSize)
	if mem == nil {
		mem = make([]byte, 0)
	}
	return &MachineSnapshot{
		CPUType:   cpu.CPUName(),
		Registers: regs,
		Memory:    mem,
	}
}

// RestoreSnapshot restores CPU registers and memory from a snapshot.
func RestoreSnapshot(cpu DebuggableCPU, snap *MachineSnapshot) {
	for _, r := range snap.Registers {
		cpu.SetRegister(r.Name, r.Value)
	}
	if len(snap.Memory) > 0 {
		cpu.WriteMemory(0, snap.Memory)
	}
}

// SaveSnapshotToFile writes a snapshot to disk with gzip compression.
func SaveSnapshotToFile(snap *MachineSnapshot, path string) error {
	var buf bytes.Buffer

	// Magic
	buf.WriteString(snapshotMagic)

	// Version
	binary.Write(&buf, binary.LittleEndian, uint32(snapshotVersion))

	// CPU type
	cpuBytes := []byte(snap.CPUType)
	buf.WriteByte(byte(len(cpuBytes)))
	buf.Write(cpuBytes)

	// Register count
	binary.Write(&buf, binary.LittleEndian, uint32(len(snap.Registers)))

	// Registers
	for _, r := range snap.Registers {
		nameBytes := []byte(r.Name)
		buf.WriteByte(byte(len(nameBytes)))
		buf.Write(nameBytes)
		binary.Write(&buf, binary.LittleEndian, r.Value)
		binary.Write(&buf, binary.LittleEndian, uint32(r.BitWidth))
	}

	// Memory: uncompressed length, then gzip-compressed data
	binary.Write(&buf, binary.LittleEndian, uint32(len(snap.Memory)))

	var compressed bytes.Buffer
	gz := gzip.NewWriter(&compressed)
	if _, err := gz.Write(snap.Memory); err != nil {
		return fmt.Errorf("compressing memory: %w", err)
	}
	if err := gz.Close(); err != nil {
		return fmt.Errorf("closing gzip: %w", err)
	}
	buf.Write(compressed.Bytes())

	return os.WriteFile(path, buf.Bytes(), 0644)
}

// LoadSnapshotFromFile reads and decompresses a snapshot from disk.
func LoadSnapshotFromFile(path string) (*MachineSnapshot, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	r := bytes.NewReader(data)

	// Magic
	magic := make([]byte, 4)
	if _, err := io.ReadFull(r, magic); err != nil {
		return nil, fmt.Errorf("reading magic: %w", err)
	}
	if string(magic) != snapshotMagic {
		return nil, fmt.Errorf("invalid snapshot magic: %q", string(magic))
	}

	// Version
	var version uint32
	if err := binary.Read(r, binary.LittleEndian, &version); err != nil {
		return nil, fmt.Errorf("reading version: %w", err)
	}
	if version != snapshotVersion {
		return nil, fmt.Errorf("unsupported snapshot version: %d", version)
	}

	// CPU type
	cpuTypeLen, err := r.ReadByte()
	if err != nil {
		return nil, fmt.Errorf("reading CPU type length: %w", err)
	}
	cpuType := make([]byte, cpuTypeLen)
	if _, err := io.ReadFull(r, cpuType); err != nil {
		return nil, fmt.Errorf("reading CPU type: %w", err)
	}

	// Registers
	var regCount uint32
	if err := binary.Read(r, binary.LittleEndian, &regCount); err != nil {
		return nil, fmt.Errorf("reading register count: %w", err)
	}

	regs := make([]RegisterInfo, regCount)
	for i := range regCount {
		nameLen, err := r.ReadByte()
		if err != nil {
			return nil, fmt.Errorf("reading register name length: %w", err)
		}
		name := make([]byte, nameLen)
		if _, err := io.ReadFull(r, name); err != nil {
			return nil, fmt.Errorf("reading register name: %w", err)
		}
		var value uint64
		if err := binary.Read(r, binary.LittleEndian, &value); err != nil {
			return nil, fmt.Errorf("reading register value: %w", err)
		}
		var bitWidth uint32
		if err := binary.Read(r, binary.LittleEndian, &bitWidth); err != nil {
			return nil, fmt.Errorf("reading register width: %w", err)
		}
		regs[i] = RegisterInfo{
			Name:     string(name),
			Value:    value,
			BitWidth: int(bitWidth),
		}
	}

	// Memory
	var uncompressedLen uint32
	if err := binary.Read(r, binary.LittleEndian, &uncompressedLen); err != nil {
		return nil, fmt.Errorf("reading memory length: %w", err)
	}

	remaining := data[len(data)-r.Len():]
	gz, err := gzip.NewReader(bytes.NewReader(remaining))
	if err != nil {
		return nil, fmt.Errorf("opening gzip reader: %w", err)
	}
	defer gz.Close()

	mem := make([]byte, uncompressedLen)
	if _, err := io.ReadFull(gz, mem); err != nil {
		return nil, fmt.Errorf("decompressing memory: %w", err)
	}

	return &MachineSnapshot{
		CPUType:   string(cpuType),
		Registers: regs,
		Memory:    mem,
	}, nil
}
