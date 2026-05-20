package main

import (
	"fmt"
	"os"
)

var (
	hostIOTraceEnabled bool
	hostIOTraceFile    string
)

func traceHostIO(component, op, guestPath, hostPath string, err error, size int) {
	path := hostIOTraceFile
	if path == "" {
		path = os.Getenv("IE_TRACE_HOSTIO_FILE")
	}
	if !hostIOTraceEnabled && path == "" && os.Getenv("IE_TRACE_HOSTIO") == "" {
		return
	}
	line := ""
	if err != nil {
		line = fmt.Sprintf("%s %s %q -> %q error=%v\n", component, op, guestPath, hostPath, err)
	} else {
		line = fmt.Sprintf("%s %s %q -> %q size=%d\n", component, op, guestPath, hostPath, size)
	}
	if path != "" {
		if f, openErr := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644); openErr == nil {
			_, _ = f.WriteString(line)
			_ = f.Close()
		}
	}
	fmt.Fprint(os.Stderr, line)
}
