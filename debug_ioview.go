// debug_ioview.go - I/O register viewer for Machine Monitor

package main

import "fmt"

// IORegisterDesc describes a single I/O register for display.
type IORegisterDesc struct {
	Name   string
	Addr   uint32
	Width  int    // 1, 2, or 4 bytes
	Access string // "RW", "RO", "WO"
}

// IODeviceDesc describes a group of I/O registers for a device.
type IODeviceDesc struct {
	Name      string
	Registers []IORegisterDesc
}

var ioDevices = map[string]*IODeviceDesc{
	"video": {
		Name: "VideoChip",
		Registers: []IORegisterDesc{
			{"CTRL", 0xF0000, 4, "RW"},
			{"MODE", 0xF0004, 4, "RW"},
			{"STATUS", 0xF0008, 4, "RO"},
			{"COPPER_CTRL", 0xF000C, 4, "RW"},
			{"COPPER_PTR", 0xF0010, 4, "RW"},
			{"COPPER_PC", 0xF0014, 4, "RO"},
			{"COPPER_STATUS", 0xF0018, 4, "RO"},
			{"BLT_CTRL", 0xF001C, 4, "RW"},
			{"BLT_OP", 0xF0020, 4, "RW"},
			{"BLT_SRC", 0xF0024, 4, "RW"},
			{"BLT_DST", 0xF0028, 4, "RW"},
			{"BLT_WIDTH", 0xF002C, 4, "RW"},
			{"BLT_HEIGHT", 0xF0030, 4, "RW"},
			{"BLT_STATUS", 0xF0044, 4, "RO"},
		},
	},
	"audio": {
		Name: "AudioChip",
		Registers: []IORegisterDesc{
			{"CTRL", 0xF0800, 4, "RW"},
		},
	},
	"psg": {
		Name: "PSG",
		Registers: []IORegisterDesc{
			{"REG0_FREQ_A_LO", 0xF0C00, 1, "RW"},
			{"REG1_FREQ_A_HI", 0xF0C01, 1, "RW"},
			{"REG2_FREQ_B_LO", 0xF0C02, 1, "RW"},
			{"REG3_FREQ_B_HI", 0xF0C03, 1, "RW"},
			{"REG4_FREQ_C_LO", 0xF0C04, 1, "RW"},
			{"REG5_FREQ_C_HI", 0xF0C05, 1, "RW"},
			{"REG6_NOISE_PER", 0xF0C06, 1, "RW"},
			{"REG7_MIXER", 0xF0C07, 1, "RW"},
			{"REG8_AMP_A", 0xF0C08, 1, "RW"},
			{"REG9_AMP_B", 0xF0C09, 1, "RW"},
			{"REG10_AMP_C", 0xF0C0A, 1, "RW"},
			{"REG11_ENV_LO", 0xF0C0B, 1, "RW"},
			{"REG12_ENV_HI", 0xF0C0C, 1, "RW"},
			{"REG13_ENV_SHAPE", 0xF0C0D, 1, "RW"},
			{"PLUS_CTRL", 0xF0C0E, 1, "RW"},
			{"PLAY_PTR", 0xF0C10, 4, "RW"},
			{"PLAY_LEN", 0xF0C14, 4, "RW"},
			{"PLAY_CTRL", 0xF0C18, 4, "RW"},
			{"PLAY_STATUS", 0xF0C1C, 4, "RO"},
		},
	},
	"pokey": {
		Name: "POKEY",
		Registers: []IORegisterDesc{
			{"AUDF1", 0xF0D00, 1, "RW"},
			{"AUDC1", 0xF0D01, 1, "RW"},
			{"AUDF2", 0xF0D02, 1, "RW"},
			{"AUDC2", 0xF0D03, 1, "RW"},
			{"AUDF3", 0xF0D04, 1, "RW"},
			{"AUDC3", 0xF0D05, 1, "RW"},
			{"AUDF4", 0xF0D06, 1, "RW"},
			{"AUDC4", 0xF0D07, 1, "RW"},
			{"AUDCTL", 0xF0D08, 1, "RW"},
			{"PLUS_CTRL", 0xF0D09, 1, "RW"},
			{"PLAY_PTR", 0xF0D10, 4, "RW"},
			{"PLAY_LEN", 0xF0D14, 4, "RW"},
			{"PLAY_CTRL", 0xF0D18, 4, "RW"},
			{"PLAY_STATUS", 0xF0D1C, 4, "RO"},
		},
	},
	"sid": {
		Name: "SID",
		Registers: []IORegisterDesc{
			{"V1_FREQ_LO", 0xF0E00, 1, "WO"},
			{"V1_FREQ_HI", 0xF0E01, 1, "WO"},
			{"V1_PW_LO", 0xF0E02, 1, "WO"},
			{"V1_PW_HI", 0xF0E03, 1, "WO"},
			{"V1_CTRL", 0xF0E04, 1, "WO"},
			{"V1_AD", 0xF0E05, 1, "WO"},
			{"V1_SR", 0xF0E06, 1, "WO"},
			{"V2_FREQ_LO", 0xF0E07, 1, "WO"},
			{"V2_FREQ_HI", 0xF0E08, 1, "WO"},
			{"V2_PW_LO", 0xF0E09, 1, "WO"},
			{"V2_PW_HI", 0xF0E0A, 1, "WO"},
			{"V2_CTRL", 0xF0E0B, 1, "WO"},
			{"V2_AD", 0xF0E0C, 1, "WO"},
			{"V2_SR", 0xF0E0D, 1, "WO"},
			{"V3_FREQ_LO", 0xF0E0E, 1, "WO"},
			{"V3_FREQ_HI", 0xF0E0F, 1, "WO"},
			{"V3_PW_LO", 0xF0E10, 1, "WO"},
			{"V3_PW_HI", 0xF0E11, 1, "WO"},
			{"V3_CTRL", 0xF0E12, 1, "WO"},
			{"V3_AD", 0xF0E13, 1, "WO"},
			{"V3_SR", 0xF0E14, 1, "WO"},
			{"FC_LO", 0xF0E15, 1, "WO"},
			{"FC_HI", 0xF0E16, 1, "WO"},
			{"RES_FILT", 0xF0E17, 1, "WO"},
			{"MODE_VOL", 0xF0E18, 1, "WO"},
			{"PLUS_CTRL", 0xF0E19, 1, "RW"},
			{"PLAY_PTR", 0xF0E20, 4, "RW"},
			{"PLAY_LEN", 0xF0E24, 4, "RW"},
			{"PLAY_CTRL", 0xF0E28, 4, "RW"},
			{"PLAY_STATUS", 0xF0E2C, 4, "RO"},
		},
	},
	"ted": {
		Name: "TED",
		Registers: []IORegisterDesc{
			{"FREQ1_LO", 0xF0F00, 1, "RW"},
			{"FREQ2_LO", 0xF0F01, 1, "RW"},
			{"FREQ2_HI", 0xF0F02, 1, "RW"},
			{"SND_CTRL", 0xF0F03, 1, "RW"},
			{"FREQ1_HI", 0xF0F04, 1, "RW"},
			{"PLUS_CTRL", 0xF0F05, 1, "RW"},
			{"PLAY_PTR", 0xF0F10, 4, "RW"},
			{"PLAY_LEN", 0xF0F14, 4, "RW"},
			{"PLAY_CTRL", 0xF0F18, 4, "RW"},
			{"PLAY_STATUS", 0xF0F1C, 4, "RO"},
		},
	},
	"vga": {
		Name: "VGA",
		Registers: []IORegisterDesc{
			{"MODE", 0xF1000, 4, "RW"},
			{"STATUS", 0xF1004, 4, "RO"},
			{"CTRL", 0xF1008, 4, "RW"},
			{"SEQ_INDEX", 0xF1010, 4, "RW"},
			{"SEQ_DATA", 0xF1014, 4, "RW"},
			{"CRTC_INDEX", 0xF1020, 4, "RW"},
			{"CRTC_DATA", 0xF1024, 4, "RW"},
			{"GC_INDEX", 0xF1030, 4, "RW"},
			{"GC_DATA", 0xF1034, 4, "RW"},
			{"DAC_MASK", 0xF1050, 4, "RW"},
			{"DAC_RINDEX", 0xF1054, 4, "RW"},
			{"DAC_WINDEX", 0xF1058, 4, "RW"},
			{"DAC_DATA", 0xF105C, 4, "RW"},
		},
	},
	"ula": {
		Name: "ULA",
		Registers: []IORegisterDesc{
			{"BORDER", 0xF2000, 4, "RW"},
			{"CTRL", 0xF2004, 4, "RW"},
			{"STATUS", 0xF2008, 4, "RO"},
		},
	},
	"antic": {
		Name: "ANTIC",
		Registers: []IORegisterDesc{
			{"DMACTL", 0xF2100, 4, "RW"},
			{"CHACTL", 0xF2104, 4, "RW"},
			{"DLISTL", 0xF2108, 4, "RW"},
			{"DLISTH", 0xF210C, 4, "RW"},
			{"HSCROL", 0xF2110, 4, "RW"},
			{"VSCROL", 0xF2114, 4, "RW"},
			{"PMBASE", 0xF2118, 4, "RW"},
			{"CHBASE", 0xF211C, 4, "RW"},
			{"WSYNC", 0xF2120, 4, "WO"},
			{"VCOUNT", 0xF2124, 4, "RO"},
			{"NMIEN", 0xF2130, 4, "RW"},
			{"NMIST", 0xF2134, 4, "RO"},
			{"ENABLE", 0xF2138, 4, "RW"},
			{"STATUS", 0xF213C, 4, "RO"},
		},
	},
	"voodoo": {
		Name: "Voodoo",
		Registers: []IORegisterDesc{
			{"STATUS", 0xF4000, 4, "RO"},
			{"ENABLE", 0xF4004, 4, "WO"},
			{"VERTEX_AX", 0xF4008, 4, "RW"},
			{"VERTEX_AY", 0xF400C, 4, "RW"},
			{"VERTEX_BX", 0xF4010, 4, "RW"},
			{"VERTEX_BY", 0xF4014, 4, "RW"},
			{"VERTEX_CX", 0xF4018, 4, "RW"},
			{"VERTEX_CY", 0xF401C, 4, "RW"},
			{"START_R", 0xF4020, 4, "RW"},
			{"START_G", 0xF4024, 4, "RW"},
			{"START_B", 0xF4028, 4, "RW"},
			{"START_Z", 0xF402C, 4, "RW"},
			{"START_A", 0xF4030, 4, "RW"},
			{"TRIANGLE_CMD", 0xF4080, 4, "WO"},
			{"COLOR_SELECT", 0xF4088, 4, "RW"},
			{"FBZCOLOR_PATH", 0xF4104, 4, "RW"},
			{"FOG_MODE", 0xF4108, 4, "RW"},
			{"ALPHA_MODE", 0xF410C, 4, "RW"},
			{"FBZ_MODE", 0xF4110, 4, "RW"},
			{"LFB_MODE", 0xF4114, 4, "RW"},
			{"FAST_FILL_CMD", 0xF4124, 4, "WO"},
			{"SWAP_BUFFER", 0xF4128, 4, "WO"},
			{"TEX_MODE", 0xF4300, 4, "RW"},
		},
	},
}

