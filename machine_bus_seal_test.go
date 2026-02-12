package main

import "testing"

func expectPanic(t *testing.T, fn func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic, got none")
		}
	}()
	fn()
}

func TestMachineBus_SealPanicsOnLateMapIO(t *testing.T) {
	bus := NewMachineBus()
	bus.SealMappings()

	expectPanic(t, func() {
		bus.MapIO(0x1000, 0x10FF, nil, nil)
	})
	expectPanic(t, func() {
		bus.MapIO64(0x2000, 0x20FF, nil, nil)
	})
}
