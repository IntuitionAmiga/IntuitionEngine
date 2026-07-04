//go:build !linux

package main

func adviseHugePages(mem []byte) {}
