package main

import (
	"testing"
	"time"
)

func TestSawtoothWave(t *testing.T) {
	// Use the global chip from common_setup_test.go

	// Play a sawtooth note at 440Hz (A4) on channel 4 (sawtooth channel)
	t.Log("Playing sawtooth wave at 440Hz for 2 seconds...")

	// Set frequency (440Hz)
	chip.HandleRegisterWrite(SAW_FREQ, 440)

	// Set volume (75%)
	chip.HandleRegisterWrite(SAW_VOL, 192)

	// Set envelope - quick attack, medium sustain
	chip.HandleRegisterWrite(SAW_ATK, 10)  // Fast attack
	chip.HandleRegisterWrite(SAW_DEC, 50)  // Medium decay
	chip.HandleRegisterWrite(SAW_SUS, 180) // High sustain
	chip.HandleRegisterWrite(SAW_REL, 100) // Medium release

	// Gate on (bit 0 of CTRL)
	chip.HandleRegisterWrite(SAW_CTRL, 0x01)

	// Let it play for 2 seconds
	time.Sleep(2 * time.Second)

	// Gate off
	chip.HandleRegisterWrite(SAW_CTRL, 0x00)

	// Let release play out
	time.Sleep(500 * time.Millisecond)

	t.Log("Sawtooth test complete!")
}

func TestSawtoothSweep(t *testing.T) {
	t.Log("Playing sawtooth frequency sweep...")

	// Set volume
	chip.HandleRegisterWrite(SAW_VOL, 200)

	// Quick envelope
	chip.HandleRegisterWrite(SAW_ATK, 5)
	chip.HandleRegisterWrite(SAW_DEC, 20)
	chip.HandleRegisterWrite(SAW_SUS, 200)
	chip.HandleRegisterWrite(SAW_REL, 50)

	// Sweep from 110Hz to 880Hz
	for freq := 110; freq <= 880; freq += 10 {
		chip.HandleRegisterWrite(SAW_FREQ, uint32(freq))
		chip.HandleRegisterWrite(SAW_CTRL, 0x01) // Gate on

		time.Sleep(50 * time.Millisecond)
	}

	// Gate off
	chip.HandleRegisterWrite(SAW_CTRL, 0x00)
	time.Sleep(300 * time.Millisecond)

	t.Log("Sweep complete!")
}

func TestSawtoothVsSquare(t *testing.T) {
	t.Log("Comparing sawtooth vs square wave...")

	// First play square wave
	t.Log("Playing square wave at 440Hz...")
	chip.HandleRegisterWrite(SQUARE_FREQ, 440)
	chip.HandleRegisterWrite(SQUARE_VOL, 192)
	chip.HandleRegisterWrite(SQUARE_ATK, 10)
	chip.HandleRegisterWrite(SQUARE_DEC, 30)
	chip.HandleRegisterWrite(SQUARE_SUS, 180)
	chip.HandleRegisterWrite(SQUARE_REL, 50)
	chip.HandleRegisterWrite(SQUARE_CTRL, 0x01)
	time.Sleep(1500 * time.Millisecond)
	chip.HandleRegisterWrite(SQUARE_CTRL, 0x00)
	time.Sleep(500 * time.Millisecond)

	// Then play sawtooth
	t.Log("Playing sawtooth wave at 440Hz...")
	chip.HandleRegisterWrite(SAW_FREQ, 440)
	chip.HandleRegisterWrite(SAW_VOL, 192)
	chip.HandleRegisterWrite(SAW_ATK, 10)
	chip.HandleRegisterWrite(SAW_DEC, 30)
	chip.HandleRegisterWrite(SAW_SUS, 180)
	chip.HandleRegisterWrite(SAW_REL, 50)
	chip.HandleRegisterWrite(SAW_CTRL, 0x01)
	time.Sleep(1500 * time.Millisecond)
	chip.HandleRegisterWrite(SAW_CTRL, 0x00)
	time.Sleep(500 * time.Millisecond)

	t.Log("Comparison complete! You should have heard a square wave, then a sawtooth (brighter, buzzier)")
}
