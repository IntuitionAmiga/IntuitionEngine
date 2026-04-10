package main

import (
	"encoding/binary"
	"strings"
	"testing"
)

type m14ELFSegmentSpec struct {
	Vaddr  uint64
	Flags  uint32
	Data   []byte
	Memsz  uint64
	Type   uint32
	Align  uint64
	Offset uint64
}

func makeM14ELFFixture(t *testing.T, entry uint64, segs []m14ELFSegmentSpec) []byte {
	t.Helper()
	if len(segs) == 0 {
		t.Fatal("need at least one segment")
	}
	const headerSize = m14ELFHeaderSize
	const phSize = m14ELFProgHdrSize

	phoff := uint64(headerSize)
	tableSize := uint64(len(segs)) * phSize
	imageSize := uint64(m14ELFPageAlign)
	for i := range segs {
		if segs[i].Type == 0 {
			segs[i].Type = m14ELFPTLoad
		}
		if segs[i].Align == 0 {
			segs[i].Align = m14ELFPageAlign
		}
		if segs[i].Memsz == 0 {
			segs[i].Memsz = uint64(len(segs[i].Data))
		}
		if segs[i].Offset == 0 {
			segs[i].Offset = uint64(i+1) * uint64(m14ELFPageAlign)
		}
		end := segs[i].Offset + uint64(len(segs[i].Data))
		if end > imageSize {
			imageSize = end
		}
	}
	if imageSize < phoff+tableSize {
		imageSize = phoff + tableSize
	}
	image := make([]byte, imageSize)
	copy(image[:4], []byte{0x7f, 'E', 'L', 'F'})
	image[4] = m14ELFClass64
	image[5] = m14ELFDataLSB
	image[6] = m14ELFVersion
	image[7] = m14ELFOSABISysV
	binary.LittleEndian.PutUint16(image[16:18], m14ELFTypeExec)
	binary.LittleEndian.PutUint16(image[18:20], m14ELFMachineIE64)
	binary.LittleEndian.PutUint32(image[20:24], m14ELFVersion)
	binary.LittleEndian.PutUint64(image[24:32], entry)
	binary.LittleEndian.PutUint64(image[32:40], phoff)
	binary.LittleEndian.PutUint16(image[52:54], m14ELFHeaderSize)
	binary.LittleEndian.PutUint16(image[54:56], m14ELFProgHdrSize)
	binary.LittleEndian.PutUint16(image[56:58], uint16(len(segs)))

	for i, seg := range segs {
		off := phoff + uint64(i)*phSize
		binary.LittleEndian.PutUint32(image[off:off+4], seg.Type)
		binary.LittleEndian.PutUint32(image[off+4:off+8], seg.Flags)
		binary.LittleEndian.PutUint64(image[off+8:off+16], seg.Offset)
		binary.LittleEndian.PutUint64(image[off+16:off+24], seg.Vaddr)
		binary.LittleEndian.PutUint64(image[off+24:off+32], seg.Vaddr)
		binary.LittleEndian.PutUint64(image[off+32:off+40], uint64(len(seg.Data)))
		binary.LittleEndian.PutUint64(image[off+40:off+48], seg.Memsz)
		binary.LittleEndian.PutUint64(image[off+48:off+56], seg.Align)
		copy(image[seg.Offset:seg.Offset+uint64(len(seg.Data))], seg.Data)
	}
	return image
}

func TestIExec_M14_Phase1_ELFValidAccepted(t *testing.T) {
	image := makeM14ELFFixture(t, 0x00601000, []m14ELFSegmentSpec{
		{Vaddr: 0x00601000, Flags: m14ELFSegFlagR | m14ELFSegFlagX, Data: []byte{1, 2, 3, 4}},
		{Vaddr: 0x00602000, Flags: m14ELFSegFlagR | m14ELFSegFlagW, Data: []byte{5, 6}, Memsz: 16},
	})
	if err := validateM14ELFContract(image); err != nil {
		t.Fatalf("valid ELF rejected: %v", err)
	}
}

func TestIExec_M14_Phase1_ELFBadMagicRejected(t *testing.T) {
	image := makeM14ELFFixture(t, 0x00601000, []m14ELFSegmentSpec{
		{Vaddr: 0x00601000, Flags: m14ELFSegFlagR | m14ELFSegFlagX, Data: []byte{1}},
	})
	image[0] = 0
	err := validateM14ELFContract(image)
	if err == nil || !strings.Contains(err.Error(), "magic") {
		t.Fatalf("expected bad magic rejection, got %v", err)
	}
}

