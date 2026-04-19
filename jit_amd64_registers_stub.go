//go:build !linux && !(amd64 && (windows || darwin))

package main

const (
	amd64RAX = 0
	amd64RCX = 1
	amd64RDX = 2
	amd64RBX = 3
	amd64RSP = 4
	amd64RBP = 5
	amd64RSI = 6
	amd64RDI = 7
	amd64R8  = 8
	amd64R9  = 9
	amd64R10 = 10
	amd64R11 = 11
	amd64R12 = 12
	amd64R13 = 13
	amd64R14 = 14
	amd64R15 = 15
)
