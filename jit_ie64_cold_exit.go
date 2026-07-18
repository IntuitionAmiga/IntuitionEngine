// jit_ie64_cold_exit.go - Shared support for outlining the cold exit of a
// native observed conditional region (amd64 and ARM64).
//
// This file is part of the Intuition Engine project.
// Copyright (c) 2024 - 2026 Zayn Otley
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as
// published by the Free Software Foundation, either version 3 of the
// License, or (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package main

import "sync/atomic"

// ie64ColdExitOutlines counts observed regions compiled with an outlined
// cold exit. Structural tests read it to prove the layout was applied.
var ie64ColdExitOutlines atomic.Uint64

// ie64ColdExitOutlineDisabled turns the outlined layout off. Benchmark-only
// toggle so both layouts run under identical conditions in one binary.
var ie64ColdExitOutlineDisabled bool

// ie64ColdExitOutlineEligible reports whether observed conditionals may use
// the outlined cold-exit layout. Every adjacent forward conditional in a
// region qualifies; the caller must still require the conditional's hot
// successor to be the next emitted block (an adjacent forward edge), so
// backward hot edges retain the current layout. Each qualifying site gets
// its own stub with independently captured state.
func ie64ColdExitOutlineEligible(observed []ie64ObservedBlock) bool {
	return !ie64ColdExitOutlineDisabled
}
