package main

type PSGEvent struct {
	Sample uint64
	Reg    uint8
	Value  uint8
}

type SNEvent struct {
	Sample uint64
	Byte   uint8
}
