//go:build darwin

package main

import (
	"fmt"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
)

const darwinMAPJIT = 0x800

var (
	darwinJITInitOnce sync.Once
	darwinJITInitErr  error

	libSystemHandle            uintptr
	pthreadJITWriteProtectNPFn func(int32) int32
	sysICacheInvalidateFn      func(unsafe.Pointer, uintptr)
)

func initDarwinJITSyscalls() error {
	darwinJITInitOnce.Do(func() {
		defer func() {
			if r := recover(); r != nil {
				darwinJITInitErr = fmt.Errorf("initialize darwin JIT syscalls: %v", r)
			}
		}()

		var err error
		libSystemHandle, err = purego.Dlopen("/usr/lib/libSystem.B.dylib", purego.RTLD_NOW|purego.RTLD_GLOBAL)
		if err != nil {
			darwinJITInitErr = fmt.Errorf("dlopen libSystem.B.dylib: %w", err)
			return
		}

		purego.RegisterLibFunc(&pthreadJITWriteProtectNPFn, libSystemHandle, "pthread_jit_write_protect_np")
		purego.RegisterLibFunc(&sysICacheInvalidateFn, libSystemHandle, "sys_icache_invalidate")
	})

	return darwinJITInitErr
}

func darwinSetJITWriteProtect(enabled bool) error {
	if err := initDarwinJITSyscalls(); err != nil {
		return err
	}
	mode := int32(0)
	if enabled {
		mode = 1
	}
	if rc := pthreadJITWriteProtectNPFn(mode); rc != 0 {
		return fmt.Errorf("pthread_jit_write_protect_np(%d) failed: %d", mode, rc)
	}
	return nil
}

func darwinICacheInvalidate(addr uintptr, size uintptr) error {
	if size == 0 {
		return nil
	}
	if err := initDarwinJITSyscalls(); err != nil {
		return err
	}
	sysICacheInvalidateFn(unsafe.Pointer(addr), size)
	return nil
}
