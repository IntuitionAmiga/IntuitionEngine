//go:build (amd64 && (linux || windows || darwin)) || (arm64 && (linux || windows || darwin))

package main

func (c *CPU_X86) trySharedX86MMIOPollMatch(adapter *X86BusAdapter, addr uint32) (bool, int) {
	pattern := X86PollPattern
	pattern.AddressIsMMIOPredicate = func(addr uint32) bool {
		hostAddr := adapter.translateIO(addr)
		return adapter.bus.IsIOAddress(hostAddr) ||
			(hostAddr == 0xF0008 && adapter.bus.videoStatusReader != nil)
	}
	match := TryFastMMIOPoll([]PollInstr{
		{Kind: PollInstrLoad, LoadShape: PollLoad32, LoadAddr: addr},
		{Kind: PollInstrTest, TestShape: PollTestCMPImm},
		{Kind: PollInstrBranchBackward},
	}, &pattern)
	return match.Matched, pattern.IterationCap
}
