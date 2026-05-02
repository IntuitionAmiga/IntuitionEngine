// ahx_tables.go - Live AHX period and vibrato tables.

package main

// Period2Freq converts an AHX period to frequency.
// The Paula clock is the NTSC colorburst frequency * 2.
const AHXPeriod2FreqClock = 3579545.25

func AHXPeriod2Freq(period int) float64 {
	if period <= 0 {
		return 0
	}
	return AHXPeriod2FreqClock / float64(period)
}

// AHXVibratoTable is the 64-entry vibrato table (sine-like).
var AHXVibratoTable = [64]int{
	0, 24, 49, 74, 97, 120, 141, 161, 180, 197, 212, 224, 235, 244, 250, 253, 255,
	253, 250, 244, 235, 224, 212, 197, 180, 161, 141, 120, 97, 74, 49, 24,
	0, -24, -49, -74, -97, -120, -141, -161, -180, -197, -212, -224, -235, -244, -250, -253, -255,
	-253, -250, -244, -235, -224, -212, -197, -180, -161, -141, -120, -97, -74, -49, -24,
}

// AHXPeriodTable is the note-to-period lookup table (61 entries, notes 0-60).
var AHXPeriodTable = [61]int{
	0x0000, 0x0D60, 0x0CA0, 0x0BE8, 0x0B40, 0x0A98, 0x0A00, 0x0970,
	0x08E8, 0x0868, 0x07F0, 0x0780, 0x0714, 0x06B0, 0x0650, 0x05F4,
	0x05A0, 0x054C, 0x0500, 0x04B8, 0x0474, 0x0434, 0x03F8, 0x03C0,
	0x038A, 0x0358, 0x0328, 0x02FA, 0x02D0, 0x02A6, 0x0280, 0x025C,
	0x023A, 0x021A, 0x01FC, 0x01E0, 0x01C5, 0x01AC, 0x0194, 0x017D,
	0x0168, 0x0153, 0x0140, 0x012E, 0x011D, 0x010D, 0x00FE, 0x00F0,
	0x00E2, 0x00D6, 0x00CA, 0x00BE, 0x00B4, 0x00AA, 0x00A0, 0x0097,
	0x008F, 0x0087, 0x007F, 0x0078, 0x0071,
}
