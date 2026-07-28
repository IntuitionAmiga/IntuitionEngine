package main

import (
	"os"
	"regexp"
	"testing"
)

func TestHostSocketModePolicy(t *testing.T) {
	for _, mode := range []string{"ie64", "ie32", "m68k", "x86", "6502", "z80", "intuitionos", "emutos", "aros"} {
		if !hostSocketModeEnabled(mode) {
			t.Errorf("%q must expose host sockets", mode)
		}
	}
	for _, mode := range []string{"basic", "", "unknown"} {
		if hostSocketModeEnabled(mode) {
			t.Errorf("%q must not expose host sockets", mode)
		}
	}
}

func TestHostSocketMappingTransitions(t *testing.T) {
	bus := NewMachineBus()
	mapping := NewHostSocketMapping(bus)
	mapping.Configure("ie64")
	mapping.Configure("emutos")
	mapping.Configure("basic")
	if mapping.mapped {
		t.Fatal("BASIC retained the socket mapping")
	}
	mapping.Configure("unknown")
}

func TestHostSocketMappingHonoursForceBasicBoot(t *testing.T) {
	bus := NewMachineBus()
	mapping := NewHostSocketMapping(bus)
	machine := NewMachine(MachineDeps{})
	machine.SetHostSocketMapping(mapping)

	mapping.Configure("ie64")
	if err := machine.ResetDevicesBeforeLoad("ie64", MachineDeviceResetTargets{
		Memory:         bus,
		ForceBasicBoot: true,
	}); err != nil {
		t.Fatal(err)
	}
	if mapping.mapped {
		t.Fatal("forced BASIC boot retained the host socket mapping")
	}
}

func TestHostSocketByteLaneAssembly(t *testing.T) {
	bus := NewMachineBus()
	dev := NewArosHostSocketDevice(bus, &disabledArosHostSocketBackend{}, true)
	bus.MapIO(AROS_HOST_SOCKET_REGION_BASE, AROS_HOST_SOCKET_REGION_END, dev.HandleRead, dev.HandleWrite)
	bus.MapIOByte(AROS_HOST_SOCKET_REGION_BASE, AROS_HOST_SOCKET_REGION_END, dev.HandleWrite8)
	bus.MapIOByteRead(AROS_HOST_SOCKET_REGION_BASE, AROS_HOST_SOCKET_REGION_END, dev.HandleRead8)

	for i, v := range []byte{0x12, 0x34, 0x56, 0x78} {
		bus.Write8(HOST_SOCKET_REQ_PTR+uint32(i), v)
	}
	if got := bus.Read32(HOST_SOCKET_REQ_PTR); got != 0x12345678 {
		t.Fatalf("REQ_PTR=%#x", got)
	}
	for i, v := range []byte{0, 0, 0, HOST_SOCKET_CMD_SOCKET} {
		bus.Write8(HOST_SOCKET_CMD+uint32(i), v)
		if i != 3 && dev.errno != 0 {
			t.Fatal("partial command dispatched")
		}
	}
	if dev.errno != arosSockErrInval {
		t.Fatalf("complete command errno=%d, want EINVAL", dev.errno)
	}
}

func TestHostSocketBankThreeAdapters(t *testing.T) {
	for name, newAdapter := range map[string]func(*MachineBus) interface {
		Write(uint16, byte)
		Read(uint16) byte
	}{
		"6502": func(bus *MachineBus) interface {
			Write(uint16, byte)
			Read(uint16) byte
		} {
			return NewBus6502Adapter(bus)
		},
		"z80": func(bus *MachineBus) interface {
			Write(uint16, byte)
			Read(uint16) byte
		} {
			return NewZ80BusAdapter(bus)
		},
	} {
		t.Run(name, func(t *testing.T) {
			bus := NewMachineBus()
			mapping := NewHostSocketMapping(bus)
			mapping.Configure(name)
			adapter := newAdapter(bus)
			adapter.Write(0xF704, 0x79)
			adapter.Write(0xF705, 0)
			for i, v := range []byte{0x12, 0x34, 0x56, 0x78} {
				adapter.Write(0x6504+uint16(i), v)
			}
			for i, want := range []byte{0x12, 0x34, 0x56, 0x78} {
				if got := adapter.Read(0x6504 + uint16(i)); got != want {
					t.Fatalf("REQ_PTR byte %d=%#x, want %#x", i, got, want)
				}
			}
		})
	}
}

func TestHostSocketIncludesExportResolverDescriptorOffsets(t *testing.T) {
	offsets := map[string]string{
		"HOST_SOCKET_REQ_HOSTENT_NAME_PTR":  "64",
		"HOST_SOCKET_REQ_HOSTENT_NAME_CAP":  "68",
		"HOST_SOCKET_REQ_HOSTENT_ADDRS_PTR": "72",
		"HOST_SOCKET_REQ_HOSTENT_ADDRS_MAX": "76",
	}
	for _, path := range []string{
		"sdk/include/ie64.inc", "sdk/include/ie32.inc", "sdk/include/ie68.inc",
		"sdk/include/ie86.inc", "sdk/include/ie65.inc", "sdk/include/ie80.inc",
	} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for name, value := range offsets {
			pattern := `(?m)^(?:\.equ\s+|\.set\s+)?` + name + `(?:\s+equ\s+|\s*=\s*|,\s*|\s+)` + value + `$`
			if !regexp.MustCompile(pattern).Match(data) {
				t.Errorf("%s missing %s=%s", path, name, value)
			}
		}
	}
}
