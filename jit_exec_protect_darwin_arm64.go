//go:build darwin && arm64

package main

import "runtime"

func jitPrepareForExec() {
	runtime.LockOSThread()
	if err := darwinSetJITWriteProtect(true); err != nil {
		runtime.UnlockOSThread()
		panic(err)
	}
}

func jitFinishExec() {
	runtime.UnlockOSThread()
}

func jitPrepareForWrite() error {
	runtime.LockOSThread()
	if err := darwinSetJITWriteProtect(false); err != nil {
		runtime.UnlockOSThread()
		return err
	}
	return nil
}

func jitFinishWrite() error {
	defer runtime.UnlockOSThread()
	return darwinSetJITWriteProtect(true)
}
