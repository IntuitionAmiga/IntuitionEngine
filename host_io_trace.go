package main

import (
	"fmt"
	"os"
)

func traceHostIO(component, op, guestPath, hostPath string, err error, size int) {
	if os.Getenv("IE_TRACE_HOSTIO") == "" {
		return
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s %s %q -> %q error=%v\n", component, op, guestPath, hostPath, err)
		return
	}
	fmt.Fprintf(os.Stderr, "%s %s %q -> %q size=%d\n", component, op, guestPath, hostPath, size)
}
