package main

import (
	"math"
	"testing"
)

func almostEq(a, b float64) bool {
	if math.IsNaN(a) && math.IsNaN(b) {
		return true
	}
	if math.IsInf(a, 0) || math.IsInf(b, 0) {
		return math.IsInf(a, 1) == math.IsInf(b, 1) && math.IsInf(a, -1) == math.IsInf(b, -1)
	}
	d := math.Abs(a - b)
	return d <= 1e-12 || d <= math.Abs(b)*1e-12
}

func TestX87_Init(t *testing.T) {
	f := NewFPU_X87()
	if f.FCW != 0x037F {
		t.Fatalf("FCW = 0x%04X, want 0x037F", f.FCW)
	}
	if f.FSW != 0 {
		t.Fatalf("FSW = 0x%04X, want 0", f.FSW)
	}
	if f.FTW != 0xFFFF {
		t.Fatalf("FTW = 0x%04X, want 0xFFFF", f.FTW)
	}
	if f.top() != 0 {
		t.Fatalf("TOP = %d, want 0", f.top())
	}
}

func TestX87_PushPopAndIndexing(t *testing.T) {
	f := NewFPU_X87()
	f.push(1)
	f.push(2)
	f.push(3)
	if f.top() != 5 {
		t.Fatalf("TOP = %d, want 5", f.top())
	}
	if f.ST(0) != 3 || f.ST(1) != 2 || f.ST(2) != 1 {
		t.Fatalf("unexpected ST order: ST0=%v ST1=%v ST2=%v", f.ST(0), f.ST(1), f.ST(2))
	}
	if f.pop() != 3 {
		t.Fatalf("pop 1 mismatch")
	}
	if f.pop() != 2 {
		t.Fatalf("pop 2 mismatch")
	}
	if f.pop() != 1 {
		t.Fatalf("pop 3 mismatch")
	}
}

func TestX87_StackOverUnderflow(t *testing.T) {
	f := NewFPU_X87()
	for i := range 8 {
		f.push(float64(i))
	}
	f.push(9)
	if (f.FSW & (x87FSW_IE | x87FSW_SF | x87FSW_C1)) != (x87FSW_IE | x87FSW_SF | x87FSW_C1) {
		t.Fatalf("overflow flags FSW=0x%04X", f.FSW)
	}

	f = NewFPU_X87()
	_ = f.pop()
	if (f.FSW & (x87FSW_IE | x87FSW_SF)) != (x87FSW_IE | x87FSW_SF) {
		t.Fatalf("underflow flags FSW=0x%04X", f.FSW)
	}
	if f.FSW&x87FSW_C1 != 0 {
		t.Fatalf("underflow should clear C1, FSW=0x%04X", f.FSW)
	}
}

func TestX87_TagWordClassification(t *testing.T) {
	f := NewFPU_X87()
	f.push(1.0)
	if f.getTag(f.physReg(0)) != x87TagValid {
		t.Fatal("1.0 should be valid")
	}
	f.pop()
	f.push(0.0)
	if f.getTag(f.physReg(0)) != x87TagZero {
		t.Fatal("0.0 should be zero tag")
	}
	f.pop()
	f.push(math.Inf(1))
	if f.getTag(f.physReg(0)) != x87TagSpecial {
		t.Fatal("inf should be special tag")
	}
}

func TestX87_FloatRoundtrip(t *testing.T) {
	bus := NewTestX86Bus()
	f := NewFPU_X87()

	vals := []float64{1.0, -1.0, 0.0, math.Inf(1), math.Inf(-1), math.NaN(), math.Pi}
	for i, v := range vals {
		addr := uint32(0x100 + i*16)
		f.storeFloat64(bus, addr, v)
		got := f.loadFloat64(bus, addr)
		if !(almostEq(got, v) || (math.IsNaN(got) && math.IsNaN(v))) {
			t.Fatalf("float64[%d] roundtrip got=%v want=%v", i, got, v)
		}

		f.storeFloat32(bus, addr+8, v)
		got32 := f.loadFloat32(bus, addr+8)
		if !(almostEq(got32, float64(float32(v))) || (math.IsNaN(got32) && math.IsNaN(v))) {
			t.Fatalf("float32[%d] roundtrip got=%v want=%v", i, got32, v)
		}
	}
}

func TestX87_Extended80Roundtrip(t *testing.T) {
	bus := NewTestX86Bus()
	f := NewFPU_X87()
	vals := []float64{math.Pi, 1.0, 0.0, math.Inf(1), math.NaN()}
	for i, v := range vals {
		addr := uint32(0x200 + i*16)
		f.storeExtended80(bus, addr, v)
		got := f.loadExtended80(bus, addr)
		if !(almostEq(got, v) || (math.IsNaN(got) && math.IsNaN(v))) {
			t.Fatalf("extended80[%d] roundtrip got=%v want=%v", i, got, v)
		}
	}
}

func TestX87_IntAndBCDRoundtrip(t *testing.T) {
	bus := NewTestX86Bus()
	f := NewFPU_X87()

	f.storeInt16(bus, 0x300, 32767)
	if got := f.loadInt16(bus, 0x300); got != 32767 {
		t.Fatalf("int16 roundtrip got=%v", got)
	}
	f.storeInt32(bus, 0x310, -123456)
	if got := f.loadInt32(bus, 0x310); got != -123456 {
		t.Fatalf("int32 roundtrip got=%v", got)
	}
	f.storeInt64(bus, 0x320, 1<<40)
	if got := f.loadInt64(bus, 0x320); got != float64(int64(1<<40)) {
		t.Fatalf("int64 roundtrip got=%v", got)
	}

	f.storeBCD(bus, 0x330, -123456789012345678)
	if got := f.loadBCD(bus, 0x330); got != -123456789012345678 {
		t.Fatalf("bcd roundtrip got=%v", got)
	}
}

func TestX87_IntStoreRoundingAndOverflow(t *testing.T) {
	bus := NewTestX86Bus()
	f := NewFPU_X87()

	f.FCW = (f.FCW &^ x87FCW_RCMask) | (x87FCW_RCNearest << x87FCW_RCShift)
	f.storeInt32(bus, 0x400, 2.5)
	if got := int32(uint32(bus.Read(0x400)) | (uint32(bus.Read(0x401)) << 8) | (uint32(bus.Read(0x402)) << 16) | (uint32(bus.Read(0x403)) << 24)); got != 2 {
		t.Fatalf("nearest-even 2.5 got=%d want=2", got)
	}

	f.FCW = (f.FCW &^ x87FCW_RCMask) | (x87FCW_RCUp << x87FCW_RCShift)
	f.storeInt32(bus, 0x404, 2.5)
	if got := int32(uint32(bus.Read(0x404)) | (uint32(bus.Read(0x405)) << 8) | (uint32(bus.Read(0x406)) << 16) | (uint32(bus.Read(0x407)) << 24)); got != 3 {
		t.Fatalf("round-up 2.5 got=%d want=3", got)
	}

	f.FSW = 0
	f.storeInt16(bus, 0x410, 1e20)
	if f.FSW&x87FSW_IE == 0 {
		t.Fatalf("expected IE on int overflow")
	}
}
