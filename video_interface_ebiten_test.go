//go:build !headless

package main

import "testing"

func TestKeyboardInput_EbitenImplements(t *testing.T) {
	eo := &EbitenOutput{}
	if _, ok := any(eo).(KeyboardInput); !ok {
		t.Fatal("expected EbitenOutput to implement KeyboardInput")
	}
}
