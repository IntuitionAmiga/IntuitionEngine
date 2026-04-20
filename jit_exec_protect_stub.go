//go:build !(darwin && arm64)

package main

func jitPrepareForExec() {}

func jitFinishExec() {}

func jitPrepareForWrite() error { return nil }

func jitFinishWrite() error { return nil }
