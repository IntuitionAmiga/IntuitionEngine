package main

func applyRuntimeVisibleRAMForMode(bus *MachineBus, mode string) {
	if bus == nil {
		return
	}

	switch mode {
	case "aros":
		bus.ApplyProfileVisibleCeiling(clampToBusMemoryWindow(arosProfileTopBytes, bus))
	case "emutos":
		bus.ApplyProfileVisibleCeiling(clampToBusMemoryWindow(EmuTOSProfileTopBytes, bus))
	case "ie64", "intuitionos":
		bus.ApplyProfileVisibleCeiling(bus.TotalGuestRAM())
	}
}

func clampToBusMemoryWindow(ceiling uint64, bus *MachineBus) uint64 {
	if bus == nil {
		return 0
	}
	busMem := uint64(len(bus.memory))
	if ceiling > busMem {
		return busMem
	}
	return ceiling
}