func TestIExec_M14_Phase1_ELFWrongMachineRejected(t *testing.T) {
	image := makeM14ELFFixture(t, 0x00601000, []m14ELFSegmentSpec{
		{Vaddr: 0x00601000, Flags: m14ELFSegFlagR | m14ELFSegFlagX, Data: []byte{1}},
	})
	binary.LittleEndian.PutUint16(image[18:20], 0x003E)
	err := validateM14ELFContract(image)
	if err == nil || !strings.Contains(err.Error(), "machine") {
		t.Fatalf("expected wrong-machine rejection, got %v", err)
	}
}

func TestIExec_M14_Phase1_ELFWrongClassEndiannessVersionRejected(t *testing.T) {
	cases := []struct {
		name string
		mut  func([]byte)
		want string
	}{
		{"class", func(b []byte) { b[4] = 1 }, "class"},
		{"endianness", func(b []byte) { b[5] = 2 }, "endianness"},
		{"ident-version", func(b []byte) { b[6] = 2 }, "version"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			image := makeM14ELFFixture(t, 0x00601000, []m14ELFSegmentSpec{
				{Vaddr: 0x00601000, Flags: m14ELFSegFlagR | m14ELFSegFlagX, Data: []byte{1}},
			})
			tc.mut(image)
			err := validateM14ELFContract(image)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected %s rejection, got %v", tc.want, err)
			}
		})
	}
}

func TestIExec_M14_Phase1_ELFMalformedProgramHeaderRejected(t *testing.T) {
	image := makeM14ELFFixture(t, 0x00601000, []m14ELFSegmentSpec{
		{Vaddr: 0x00601000, Flags: m14ELFSegFlagR | m14ELFSegFlagX, Data: []byte{1, 2, 3, 4}},
	})
	phoff := binary.LittleEndian.Uint64(image[32:40])
	binary.LittleEndian.PutUint64(image[phoff+32:phoff+40], 8)
	binary.LittleEndian.PutUint64(image[phoff+40:phoff+48], 4)
	err := validateM14ELFContract(image)
	if err == nil || !strings.Contains(err.Error(), "filesz exceeds memsz") {
		t.Fatalf("expected malformed phdr rejection, got %v", err)
	}
}

func TestIExec_M14_Phase1_ELFProgramHeaderOffsetOverflowRejected(t *testing.T) {
	image := makeM14ELFFixture(t, 0x00601000, []m14ELFSegmentSpec{
		{Vaddr: 0x00601000, Flags: m14ELFSegFlagR | m14ELFSegFlagX, Data: []byte{1, 2, 3, 4}},
	})
	binary.LittleEndian.PutUint64(image[32:40], ^uint64(0)-15)
	err := validateM14ELFContract(image)
	if err == nil || !strings.Contains(err.Error(), "program header table out of range") {
		t.Fatalf("expected overflowing phoff rejection, got %v", err)
	}
}

func TestIExec_M14_Phase1_ELFUnsupportedDynamicFeaturesRejected(t *testing.T) {
	image := makeM14ELFFixture(t, 0x00601000, []m14ELFSegmentSpec{
		{Vaddr: 0x00601000, Flags: m14ELFSegFlagR | m14ELFSegFlagX, Data: []byte{1, 2, 3, 4}},
		{Vaddr: 0x00602000, Flags: m14ELFSegFlagR, Data: []byte{0}, Type: m14ELFPTDynamic},
	})
	err := validateM14ELFContract(image)
	if err == nil || !strings.Contains(err.Error(), "dynamic") {
		t.Fatalf("expected dynamic-feature rejection, got %v", err)
	}
}

func TestIExec_M14_Phase1_ELFSegmentRangeOverflowRejected(t *testing.T) {
	image := makeM14ELFFixture(t, 0x00601000, []m14ELFSegmentSpec{
		{Vaddr: 0x00601000, Flags: m14ELFSegFlagR | m14ELFSegFlagX, Data: []byte{1, 2, 3, 4}},
	})
	phoff := binary.LittleEndian.Uint64(image[32:40])
	binary.LittleEndian.PutUint64(image[phoff+16:phoff+24], m14ELFUserLimit-0x1000)
	binary.LittleEndian.PutUint64(image[phoff+40:phoff+48], ^uint64(0))
	err := validateM14ELFContract(image)
	if err == nil || !strings.Contains(err.Error(), "segment outside user image space") {
		t.Fatalf("expected segment overflow rejection, got %v", err)
	}
}
