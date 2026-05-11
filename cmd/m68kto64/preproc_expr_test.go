package main

import "testing"

func TestExpr_Literals(t *testing.T) {
	cases := []struct {
		src  string
		want int64
	}{
		{"0", 0},
		{"42", 42},
		{"$ff", 255},
		{"$FF", 255},
		{"0x10", 16},
		{"0X10", 16},
		{"%1010", 10},
		{"%0", 0},
		{"-5", -5},
		{"+7", 7},
		{"~0", -1},
		{"!0", 1},
		{"!42", 0},
	}
	for _, c := range cases {
		got, err := EvalExpr(c.src, NewSymtab())
		if err != nil {
			t.Errorf("%q: unexpected err: %v", c.src, err)
			continue
		}
		if got != c.want {
			t.Errorf("%q: got %d, want %d", c.src, got, c.want)
		}
	}
}

func TestExpr_Operators(t *testing.T) {
	cases := []struct {
		src  string
		want int64
	}{
		{"1+2", 3},
		{"5-3", 2},
		{"4*3", 12},
		{"10/3", 3},
		{"10%3", 1},
		{"1+2*3", 7},
		{"(1+2)*3", 9},
		{"1<<3", 8},
		{"16>>2", 4},
		{"$f & $3", 3},
		{"$1 | $2", 3},
		{"$f ^ $5", 10},
		{"3==3", 1},
		{"3==4", 0},
		{"3=3", 1}, // vasm `=` equality
		{"3<>4", 1},
		{"3<>3", 0},
		{"3!=4", 1},
		{"1<2", 1},
		{"2<1", 0},
		{"2<=2", 1},
		{"3>=4", 0},
		{"1 && 0", 0},
		{"1 && 1", 1},
		{"0 || 1", 1},
		{"0 || 0", 0},
	}
	for _, c := range cases {
		got, err := EvalExpr(c.src, NewSymtab())
		if err != nil {
			t.Errorf("%q: unexpected err: %v", c.src, err)
			continue
		}
		if got != c.want {
			t.Errorf("%q: got %d, want %d", c.src, got, c.want)
		}
	}
}

func TestExpr_CharLiteral(t *testing.T) {
	cases := []struct {
		src  string
		want int64
	}{
		{"'a'", int64('a')},
		{"'A'", int64('A')},
		{"'0'", int64('0')},
		{"'m'", int64('m')},
		{"' '", int64(' ')},
		// Multi-char packed big-endian (vasm convention).
		{"'AB'", int64('A')<<8 | int64('B')},
		{"'ABCD'", int64('A')<<24 | int64('B')<<16 | int64('C')<<8 | int64('D')},
	}
	for _, c := range cases {
		got, err := EvalExpr(c.src, NewSymtab())
		if err != nil {
			t.Errorf("%q: %v", c.src, err)
			continue
		}
		if got != c.want {
			t.Errorf("%q: got %d (0x%x), want %d (0x%x)", c.src, got, got, c.want, c.want)
		}
	}
}

func TestExpr_SymbolLookup(t *testing.T) {
	st := NewSymtab()
	_ = st.SetEqu("FOO", 7)
	_ = st.SetMutable("BAR", 3)
	cases := []struct {
		src  string
		want int64
	}{
		{"FOO", 7},
		{"BAR", 3},
		{"FOO + BAR", 10},
		{"FOO * 2", 14},
		{"FOO > 3", 1},
	}
	for _, c := range cases {
		got, err := EvalExpr(c.src, st)
		if err != nil {
			t.Fatalf("%q: %v", c.src, err)
		}
		if got != c.want {
			t.Errorf("%q: got %d, want %d", c.src, got, c.want)
		}
	}
	if _, err := EvalExpr("UNKNOWN", st); err == nil {
		t.Errorf("expected error for undefined symbol")
	}
}

func TestSymtab_EquImmutable(t *testing.T) {
	st := NewSymtab()
	if err := st.SetEqu("X", 1); err != nil {
		t.Fatal(err)
	}
	if err := st.SetEqu("X", 2); err == nil {
		t.Errorf("expected redefinition error for equ")
	}
}

func TestSymtab_SetMutable(t *testing.T) {
	st := NewSymtab()
	if err := st.SetMutable("X", 1); err != nil {
		t.Fatal(err)
	}
	if err := st.SetMutable("X", 2); err != nil {
		t.Errorf("set should be mutable: %v", err)
	}
	v, _ := st.Get("X")
	if v != 2 {
		t.Errorf("got %d, want 2", v)
	}
}

func TestSymtab_EquAfterSetErrors(t *testing.T) {
	st := NewSymtab()
	_ = st.SetMutable("X", 1)
	if err := st.SetEqu("X", 2); err == nil {
		t.Errorf("equ on existing mutable name should error")
	}
}

func TestSymtab_SetAfterEquErrors(t *testing.T) {
	st := NewSymtab()
	_ = st.SetEqu("X", 1)
	if err := st.SetMutable("X", 2); err == nil {
		t.Errorf("set on existing equ name should error (vasm semantics)")
	}
}
