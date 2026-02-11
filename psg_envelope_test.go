// psg_envelope_test.go - Tests for AY/YM envelope shapes.

package main

import "testing"

func collectEnvelopeLevels(shape uint8, steps int) []int {
	engine := NewPSGEngine(nil, SAMPLE_RATE)
	engine.SetClockHz(uint32(SAMPLE_RATE * 256))
	engine.WriteRegister(11, 0x01)
	engine.WriteRegister(12, 0x00)
	engine.WriteRegister(13, shape)

	levels := make([]int, 0, steps+1)
	levels = append(levels, engine.envLevel)
	for range steps {
		engine.TickSample()
		levels = append(levels, engine.envLevel)
	}
	return levels
}

func TestPSGEnvelopeShapesBehavior(t *testing.T) {
	for shape := range 16 {
		levels := collectEnvelopeLevels(uint8(shape), 32)
		cont := shape&0x08 != 0
		attack := shape&0x04 != 0
		alt := shape&0x02 != 0
		hold := shape&0x01 != 0

		start := 15
		end := 0
		if attack {
			start = 0
			end = 15
		}
		if levels[0] != start {
			t.Fatalf("shape 0x%X start=%d, want %d", shape, levels[0], start)
		}

		if !cont {
			held := levels[len(levels)-1]
			if held != 0 {
				t.Fatalf("shape 0x%X should hold at 0, got %d", shape, held)
			}
			continue
		}

		if hold {
			held := levels[len(levels)-1]
			if held != end && held != start {
				t.Fatalf("shape 0x%X hold at boundary, got %d", shape, held)
			}
			continue
		}

		if alt {
			if levels[16] != end {
				t.Fatalf("shape 0x%X alt should reach end at step 16, got %d", shape, levels[16])
			}
			if levels[32] != start {
				t.Fatalf("shape 0x%X alt should return to start at step 32, got %d", shape, levels[32])
			}
		} else {
			if levels[16] != start {
				t.Fatalf("shape 0x%X should wrap to start at step 16, got %d", shape, levels[16])
			}
		}
	}
}
