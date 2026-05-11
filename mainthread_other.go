//go:build !darwin || headless

package main

func lockMainThread() {}

func mainThreadCall(fn func()) { go fn() }

func driveMainThread(block func()) { block() }
