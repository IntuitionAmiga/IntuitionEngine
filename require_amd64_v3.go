//go:build amd64 && !amd64.v3

package main

// This file is selected only for amd64 builds below GOAMD64=v3. The missing
// import intentionally turns unsupported v1/v2 builds into a build-time error.
import _ "intuition_engine_requires_GOAMD64_v3_or_newer"
