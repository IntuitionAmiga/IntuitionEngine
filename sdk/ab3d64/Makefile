# sdk/ab3d64/Makefile — IE64 source-port of AB3D2 Redux High Overdrive.
#
# Drives the m68kto64 + ie64asm pipeline against the snapshot under src/.
# See README.md (operator) and ../docs/AB3D64.md (engineer) for context.

REPO_ROOT := $(abspath $(CURDIR)/../..)
M68KTO64_KMAKE := $(REPO_ROOT)/sdk/scripts/ab3d2/kmake.sh

# Include search paths for both transpile (m68kto64 -I) and assemble
# (ie64asm -I). Order mirrors upstream build.mk closely enough that
# `include "ie/foo.i"` and `include "foo.i"` resolve identically.
INCLUDES := \
    -I src/ie \
    -I src \
    -I src/modules \
    -I src/data \
    -I src/bss \
    -I src/media \
    -I src/media/includes \
    -I build/redux-high \
    -I _build/ie_unpacked/media \
    -I _build/ie_unpacked/media/includes \
    -I _build/ie_media/redux-high \
    -I _build/ie_media/redux-high/includes \
    -I .

# All toggles are IFD/IFND upstream — defined-ness, not value.
# "On" = -D NAME=1 (=1 signals intent; value is irrelevant to IFD).
# IE_ENABLE_SID_MUSIC intentionally OMITTED → IFD false → SID off.
#   mnu_nocode (IFND): intentionally omitted → include the interactive menu path.
#   USE_16X16_TEXEL_MULS (IFD): defined → use 16x16 muls path
#   IE_OVERDRIVE (IFD): defined → overdrive renderer (the only variant)
DEFINES := -D USE_16X16_TEXEL_MULS=1 -D IE_OVERDRIVE=1 -D IE_FIRST_FRAME_PRESENT_EARLY=1 -D IE_DISABLE_BRIGHTANIM=1 -D IE_DISABLE_BACKSFX=1 -D IS_IE=1

# -strip-cond drops if/else/endif wrappers so dead branches don't leak
# unsupported-mnemonic noise into ie64asm. Match Phase-2 contract.
# -flag-liveness elides shadow-CCR emission when N/Z/C/V/X are dead
# (no downstream Bcc/DBcc/Scc/addx/etc. before the next producer).
# Substantial throughput win for arith-heavy code without semantic change.
export M68KTO64_EXTRA_FLAGS := -strip-cond -flag-liveness -mmio-range 0xF0000-0xF2FFF

SRCS := src/ie/ie_hires_platform.s src/ie/hires.s src/ie/ie_fake_vectors.s
SRC_DEPS := $(shell find src -type f)

TARGET := bin/ab3d2_ie64_redux_high_overdrive.ie64

.PHONY: all ie64-overdrive clean drift

all: ie64-overdrive

ie64-overdrive: $(TARGET)

$(TARGET): $(SRCS) $(SRC_DEPS) tools/run_media_prep.sh Makefile
	@mkdir -p bin build/redux-high tools/generated tools/shots tools/states tools/memdumps
	tools/run_media_prep.sh redux-high build/redux-high
	IE64_SOURCE_OUT=build/ab3d64.ie64.s \
	IE64_LISTING_OUT=build/ab3d64.listing.txt \
		$(M68KTO64_KMAKE) $(INCLUDES) $(DEFINES) -o $@ $(SRCS)
	python3 tools/extract_ie64_symbols.py \
		build/ab3d64.listing.txt tools/generated/diag_symbols_ab3d64.lua

clean:
	rm -rf bin build

# Informational diff against a fresh upstream rsync+strip. No exit-code
# gate; runs out-of-band when a re-sync is contemplated.
drift:
	tools/check_upstream_drift.sh