// formatIOView renders the register view for a device.
func formatIOView(cpu DebuggableCPU, deviceName string) []string {
	dev, ok := ioDevices[deviceName]
	if !ok {
		return []string{fmt.Sprintf("Unknown device: %s", deviceName)}
	}

	var lines []string
	lines = append(lines, fmt.Sprintf("--- %s Registers ---", dev.Name))

	for _, reg := range dev.Registers {
		data := cpu.ReadMemory(uint64(reg.Addr), reg.Width)
		if len(data) < reg.Width {
			lines = append(lines, fmt.Sprintf("  %-16s ($%05X) = ??       [%s]", reg.Name, reg.Addr, reg.Access))
			continue
		}

		var val uint32
		switch reg.Width {
		case 1:
			val = uint32(data[0])
			lines = append(lines, fmt.Sprintf("  %-16s ($%05X) = $%02X       [%d] %s", reg.Name, reg.Addr, val, val, reg.Access))
		case 2:
			val = uint32(data[0]) | uint32(data[1])<<8
			lines = append(lines, fmt.Sprintf("  %-16s ($%05X) = $%04X     [%d] %s", reg.Name, reg.Addr, val, val, reg.Access))
		case 4:
			val = uint32(data[0]) | uint32(data[1])<<8 | uint32(data[2])<<16 | uint32(data[3])<<24
			lines = append(lines, fmt.Sprintf("  %-16s ($%05X) = $%08X [%d] %s", reg.Name, reg.Addr, val, val, reg.Access))
		}
	}

	return lines
}

// listIODevices returns the names of all available IO devices.
func listIODevices() []string {
	return []string{"video", "audio", "psg", "pokey", "sid", "ted", "vga", "ula", "antic", "voodoo"}
}
