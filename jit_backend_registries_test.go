// jit_backend_registries_test.go - registry-coverage gates for the
// per-backend scaffold maps populated across Phases 3-7 of the six-CPU
// JIT unification plan.
//
// Each map registers one entry per backend tag in the canonical
// five-backend set ("ie64", "x86", "m68k", "z80", "6502"). A renamed or
// dropped backend silently dropping out of a registry would make the
// scaffold-to-real wiring miss a backend at integration time. These
// tests fail compile if a registry's value type is renamed and fail
// the run if a backend tag is missing.

//go:build amd64 && (linux || windows || darwin)

package main

import "testing"

func backendTags() []string {
	return []string{"ie64", "x86", "m68k", "z80", "6502"}
}

func TestBackendTierAllocators_HasAllBackends(t *testing.T) {
	for _, tag := range backendTags() {
		if _, ok := BackendTierAllocators[tag]; !ok {
			t.Errorf("BackendTierAllocators missing backend %q", tag)
		}
	}
}

func TestBackendRegionScanners_HasAllBackends(t *testing.T) {
	for _, tag := range backendTags() {
		if _, ok := BackendRegionScanners[tag]; !ok {
			t.Errorf("BackendRegionScanners missing backend %q", tag)
		}
	}
}

func TestBackendFastPathKinds_HasAllBackends(t *testing.T) {
	for _, tag := range backendTags() {
		kinds, ok := BackendFastPathKinds[tag]
		if !ok {
			t.Errorf("BackendFastPathKinds missing backend %q", tag)
			continue
		}
		if len(kinds) == 0 {
			t.Errorf("BackendFastPathKinds[%q] is empty — every backend "+
				"declares ≥1 fast-path bitmap kind", tag)
		}
	}
}

func TestBackendPollPatterns_HasAllBackends(t *testing.T) {
	for _, tag := range backendTags() {
		p, ok := BackendPollPatterns[tag]
		if !ok {
			t.Errorf("BackendPollPatterns missing backend %q", tag)
			continue
		}
		if p == nil {
			t.Errorf("BackendPollPatterns[%q] is nil", tag)
			continue
		}
		if p.IterationCap == 0 {
			t.Errorf("BackendPollPatterns[%q].IterationCap == 0 — must use "+
				"DefaultPollIterationCap or higher", tag)
		}
	}
}

func TestBackendCanonicalABI_HasAllBackends(t *testing.T) {
	for _, tag := range backendTags() {
		abi, ok := BackendCanonicalABI[tag]
		if !ok {
			t.Errorf("BackendCanonicalABI missing backend %q", tag)
			continue
		}
		if len(abi) == 0 {
			t.Errorf("BackendCanonicalABI[%q] has zero slot bindings", tag)
		}
	}
}
