# Makefile for Intuition Engine Virtual Machine and IE32Asm assembler
#
# This Makefile handles the build process for:
# - IntuitionEngine: The main virtual machine executable
# - ie32asm: The IE32 assembly language assembler
#
# Features:
# - Parallel compilation using available CPU cores
# - Debug symbol stripping for smaller binaries
# - UPX compression for reduced executable size
# - Automatic dependency management
# - Build artifact organization
# Directory structure
BIN_DIR := ./bin
SDK_BIN_DIR := ./sdk/bin
IEXEC_DIR := ./sdk/intuitionos/iexec
IEXEC_SRC := $(IEXEC_DIR)/iexec.s
IEXEC_RUNTIME_SRC := $(IEXEC_DIR)/runtime_builder.s
IEXEC_IMG := $(IEXEC_DIR)/iexec.ie64
IEXEC_LST := $(IEXEC_DIR)/iexec.lst
IEXEC_RUNTIME_IMG := $(IEXEC_DIR)/runtime_builder.ie64
IEXEC_RUNTIME_LST := $(IEXEC_DIR)/runtime_builder.lst
IEXEC_ELF_REBUILDER := ./tools/rebuild_boot_dos_elf
IEXEC_SYSTEM_EXPORTER := ./tools/export_intuitionos_system
IEXEC_SYSTEM_DIR := ./sdk/intuitionos/system/SYS/IOSSYS
IEXEC_RUNTIME_ELF_TARGETS := \
	boot_dos_library.elf:prog_doslib \
	boot_console_handler.elf:prog_console \
	boot_shell.elf:prog_shell \
	boot_hardware_resource.elf:prog_hwres \
	boot_input_device.elf:prog_input_device \
	boot_graphics_library.elf:prog_graphics_library \
	boot_intuition_library.elf:prog_intuition_library \
	cmd_version.elf:prog_version \
	cmd_avail.elf:prog_avail \
	cmd_dir.elf:prog_dir \
	cmd_type.elf:prog_type \
	cmd_echo.elf:prog_echo_cmd \
	cmd_assign.elf:prog_assign_cmd \
	cmd_list.elf:prog_list_cmd \
	cmd_which.elf:prog_which_cmd \
	cmd_help.elf:prog_help_app \
	cmd_gfxdemo.elf:prog_gfxdemo \
	cmd_about.elf:prog_about \
	elfseg_fixture.elf:prog_elfseg
EMUTOS_SRC_DIR ?= ../EmuTOS
EMUTOS_ROM ?= ./sdk/examples/prebuilt/etos256us.img
EMUTOS_GIT_URL ?= https://github.com/IntuitionAmiga/EmuTOS.git
EMUTOS_GIT_REF ?= master
EMUTOS_BUILD_TARGET ?= 256
EMUTOS_CPUFLAGS ?= -m68020
EMUTOS_EXTRA_BIOS_SRC ?=
EMUTOS_MACHINE_DEF ?= -DMACHINE_IE -DCONF_ATARI_HARDWARE=0 -DCONF_STRAM_SIZE=4*1024*1024 -DCONF_WITH_TTRAM=0 -DCONF_WITH_ALT_RAM=0 -DCONF_VRAM_ADDRESS=0x00100000
EMUTOS_LINUX_GCC ?= m68k-linux-gnu-gcc-13
EMUTOS_LINUX_OPTFLAGS ?= -Os -fno-ivopts -fno-tree-slsr
EMUTOS_LINUX_WARNFLAGS ?= -Wall
# Override when your EmuTOS tree needs custom build args.
# Default "auto" chooses:
# - m68k-atari-mint-gcc => make -C <src> <target>
# - m68k-elf-gcc        => make -C <src> ELF=1 <target>
EMUTOS_BUILD_CMD ?= auto

# AROS build configuration
AROS_SRC_DIR ?= ../AROS
AROS_BUILD_DIR ?= $(AROS_SRC_DIR)/bin/ie-m68k
AROS_ROM ?= ./sdk/examples/prebuilt/aros-ie.rom
AROS_GIT_URL ?= https://github.com/IntuitionAmiga/AROS.git
AROS_GIT_REF ?= master
AROS_GCC_VER ?= 15.2.0

# Detect number of CPU cores for parallel compilation
NCORES := $(shell nproc)

# Detect host architecture
ARCH := $(shell uname -m)

# Map host architecture to Go architecture name
ifeq ($(ARCH),x86_64)
    NATIVE_GOARCH := amd64
else ifeq ($(ARCH),aarch64)
    NATIVE_GOARCH := arm64
else
    NATIVE_GOARCH := $(ARCH)
endif

# Cross-compiler detection for dual-architecture Linux release builds.
# aarch64 host → cross-compile for amd64; x86_64 host → cross-compile for arm64.
CROSS_SYSROOT := ./sysroot
HAS_SYSROOT := $(wildcard $(CROSS_SYSROOT)/usr)

ifeq ($(ARCH),aarch64)
    CROSS_CC := x86_64-linux-gnu-gcc
    CROSS_CXX := x86_64-linux-gnu-g++
    CROSS_GOARCH := amd64
    CROSS_PKG_CONFIG_LIBDIR := /usr/lib/x86_64-linux-gnu/pkgconfig:/usr/share/pkgconfig
    CROSS_CGO_CFLAGS :=
    CROSS_CGO_CXXFLAGS :=
    CROSS_CGO_LDFLAGS :=
else ifeq ($(ARCH),x86_64)
    CROSS_GOARCH := arm64
    # Auto-detect aarch64 cross-compiler (Debian/Ubuntu vs Tumbleweed naming)
    ifneq ($(shell command -v aarch64-linux-gnu-gcc 2>/dev/null),)
        CROSS_CC := aarch64-linux-gnu-gcc
        CROSS_CXX := aarch64-linux-gnu-g++
    else ifneq ($(shell command -v aarch64-suse-linux-gcc 2>/dev/null),)
        CROSS_CC := aarch64-suse-linux-gcc
        CROSS_CXX := aarch64-suse-linux-g++
    else
        CROSS_CC := aarch64-linux-gnu-gcc
        CROSS_CXX := aarch64-linux-gnu-g++
    endif
    # Sysroot-based cross-compilation (Tumbleweed or manually populated sysroot)
    ifneq ($(HAS_SYSROOT),)
        CROSS_CGO_CFLAGS := --sysroot=$(CROSS_SYSROOT)
        CROSS_CGO_CXXFLAGS := --sysroot=$(CROSS_SYSROOT)
        CROSS_CGO_LDFLAGS := --sysroot=$(CROSS_SYSROOT)
        CROSS_PKG_CONFIG_LIBDIR := $(CROSS_SYSROOT)/usr/lib64/pkgconfig:$(CROSS_SYSROOT)/usr/share/pkgconfig:$(CROSS_SYSROOT)/usr/lib/aarch64-linux-gnu/pkgconfig
        CROSS_PKG_CONFIG_SYSROOT_DIR := $(CROSS_SYSROOT)
    else
        CROSS_CGO_CFLAGS :=
        CROSS_CGO_CXXFLAGS :=
        CROSS_CGO_LDFLAGS :=
        CROSS_PKG_CONFIG_LIBDIR := /usr/lib/aarch64-linux-gnu/pkgconfig:/usr/share/pkgconfig
    endif
endif

# Version metadata
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
BUILD_DATE := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

# Go build flags with version injection
GO_FLAGS := -ldflags "-s -w -X main.Version=$(APP_VERSION) -X main.Commit=$(COMMIT) -X main.BuildDate=$(BUILD_DATE)"

# Commands and tools
GO := go
SSTRIP := true
UPX := upx
MKDIR := mkdir
NICE := nice
INSTALL := install
NICE_LEVEL := 19

# Installation paths
PREFIX := /usr/local
INSTALL_BIN_DIR := $(PREFIX)/bin

# Application metadata
APP_NAME := IntuitionEngine
APP_VERSION := 1.0.0

SHOWREEL_SCRIPT := ./sdk/scripts/ie_product_demo.ies
SHOWREEL_PREBUILT_DIR := ./sdk/examples/prebuilt
SHOWREEL_MUSIC_DIR := ./sdk/examples/assets/music
SHOWREEL_BASIC_SOURCE := ./sdk/examples/basic/rotozoomer_basic.bas
ROTOZOOM_VARIANT_TEXTURES := \
	./sdk/examples/assets/rotozoomtexture_ehbasic.raw \
	./sdk/examples/assets/rotozoomtexture_ie32.raw \
	./sdk/examples/assets/rotozoomtexture_ie64.raw \
	./sdk/examples/assets/rotozoomtexture_m68k.raw \
	./sdk/examples/assets/rotozoomtexture_6502.raw \
	./sdk/examples/assets/rotozoomtexture_z80.raw \
	./sdk/examples/assets/rotozoomtexture_x86.raw
SHOWREEL_ROBOCOP_PNG := ./sdk/examples/assets/robocop.png
SHOWREEL_FONT_RGBA := ./sdk/examples/assets/font_rgba.bin
SHOWREEL_BOING_TEXTURE := ./sdk/examples/assets/boing_checker_64.bin
SHOWREEL_FONT_SOURCES := ./tools/font2rgba/main.go ./tools/font2rgba/font.go
SHOWREEL_BOING_SOURCES := ./tools/gen_boing_checker/main.go
SHOWREEL_STATIC_ASSETS := $(SHOWREEL_ROBOCOP_PNG)
SHOWREEL_MUSIC_FILES := \
	$(SHOWREEL_MUSIC_DIR)/demo_sid.sid \
	$(SHOWREEL_MUSIC_DIR)/demo_pokey.sap \
	$(SHOWREEL_MUSIC_DIR)/demo_ted.ted \
	$(SHOWREEL_MUSIC_DIR)/demo_sndh.sndh \
	$(SHOWREEL_MUSIC_DIR)/demo_sn76489.vgm \
	$(SHOWREEL_MUSIC_DIR)/demo_sn76489.vgz \
	$(SHOWREEL_MUSIC_DIR)/demo_ay_spectrum.ay \
	$(SHOWREEL_MUSIC_DIR)/demo_ay_cpc.ay \
	$(SHOWREEL_MUSIC_DIR)/demo_ym.ym \
	$(SHOWREEL_MUSIC_DIR)/demo_vtx.vtx \
	$(SHOWREEL_MUSIC_DIR)/demo_pt3.pt3 \
	$(SHOWREEL_MUSIC_DIR)/demo_pt2.pt2 \
	$(SHOWREEL_MUSIC_DIR)/demo_pt1.pt1 \
	$(SHOWREEL_MUSIC_DIR)/demo_stc.stc \
	$(SHOWREEL_MUSIC_DIR)/demo_sqt.sqt \
	$(SHOWREEL_MUSIC_DIR)/demo_asc.asc \
	$(SHOWREEL_MUSIC_DIR)/demo_ftc.ftc \
	$(SHOWREEL_MUSIC_DIR)/demo_ahx.ahx \
	$(SHOWREEL_MUSIC_DIR)/demo_mod.mod \
	$(SHOWREEL_MUSIC_DIR)/demo_wav.wav
SHOWREEL_RUNTIME_INPUTS := $(SHOWREEL_BASIC_SOURCE) $(SHOWREEL_STATIC_ASSETS) $(SHOWREEL_MUSIC_FILES)
SHOWREEL_CORE_ARTIFACTS := \
	$(BIN_DIR)/IntuitionEngine \
	$(SHOWREEL_PREBUILT_DIR)/ehbasic_ie64.ie64 \
	$(EMUTOS_ROM)
SHOWREEL_IE32_ARTIFACTS := \
	$(SHOWREEL_PREBUILT_DIR)/rotozoomer.iex \
	$(SHOWREEL_PREBUILT_DIR)/voodoo_mega_demo.iex \
	$(SHOWREEL_PREBUILT_DIR)/vga_mode13h_fire.iex \
	$(SHOWREEL_PREBUILT_DIR)/vga_modex_circles.iex \
	$(SHOWREEL_PREBUILT_DIR)/vga_mode12h_bars.iex \
	$(SHOWREEL_PREBUILT_DIR)/vga_text_hello.iex \
	$(SHOWREEL_PREBUILT_DIR)/robocop_intro.iex
SHOWREEL_IE64_ARTIFACTS := \
	$(SHOWREEL_PREBUILT_DIR)/rotozoomer_ie64.ie64 \
	./sdk/examples/asm/mandelbrot_ie64.ie64
SHOWREEL_M68K_ARTIFACTS := \
	$(SHOWREEL_PREBUILT_DIR)/rotozoomer_68k.ie68 \
	$(SHOWREEL_PREBUILT_DIR)/ted_121_colors_68k.ie68 \
	$(SHOWREEL_PREBUILT_DIR)/rotating_cube_copper_68k.ie68 \
	$(SHOWREEL_PREBUILT_DIR)/voodoo_cube_68k.ie68 \
	$(SHOWREEL_PREBUILT_DIR)/voodoo_3dfx_logo_68k.ie68 \
	$(SHOWREEL_PREBUILT_DIR)/voodoo_triangle_68k.ie68 \
	$(SHOWREEL_PREBUILT_DIR)/robocop_intro_68k.ie68 \
	$(SHOWREEL_PREBUILT_DIR)/rotozoomer_gem.prg
SHOWREEL_Z80_ARTIFACTS := \
	$(SHOWREEL_PREBUILT_DIR)/rotozoomer_z80.ie80 \
	$(SHOWREEL_PREBUILT_DIR)/voodoo_tunnel_z80.ie80 \
	$(SHOWREEL_PREBUILT_DIR)/vga_text_sap_demo.ie80 \
	$(SHOWREEL_PREBUILT_DIR)/robocop_intro_z80.ie80
SHOWREEL_6502_ARTIFACTS := \
	$(SHOWREEL_PREBUILT_DIR)/rotozoomer_65.ie65 \
	$(SHOWREEL_PREBUILT_DIR)/ula_rotating_cube_65.ie65 \
	$(SHOWREEL_PREBUILT_DIR)/robocop_intro_65.ie65
SHOWREEL_X86_ARTIFACTS := \
	$(SHOWREEL_PREBUILT_DIR)/rotozoomer_x86.ie86 \
	$(SHOWREEL_PREBUILT_DIR)/antic_plasma_x86.ie86
SHOWREEL_ALL_ARTIFACTS := \
	$(SHOWREEL_CORE_ARTIFACTS) \
	$(SHOWREEL_IE32_ARTIFACTS) \
	$(SHOWREEL_IE64_ARTIFACTS) \
	$(SHOWREEL_M68K_ARTIFACTS) \
	$(SHOWREEL_Z80_ARTIFACTS) \
	$(SHOWREEL_6502_ARTIFACTS) \
	$(SHOWREEL_X86_ARTIFACTS)

# Build profiles:
#   make                      Full build (Vulkan + Ebiten + OTO)
#   make novulkan             Software Voodoo only (no Vulkan SDK needed)
#   make headless             No display, no audio, no Vulkan (CI/testing)
#   make headless-novulkan    CGO_ENABLED=0 portable build (cross-compile safe)

# Release directories
RELEASE_DIR := ./release

# Main targets
.PHONY: all clean list install uninstall novulkan headless headless-novulkan
.PHONY: sdk clean-sdk release-src release-sdk release-linux release-linux-amd64 release-linux-arm64 release-windows release-all players
.PHONY: build-showreel-deps run-showreel check-showreel-prereqs showreel-emutos showreel-ie32 showreel-ie64 showreel-m68k showreel-z80 showreel-6502 showreel-x86 font-rgba boing-checker
.PHONY: testdata-opl

# Default target builds everything
all: setup intuition-engine ie32asm ie64asm ie32to64 ie64dis
	@echo "Build complete! VM in $(BIN_DIR)/, tools in $(SDK_BIN_DIR)/"
	@$(MAKE) list

# Create necessary directories
setup:
	@echo "Creating build directories..."
	@$(MKDIR) -p $(BIN_DIR)
	@$(GO) mod tidy -v

# Build the Intuition Engine VM
intuition-engine: setup
	@echo "Building Intuition Engine VM..."
	@CGO_JOBS=$(NCORES) $(NICE) -$(NICE_LEVEL) $(GO) build $(GO_FLAGS) .
	@echo "Stripping debug symbols..."
	@$(NICE) -$(NICE_LEVEL) $(SSTRIP) -z IntuitionEngine
	@echo "Applying UPX compression..."
	@$(NICE) -$(NICE_LEVEL) $(UPX) --lzma IntuitionEngine
	@mv IntuitionEngine $(BIN_DIR)/
	@echo "Intuition Engine VM build complete"

# Build without Vulkan (software Voodoo rasterizer only)
novulkan: setup
	@echo "Building Intuition Engine VM (novulkan)..."
	@CGO_JOBS=$(NCORES) $(NICE) -$(NICE_LEVEL) $(GO) build $(GO_FLAGS) -tags novulkan .
	@echo "Stripping debug symbols..."
	@$(NICE) -$(NICE_LEVEL) $(SSTRIP) -z IntuitionEngine
	@echo "Applying UPX compression..."
	@$(NICE) -$(NICE_LEVEL) $(UPX) --lzma IntuitionEngine
	@mv IntuitionEngine $(BIN_DIR)/
	@echo "Intuition Engine VM (novulkan) build complete"

# Build headless (no display, no audio, no Vulkan - for CI/testing)
headless: setup
	@echo "Building Intuition Engine VM (headless)..."
	@CGO_JOBS=$(NCORES) $(NICE) -$(NICE_LEVEL) $(GO) build $(GO_FLAGS) -tags headless .
	@echo "Stripping debug symbols..."
	@$(NICE) -$(NICE_LEVEL) $(SSTRIP) -z IntuitionEngine
	@echo "Applying UPX compression..."
	@$(NICE) -$(NICE_LEVEL) $(UPX) --lzma IntuitionEngine
	@mv IntuitionEngine $(BIN_DIR)/
	@echo "Intuition Engine VM (headless) build complete"

# Build headless+novulkan with CGO disabled (fully portable, cross-compile safe)
headless-novulkan: setup
	@echo "Building Intuition Engine VM (headless-novulkan, CGO_ENABLED=0)..."
	@CGO_ENABLED=0 $(NICE) -$(NICE_LEVEL) $(GO) build $(GO_FLAGS) -tags "novulkan headless" .
	@mv IntuitionEngine $(BIN_DIR)/
	@echo "Intuition Engine VM (headless-novulkan) build complete"

# Build the IE32 assembler
ie32asm: setup
	@echo "Building IE32 assembler..."
	@$(GO) build $(GO_FLAGS) assembler/ie32asm.go
	@echo "Stripping debug symbols..."
	@$(SSTRIP) -z ie32asm
	@echo "Applying UPX compression..."
	@$(UPX) --lzma ie32asm
	@$(MKDIR) -p $(SDK_BIN_DIR)
	@mv ie32asm $(SDK_BIN_DIR)/
	@echo "IE32 assembler build complete"

# Build the IE64 assembler
ie64asm: setup
	@echo "Building IE64 assembler..."
	@$(GO) build $(GO_FLAGS) -tags ie64 -o ie64asm assembler/ie64asm.go
	@echo "Stripping debug symbols..."
	@$(SSTRIP) -z ie64asm
	@echo "Applying UPX compression..."
	@$(UPX) --lzma ie64asm
	@$(MKDIR) -p $(SDK_BIN_DIR)
	@mv ie64asm $(SDK_BIN_DIR)/
	@echo "IE64 assembler build complete"

# Build the IE32-to-IE64 converter
ie32to64: setup
	@echo "Building IE32-to-IE64 converter..."
	@$(GO) build $(GO_FLAGS) -o ie32to64 ./cmd/ie32to64/
	@$(MKDIR) -p $(SDK_BIN_DIR)
	@mv ie32to64 $(SDK_BIN_DIR)/
	@echo "IE32-to-IE64 converter build complete"

# Assemble the IExec microkernel
.PHONY: intuitionos intuitionos-clean
intuitionos: ie64asm
	@echo "Assembling IExec kernel and runtime images..."
	@rm -f $(IEXEC_DIR)/*.elf $(IEXEC_IMG) $(IEXEC_LST) $(IEXEC_RUNTIME_IMG) $(IEXEC_RUNTIME_LST)
	@$(SDK_BIN_DIR)/ie64asm -list -I sdk/include -I $(IEXEC_DIR) $(IEXEC_RUNTIME_SRC) > $(IEXEC_RUNTIME_LST)
	@$(MKDIR) -p $(SDK_BIN_DIR)
	@$(GO) build $(GO_FLAGS) -o $(SDK_BIN_DIR)/iexec-elf-rebuilder $(IEXEC_ELF_REBUILDER)
	@$(GO) build $(GO_FLAGS) -o $(SDK_BIN_DIR)/iexec-system-exporter $(IEXEC_SYSTEM_EXPORTER)
	@for spec in $(IEXEC_RUNTIME_ELF_TARGETS); do \
		out=$${spec%%:*}; \
		label=$${spec##*:}; \
		$(SDK_BIN_DIR)/iexec-elf-rebuilder -listing $(IEXEC_RUNTIME_LST) -image $(IEXEC_RUNTIME_IMG) -out $(IEXEC_DIR)/$$out -label $$label; \
	done
	@$(SDK_BIN_DIR)/ie64asm -list -I sdk/include -I $(IEXEC_DIR) $(IEXEC_SRC) > $(IEXEC_LST)
	@$(SDK_BIN_DIR)/iexec-system-exporter -repo-root . -iexec-dir $(IEXEC_DIR) -out-root $(IEXEC_SYSTEM_DIR)
	@echo "IExec kernel and runtime images assembled under $(IEXEC_DIR)"

intuitionos-clean:
	@echo "Cleaning IExec kernel and runtime images..."
	@rm -f $(IEXEC_DIR)/*.elf $(IEXEC_IMG) $(IEXEC_LST) $(IEXEC_RUNTIME_IMG) $(IEXEC_RUNTIME_LST)

# Build with embedded EhBASIC BASIC interpreter
.PHONY: basic
basic: ie64asm
	@echo "Assembling EhBASIC IE64 interpreter..."
	@$(SDK_BIN_DIR)/ie64asm -I sdk/include sdk/examples/asm/ehbasic_ie64.asm
	@$(MKDIR) -p sdk/examples/prebuilt
	@mv sdk/examples/asm/ehbasic_ie64.ie64 sdk/examples/prebuilt/
	@echo "Building Intuition Engine with embedded BASIC..."
	@CGO_JOBS=$(NCORES) $(NICE) -$(NICE_LEVEL) $(GO) build $(GO_FLAGS) -tags embed_basic .
	@echo "Stripping debug symbols..."
	@$(NICE) -$(NICE_LEVEL) $(SSTRIP) -z IntuitionEngine
	@echo "Applying UPX compression..."
	@$(NICE) -$(NICE_LEVEL) $(UPX) --lzma IntuitionEngine
	@mv IntuitionEngine $(BIN_DIR)/
	@echo "EhBASIC build complete - run with: $(BIN_DIR)/IntuitionEngine -basic"

# Build with embedded BASIC + EmuTOS ROM (type EMUTOS at the BASIC prompt).
.PHONY: basic-emutos
basic-emutos: ie64asm emutos-rom
	@echo "Assembling EhBASIC IE64 interpreter..."
	@$(SDK_BIN_DIR)/ie64asm -I sdk/include sdk/examples/asm/ehbasic_ie64.asm
	@$(MKDIR) -p sdk/examples/prebuilt
	@mv sdk/examples/asm/ehbasic_ie64.ie64 sdk/examples/prebuilt/
	@echo "Building Intuition Engine with embedded BASIC + EmuTOS..."
	@CGO_JOBS=$(NCORES) $(NICE) -$(NICE_LEVEL) $(GO) build $(GO_FLAGS) -tags "embed_basic embed_emutos" .
	@echo "Stripping debug symbols..."
	@$(NICE) -$(NICE_LEVEL) $(SSTRIP) -z IntuitionEngine
	@echo "Applying UPX compression..."
	@$(NICE) -$(NICE_LEVEL) $(UPX) --lzma IntuitionEngine
	@mv IntuitionEngine $(BIN_DIR)/
	@echo "BASIC+EmuTOS build complete - run with: $(BIN_DIR)/IntuitionEngine -basic"

# Build with embedded EmuTOS ROM image.
.PHONY: emutos
emutos: setup emutos-rom
	@echo "Building Intuition Engine with embedded EmuTOS ROM..."
	@CGO_JOBS=$(NCORES) $(NICE) -$(NICE_LEVEL) $(GO) build $(GO_FLAGS) -tags embed_emutos .
	@echo "Stripping debug symbols..."
	@$(NICE) -$(NICE_LEVEL) $(SSTRIP) -z IntuitionEngine
	@echo "Applying UPX compression..."
	@$(NICE) -$(NICE_LEVEL) $(UPX) --lzma IntuitionEngine
	@mv IntuitionEngine $(BIN_DIR)/
	@echo "EmuTOS build complete - run with: $(BIN_DIR)/IntuitionEngine -emutos"

# Build with embedded AROS ROM image.
.PHONY: aros
aros: setup aros-rom
	@echo "Building Intuition Engine with embedded AROS ROM..."
	@CGO_JOBS=$(NCORES) $(NICE) -$(NICE_LEVEL) $(GO) build $(GO_FLAGS) -tags embed_aros .
	@echo "Stripping debug symbols..."
	@$(NICE) -$(NICE_LEVEL) $(SSTRIP) -z IntuitionEngine
	@echo "Applying UPX compression..."
	@$(NICE) -$(NICE_LEVEL) $(UPX) --lzma IntuitionEngine
	@mv IntuitionEngine $(BIN_DIR)/
	@echo "AROS build complete - run with: $(BIN_DIR)/IntuitionEngine -aros"

.PHONY: aros-rom
aros-rom:
	@if [ ! -d "$(AROS_SRC_DIR)" ]; then \
		if ! command -v git >/dev/null 2>&1; then \
			echo "Error: git is required to clone AROS source."; \
			exit 1; \
		fi; \
		echo "Cloning AROS source..."; \
		git clone --depth 1 --branch "$(AROS_GIT_REF)" "$(AROS_GIT_URL)" "$(AROS_SRC_DIR)"; \
	fi
	@echo "Initialising AROS submodules (catalogs, tests, etc.)..."
	@git -C "$(AROS_SRC_DIR)" submodule update --init --recursive
	@if [ ! -f "$(AROS_BUILD_DIR)/config/make.cfg" ]; then \
		echo "Configuring AROS for ie-m68k..."; \
		$(MKDIR) -p "$(AROS_BUILD_DIR)"; \
		AROS_CONFIGURE="$$(cd "$(AROS_SRC_DIR)" && pwd)/configure"; \
		cd "$(AROS_BUILD_DIR)" && \
		"$$AROS_CONFIGURE" --target=ie-m68k \
			--with-cpu=68020 \
			--with-gcc-version=$(AROS_GCC_VER) \
			--enable-build-type=personal; \
	fi
	@echo "Building AROS ROM..."
	@# Regenerate linker include files to pick up mmakefile.src changes
	@rm -f "$(AROS_BUILD_DIR)/bin/ie-m68k/gen/rom_objs.ld" \
		"$(AROS_BUILD_DIR)/bin/ie-m68k/gen/ext_objs.ld" 2>/dev/null || true
	@$(MAKE) -C "$(AROS_BUILD_DIR)" -j$(NCORES) kernel-ie-m68k-rom
	@echo "Building AROS workbench components (S/, C/, Fonts/)..."
	@# Fix m68k-amiga workbench-c targets: scope them to amiga arch only.
	@# Without this, workbench-c-m68k pulls in amiga-specific gdbstub/SetPatch for ALL
	@# m68k targets (including IE). The fix: rename workbench-c-m68k → workbench-c-amiga-m68k
	@# and wire via workbench-c-amiga, then drop the bare CPU-level dep from workbench/c/.
	@sed -i 's/workbench-c-m68k/workbench-c-amiga-m68k/g' \
		"$(AROS_SRC_DIR)/arch/m68k-amiga/c/mmakefile.src" 2>/dev/null || true
	@if ! grep -q 'workbench-c-amiga :' "$(AROS_SRC_DIR)/arch/m68k-amiga/c/mmakefile.src" 2>/dev/null; then \
		sed -i '1a #MM- workbench-c-amiga : workbench-c-amiga-m68k' \
			"$(AROS_SRC_DIR)/arch/m68k-amiga/c/mmakefile.src" 2>/dev/null || true; \
	fi
	@# Remove bare workbench-c-$(AROS_TARGET_CPU) dep so IE doesn't inherit amiga targets
	@sed -i '/#MM.*workbench-c-\$$(AROS_TARGET_CPU) /d' \
		"$(AROS_SRC_DIR)/workbench/c/mmakefile.src" 2>/dev/null || true
	@sed -i '/#MM.*workbench-c-\$$(AROS_TARGET_CPU)-quick/d' \
		"$(AROS_SRC_DIR)/workbench/c/mmakefile.src" 2>/dev/null || true
	@# Disable workbench-c-r and workbench-c-loadresource: broken catalog builds
	@sed -i 's/^#MM- workbench-c : workbench-c-r$$/#disabled: workbench-c-r (broken catalog build)/' \
		"$(AROS_SRC_DIR)/workbench/c/R/mmakefile.src" 2>/dev/null || true
	@sed -i 's/^#MM- workbench-c : workbench-c-loadresource$$/#disabled: workbench-c-loadresource (broken catalog build)/' \
		"$(AROS_SRC_DIR)/workbench/c/LoadResource/mmakefile.src" 2>/dev/null || true
	@# Regenerate mmakefiles after patching sources
	@rm -f "$(AROS_BUILD_DIR)/workbench/c/mmakefile" \
		"$(AROS_BUILD_DIR)/arch/m68k-amiga/c/mmakefile" \
		"$(AROS_BUILD_DIR)/workbench/c/R/mmakefile" \
		"$(AROS_BUILD_DIR)/workbench/c/LoadResource/mmakefile" 2>/dev/null || true
	@echo "Building complete AROS workbench (all libs, classes, tools, prefs, devices)..."
	@$(MAKE) -C "$(AROS_BUILD_DIR)" -j$(NCORES) workbench-complete 2>&1 || \
		echo "  Warning: workbench-complete had some failures (non-fatal)"
	@echo "Ensuring Zune classes are built (may have been skipped by Mesa failure)..."
	@$(MAKE) -C "$(AROS_BUILD_DIR)" -j$(NCORES) workbench-classes-zune 2>&1 || \
		echo "  Warning: workbench-classes-zune had some failures (non-fatal)"
	@echo "Building additional targets not in workbench-complete..."
	@$(MAKE) -C "$(AROS_BUILD_DIR)" -j$(NCORES) \
		compiler-stdcio \
		workbench-images-themes \
		kernel-ie-m68k-ahidrv 2>&1 || \
		echo "  Warning: additional targets had failures (non-fatal)"
	@echo "Installing stock AROS Startup-Sequence..."
	@mkdir -p "$(AROS_BUILD_DIR)/bin/ie-m68k/AROS/S"
	@cp -f "$(AROS_SRC_DIR)/workbench/s/Startup-Sequence" \
		"$(AROS_BUILD_DIR)/bin/ie-m68k/AROS/S/Startup-Sequence"
	@echo "Creating AROS directory structure..."
	@AROSDIR="$(AROS_BUILD_DIR)/bin/ie-m68k/AROS"; \
	for dir in WBStartup System System/Images Tools Tools/Commodities Utilities Demos Rexxc \
		Classes Classes/datatypes Classes/Gadgets Classes/Zune \
		Libs/Zune \
		Prefs Prefs/Presets/Themes Locale Locale/Languages Locale/Countries Locale/Flags \
		Storage Storage/DataTypes Storage/DOSDrivers Storage/Keymaps Storage/Monitors \
		Devs/Monitors Devs/AHI Devs/DataTypes Devs/Keymaps Devs/DOSDrivers Devs/Printers; do \
		$(MKDIR) -p "$$AROSDIR/$$dir"; \
	done
	@echo "Building AROS icons..."
	@ILBMTOICON="$(AROS_BUILD_DIR)/bin/$$(uname -s | tr A-Z a-z)-$$(uname -m)/tools/ilbmtoicon"; \
	AROSDIR="$(AROS_BUILD_DIR)/bin/ie-m68k/AROS"; \
	MASON_WB="$(AROS_SRC_DIR)/images/IconSets/Mason/workbench"; \
	MASON_EA="$(AROS_SRC_DIR)/images/IconSets/Mason/workbench/Prefs/Env-Archive"; \
	if [ -x "$$ILBMTOICON" ]; then \
		echo "  Building directory icons (Mason workbench set)..."; \
		for name in Demos Developer Devs Fonts Locale Prefs Storage System Tools Utilities; do \
			src="$$MASON_WB/$${name}.info.src"; \
			png="$$MASON_WB/$${name}.png"; \
			if [ -f "$$src" ] && [ -f "$$png" ]; then \
				$$ILBMTOICON --png "$$src" "$$png" "$$AROSDIR/$${name}.info"; \
			fi; \
		done; \
		echo "  Building fallback drawer icons (Mason def_Drawer)..."; \
		for name in WBStartup Classes; do \
			src="$$MASON_EA/def_Drawer.info.src"; \
			png="$$MASON_EA/def_Drawer.png"; \
			if [ -f "$$src" ] && [ -f "$$png" ]; then \
				$$ILBMTOICON --png "$$src" "$$png" "$$AROSDIR/$${name}.info"; \
			fi; \
		done; \
		echo "  Building Prefs program icons (Mason Prefs set)..."; \
		MASON_PREFS="$$MASON_WB/Prefs"; \
		for name in Font Input Locale Palette Pointer Printer ReqTools ScreenMode Serial Time Zune; do \
			src="$$MASON_PREFS/$${name}.info.src"; \
			png="$$MASON_PREFS/$${name}.png"; \
			if [ -f "$$src" ] && [ -f "$$png" ]; then \
				$$ILBMTOICON --png "$$src" "$$png" "$$AROSDIR/Prefs/$${name}.info"; \
			fi; \
		done; \
		for name in Appearance Asl BoingIconBar Editor IControl Network PSI Trackdisk; do \
			src="$$MASON_EA/def_Tool.info.src"; \
			png="$$MASON_EA/def_Tool.png"; \
			if [ -f "$$src" ] && [ -f "$$png" ] && [ -f "$$AROSDIR/Prefs/$${name}" ]; then \
				$$ILBMTOICON --png "$$src" "$$png" "$$AROSDIR/Prefs/$${name}.info"; \
			fi; \
		done; \
		echo "  Building Tools program icons (Mason Tools set)..."; \
		MASON_TOOLS="$$MASON_WB/Tools"; \
		for name in Calculator HDToolBox WiMP; do \
			src="$$MASON_TOOLS/$${name}.info.src"; \
			png="$$MASON_TOOLS/$${name}.png"; \
			if [ -f "$$src" ] && [ -f "$$png" ]; then \
				$$ILBMTOICON --png "$$src" "$$png" "$$AROSDIR/Tools/$${name}.info"; \
			fi; \
		done; \
		for name in Editor KeyShow ScreenGrabber ShowConfig SysExplorer BoingIconBar; do \
			src="$$MASON_EA/def_Tool.info.src"; \
			png="$$MASON_EA/def_Tool.png"; \
			if [ -f "$$src" ] && [ -f "$$png" ] && [ -f "$$AROSDIR/Tools/$${name}" ]; then \
				$$ILBMTOICON --png "$$src" "$$png" "$$AROSDIR/Tools/$${name}.info"; \
			fi; \
		done; \
		echo "  Building Commodities icons (Mason Commodities set)..."; \
		MASON_COMM="$$MASON_WB/Tools/Commodities"; \
		for name in AutoPoint Blanker ClickToFront Exchange Opaque; do \
			src="$$MASON_COMM/$${name}.info.src"; \
			png="$$MASON_COMM/$${name}.png"; \
			if [ -f "$$src" ] && [ -f "$$png" ]; then \
				$$ILBMTOICON --png "$$src" "$$png" "$$AROSDIR/Tools/Commodities/$${name}.info"; \
			fi; \
		done; \
		for name in ASCIITable AltKeyQ DepthMenu NoCapsLock; do \
			src="$$MASON_EA/def_Tool.info.src"; \
			png="$$MASON_EA/def_Tool.png"; \
			if [ -f "$$src" ] && [ -f "$$png" ] && [ -f "$$AROSDIR/Tools/Commodities/$${name}" ]; then \
				$$ILBMTOICON --png "$$src" "$$png" "$$AROSDIR/Tools/Commodities/$${name}.info"; \
			fi; \
		done; \
		echo "  Building Utilities icons (Mason Utilities set)..."; \
		MASON_UTIL="$$MASON_WB/Utilities"; \
		for name in Clock Installer More MultiView; do \
			src="$$MASON_UTIL/$${name}.info.src"; \
			png="$$MASON_UTIL/$${name}.png"; \
			if [ -f "$$src" ] && [ -f "$$png" ]; then \
				$$ILBMTOICON --png "$$src" "$$png" "$$AROSDIR/Utilities/$${name}.info"; \
			fi; \
		done; \
		for name in Help; do \
			src="$$MASON_EA/def_Tool.info.src"; \
			png="$$MASON_EA/def_Tool.png"; \
			if [ -f "$$src" ] && [ -f "$$png" ] && [ -f "$$AROSDIR/Utilities/$${name}" ]; then \
				$$ILBMTOICON --png "$$src" "$$png" "$$AROSDIR/Utilities/$${name}.info"; \
			fi; \
		done; \
		echo "  Building System icons (Mason System set)..."; \
		MASON_SYS="$$MASON_WB/system"; \
		for name in Format FixFonts; do \
			src="$$MASON_SYS/$${name}.info.src"; \
			png="$$MASON_SYS/$${name}.png"; \
			if [ -f "$$src" ] && [ -f "$$png" ]; then \
				$$ILBMTOICON --png "$$src" "$$png" "$$AROSDIR/System/$${name}.info"; \
			fi; \
		done; \
		for name in About CLI Find FTManager Snoopy SysMon VMM Workbook; do \
			src="$$MASON_EA/def_Tool.info.src"; \
			png="$$MASON_EA/def_Tool.png"; \
			if [ -f "$$src" ] && [ -f "$$png" ] && [ -f "$$AROSDIR/System/$${name}" ]; then \
				$$ILBMTOICON --png "$$src" "$$png" "$$AROSDIR/System/$${name}.info"; \
			fi; \
		done; \
		echo "  Building default type icons (Mason Env-Archive set)..."; \
		$(MKDIR) -p "$$AROSDIR/Prefs/Env-Archive/SYS"; \
		for src in "$$MASON_EA"/*.info.src; do \
			base=$$(basename "$$src" .info.src); \
			png="$$MASON_EA/$${base}.png"; \
			if [ -f "$$png" ]; then \
				$$ILBMTOICON --png "$$src" "$$png" "$$AROSDIR/Prefs/Env-Archive/SYS/$${base}.info"; \
			fi; \
		done; \
		echo "  Removing icons for non-user drawers..."; \
		for name in boot C Libs S T Rexxc; do \
			rm -f "$$AROSDIR/$${name}.info"; \
		done; \
		echo "  Built $$(ls "$$AROSDIR"/*.info 2>/dev/null | wc -l) directory icons, $$(ls "$$AROSDIR/Prefs/Env-Archive/SYS"/*.info 2>/dev/null | wc -l) default icons"; \
	else \
		echo "  Warning: ilbmtoicon not found, skipping icon build"; \
	fi
	@echo "Cleaning up Wanderer artifacts (using Workbook desktop)..."
	@AROSDIR="$(AROS_BUILD_DIR)/bin/ie-m68k/AROS"; \
	rm -rf "$$AROSDIR/Prefs/Env-Archive/SYS/Wanderer" "$$AROSDIR/System/Wanderer" "$$AROSDIR/System/Wanderer.info"
	@echo "Merging DataTypes into lowercase datatypes dir..."
	@AROSDIR="$(AROS_BUILD_DIR)/bin/ie-m68k/AROS"; \
	if [ -d "$$AROSDIR/Classes/DataTypes" ] && [ -d "$$AROSDIR/Classes/datatypes" ]; then \
		cp -f "$$AROSDIR/Classes/DataTypes/"* "$$AROSDIR/Classes/datatypes/" 2>/dev/null; \
		echo "  Merged $$(ls "$$AROSDIR/Classes/datatypes/" 2>/dev/null | wc -l) datatypes"; \
	fi
	@echo "Copying Zune classes to Libs/Zune/ for lddemon..."
	@AROSDIR="$(AROS_BUILD_DIR)/bin/ie-m68k/AROS"; \
	if [ -d "$$AROSDIR/Classes/Zune" ]; then \
		cp -f "$$AROSDIR/Classes/Zune/"*.mcc "$$AROSDIR/Libs/Zune/" 2>/dev/null; \
		cp -f "$$AROSDIR/Classes/Zune/"*.mcp "$$AROSDIR/Libs/Zune/" 2>/dev/null; \
		cp -f "$$AROSDIR/Classes/Zune/"*.mui "$$AROSDIR/Libs/Zune/" 2>/dev/null; \
		echo "  Copied $$(ls "$$AROSDIR/Libs/Zune/" 2>/dev/null | wc -l) Zune classes to Libs/Zune/"; \
	fi
	@echo "Extracting ROM binary..."
	@AROS_TARGETDIR="$(AROS_BUILD_DIR)/bin/ie-m68k"; \
	ROM_ELF="$$AROS_TARGETDIR/gen/boot/aros-ie-m68k-rom.elf"; \
	if [ ! -f "$$ROM_ELF" ]; then \
		echo "Error: ROM ELF not found at $$ROM_ELF"; \
		exit 1; \
	fi; \
	if command -v m68k-aros-objcopy >/dev/null 2>&1; then \
		AROS_OBJCOPY="m68k-aros-objcopy"; \
	elif command -v m68k-atari-mint-objcopy >/dev/null 2>&1; then \
		AROS_OBJCOPY="m68k-atari-mint-objcopy"; \
	elif command -v m68k-suse-linux-objcopy >/dev/null 2>&1; then \
		AROS_OBJCOPY="m68k-suse-linux-objcopy"; \
	else \
		AROS_OBJCOPY="m68k-linux-gnu-objcopy"; \
	fi; \
	$(MKDIR) -p "$$(dirname "$(AROS_ROM)")"; \
	$$AROS_OBJCOPY --output-target binary \
		--only-section=.rom --only-section=.ext --only-section=.ss \
		--gap-fill 0xff "$$ROM_ELF" "$(AROS_ROM)"; \
	echo "AROS ROM prepared: $(AROS_ROM) ($$(wc -c < "$(AROS_ROM)") bytes)"
	@echo "Installing AHI artifacts..."
	@AROSDIR="$(AROS_BUILD_DIR)/bin/ie-m68k/AROS"; \
	GENDIR="$(AROS_BUILD_DIR)/bin/ie-m68k/gen"; \
	$(MKDIR) -p "$$AROSDIR/Devs/AHI"; \
	if [ -f "$$GENDIR/workbench/devs/AHI/Device/ahi.device" ]; then \
		cp "$$GENDIR/workbench/devs/AHI/Device/ahi.device" "$$AROSDIR/Devs/ahi.device"; \
		echo "  Installed ahi.device"; \
	fi; \
	if [ -f "$$AROSDIR/Libs/ie-audio.library" ]; then \
		cp "$$AROSDIR/Libs/ie-audio.library" "$$AROSDIR/Devs/AHI/ie-audio.audio"; \
		echo "  Installed ie-audio.audio"; \
	fi
	@echo "Checking AHI artifacts..."
	@AROSDIR="$(AROS_BUILD_DIR)/bin/ie-m68k/AROS"; \
	for f in "$$AROSDIR/Devs/ahi.device" "$$AROSDIR/Devs/AHI/ie-audio.audio"; do \
		if [ ! -f "$$f" ]; then echo "Warning: AHI artifact missing: $$f (AHI may not be functional)"; fi; \
	done

.PHONY: clean-aros
clean-aros:
	@if [ ! -d "$(AROS_BUILD_DIR)" ]; then \
		echo "Nothing to clean — AROS build directory does not exist."; \
		exit 0; \
	fi
	@echo "Cleaning AROS ROM link + module objects..."
	@GENDIR="$(AROS_BUILD_DIR)/bin/ie-m68k/gen"; \
	rm -rf "$$GENDIR/boot" "$$GENDIR/rom"
	@rm -f "$(AROS_ROM)"

.PHONY: clean-aros-all
clean-aros-all:
	@RESOLVED=$$(cd "$(AROS_BUILD_DIR)" 2>/dev/null && pwd || echo ""); \
	SRCBASE=$$(cd "$(AROS_SRC_DIR)" 2>/dev/null && pwd || echo ""); \
	if [ -z "$$RESOLVED" ]; then \
		echo "Nothing to clean — AROS build directory does not exist."; \
		exit 0; \
	fi; \
	if [ -z "$$SRCBASE" ] || ! echo "$$RESOLVED" | grep -q "^$$SRCBASE/"; then \
		echo "Error: AROS_BUILD_DIR ($$RESOLVED) is not under AROS_SRC_DIR ($(AROS_SRC_DIR)) — refusing to delete."; \
		exit 1; \
	fi; \
	echo "Removing AROS build directory: $$RESOLVED"; \
	rm -rf "$$RESOLVED"

.PHONY: emutos-probe
emutos-probe: emutos
	@echo "Running EmuTOS boot probe script..."
	@$(BIN_DIR)/IntuitionEngine -emutos -script ./sdk/scripts/emutos_boot_probe.ies

$(SHOWREEL_FONT_RGBA): $(SHOWREEL_FONT_SOURCES)
	@echo "Generating Robocop font atlas..."
	@$(GO) run ./tools/font2rgba

font-rgba: $(SHOWREEL_FONT_RGBA)

$(SHOWREEL_BOING_TEXTURE): $(SHOWREEL_BOING_SOURCES)
	@echo "Generating Boing checker texture..."
	@$(GO) run ./tools/gen_boing_checker

boing-checker: $(SHOWREEL_BOING_TEXTURE)

check-showreel-prereqs:
	@echo "Checking showreel prerequisites..."
	@missing_tools=""; \
	missing_inputs=""; \
	host_build_err=""; \
	if ! command -v $(GO) >/dev/null 2>&1; then \
		missing_tools="$$missing_tools\n  - Go toolchain ($(GO))"; \
	fi; \
	if ! command -v $(UPX) >/dev/null 2>&1; then \
		missing_tools="$$missing_tools\n  - $(UPX)"; \
	fi; \
	if [ "$(SSTRIP)" != "true" ] && ! command -v $(SSTRIP) >/dev/null 2>&1; then \
		missing_tools="$$missing_tools\n  - $(SSTRIP)"; \
	fi; \
	for tool in vasmm68k_mot vasmz80_std ca65 ld65 nasm convert identify; do \
		if ! command -v $$tool >/dev/null 2>&1; then \
			missing_tools="$$missing_tools\n  - $$tool"; \
		fi; \
	done; \
	for path in $(SHOWREEL_FONT_SOURCES); do \
		if [ ! -f "$$path" ]; then \
			missing_inputs="$$missing_inputs\n  - $$path"; \
		fi; \
	done; \
	for path in $(SHOWREEL_BOING_SOURCES); do \
		if [ ! -f "$$path" ]; then \
			missing_inputs="$$missing_inputs\n  - $$path"; \
		fi; \
	done; \
	for path in $(SHOWREEL_RUNTIME_INPUTS); do \
		if [ ! -f "$$path" ]; then \
			missing_inputs="$$missing_inputs\n  - $$path"; \
		fi; \
	done; \
	if command -v $(GO) >/dev/null 2>&1; then \
		HOSTCHECK_BIN="$(BIN_DIR)/.showreel-hostcheck"; \
		$(MKDIR) -p $(BIN_DIR); \
		rm -f "$$HOSTCHECK_BIN"; \
		if ! CGO_JOBS=$(NCORES) $(GO) build $(GO_FLAGS) -o "$$HOSTCHECK_BIN" . >/dev/null 2>&1; then \
			host_build_err="  - host build prerequisites for intuition-engine (Go/CGO/Vulkan/audio/video toolchain)"; \
		fi; \
		rm -f "$$HOSTCHECK_BIN"; \
	fi; \
	if [ ! -f "$(EMUTOS_ROM)" ]; then \
		if ! command -v $(MAKE) >/dev/null 2>&1; then \
			missing_tools="$$missing_tools\n  - $(MAKE)"; \
		fi; \
		if [ ! -d "$(EMUTOS_SRC_DIR)" ]; then \
			missing_inputs="$$missing_inputs\n  - local EmuTOS source tree at $(EMUTOS_SRC_DIR) or prebuilt ROM at $(EMUTOS_ROM)"; \
		elif ! command -v m68k-atari-mint-gcc >/dev/null 2>&1 && \
		     ! command -v m68k-elf-gcc >/dev/null 2>&1 && \
		     ! command -v $(EMUTOS_LINUX_GCC) >/dev/null 2>&1 && \
		     ! command -v m68k-linux-gnu-gcc >/dev/null 2>&1 && \
		     ! command -v m68k-linux-gnu-gcc-13 >/dev/null 2>&1 && \
		     ! command -v m68k-suse-linux-gcc >/dev/null 2>&1; then \
			missing_tools="$$missing_tools\n  - supported EmuTOS M68K cross-compiler"; \
		fi; \
	fi; \
	if [ -n "$$missing_tools$$missing_inputs$$host_build_err" ]; then \
		echo "Error: showreel prerequisites are incomplete."; \
		if [ -n "$$missing_tools" ]; then \
			printf '%b\n' "Missing tools:$$missing_tools"; \
		fi; \
		if [ -n "$$missing_inputs" ]; then \
			printf '%b\n' "Missing inputs:$$missing_inputs"; \
		fi; \
		if [ -n "$$host_build_err" ]; then \
			printf '%s\n' "$$host_build_err"; \
			echo "Run 'make intuition-engine' for the full host build failure output."; \
		fi; \
		exit 1; \
	fi; \
	echo "Showreel prerequisites look complete."

showreel-emutos:
	@if [ -f "$(EMUTOS_ROM)" ]; then \
		echo "Using existing EmuTOS ROM: $(EMUTOS_ROM)"; \
	elif [ -d "$(EMUTOS_SRC_DIR)" ]; then \
		$(MAKE) emutos-rom; \
	else \
		echo "Error: missing EmuTOS ROM ($(EMUTOS_ROM)) and local source tree ($(EMUTOS_SRC_DIR))."; \
		echo "Provide one of them, then re-run 'make build-showreel-deps'."; \
		exit 1; \
	fi

showreel-ie32: ie32asm robocop-32 rotozoom-textures
	@echo "Building showreel IE32 artifacts..."
	@$(MKDIR) -p $(SHOWREEL_PREBUILT_DIR)
	@set -e; \
	for src in rotozoomer.asm voodoo_mega_demo.asm vga_mode13h_fire.asm vga_modex_circles.asm vga_mode12h_bars.asm vga_text_hello.asm; do \
		echo "  [IE32] $$src"; \
		(cd sdk/examples/asm && ../../../$(SDK_BIN_DIR)/ie32asm -I ../../include $$src); \
	done; \
	for out in rotozoomer.iex voodoo_mega_demo.iex vga_mode13h_fire.iex vga_modex_circles.iex vga_mode12h_bars.iex vga_text_hello.iex; do \
		mv sdk/examples/asm/$$out $(SHOWREEL_PREBUILT_DIR)/; \
	done

showreel-ie64: ie64asm rotozoom-textures
	@echo "Building showreel IE64 artifacts..."
	@$(MKDIR) -p $(SHOWREEL_PREBUILT_DIR)
	@set -e; \
	echo "  [IE64] ehbasic_ie64.asm"; \
	$(SDK_BIN_DIR)/ie64asm -I sdk/include sdk/examples/asm/ehbasic_ie64.asm; \
	mv sdk/examples/asm/ehbasic_ie64.ie64 $(SHOWREEL_PREBUILT_DIR)/; \
	echo "  [IE64] rotozoomer_ie64.asm"; \
	$(SDK_BIN_DIR)/ie64asm -I sdk/include sdk/examples/asm/rotozoomer_ie64.asm; \
	mv sdk/examples/asm/rotozoomer_ie64.ie64 $(SHOWREEL_PREBUILT_DIR)/; \
	echo "  [IE64] mandelbrot_ie64.asm"; \
	$(SDK_BIN_DIR)/ie64asm -I sdk/include sdk/examples/asm/mandelbrot_ie64.asm

showreel-m68k: robocop-68k gem-rotozoomer rotozoom-textures
	@echo "Building showreel M68K artifacts..."
	@$(MKDIR) -p $(SHOWREEL_PREBUILT_DIR)
	@set -e; \
	for src in rotozoomer_68k.asm ted_121_colors_68k.asm rotating_cube_copper_68k.asm voodoo_cube_68k.asm voodoo_3dfx_logo_68k.asm voodoo_triangle_68k.asm; do \
		out=$${src%.asm}.ie68; \
		echo "  [M68K] $$src"; \
		vasmm68k_mot -Fbin -m68020 -devpac -I sdk/include -o $(SHOWREEL_PREBUILT_DIR)/$$out sdk/examples/asm/$$src; \
	done

showreel-z80: robocop-z80 $(SHOWREEL_BOING_TEXTURE) rotozoom-textures
	@echo "Building showreel Z80 artifacts..."
	@$(MKDIR) -p $(SHOWREEL_PREBUILT_DIR)
	@set -e; \
	for src in rotozoomer_z80.asm voodoo_tunnel_z80.asm vga_text_sap_demo.asm; do \
		out=$${src%.asm}.ie80; \
		echo "  [Z80] $$src"; \
		vasmz80_std -Fbin -I sdk/include -o $(SHOWREEL_PREBUILT_DIR)/$$out sdk/examples/asm/$$src; \
	done

showreel-6502: rotozoomer-65 robocop-65 rotozoom-textures
	@echo "Building showreel 6502 artifacts..."
	@$(MKDIR) -p $(SHOWREEL_PREBUILT_DIR)
	@set -e; \
	echo "  [6502] ula_rotating_cube_65.asm"; \
	(cd sdk/examples/asm && \
		ca65 -I ../../include -o ula_rotating_cube_65.o ula_rotating_cube_65.asm && \
		ld65 -C ../../include/ie65.cfg -o ../prebuilt/ula_rotating_cube_65.ie65 ula_rotating_cube_65.o && \
		rm -f ula_rotating_cube_65.o)

showreel-x86: rotozoom-textures
	@echo "Building showreel x86 artifacts..."
	@$(MKDIR) -p $(SHOWREEL_PREBUILT_DIR)
	@set -e; \
	for src in rotozoomer_x86.asm antic_plasma_x86.asm; do \
		out=$${src%.asm}.ie86; \
		echo "  [x86] $$src"; \
		(cd sdk/examples/asm && nasm -f bin -I ../../include/ -o ../prebuilt/$$out $$src); \
	done

build-showreel-deps: check-showreel-prereqs intuition-engine showreel-emutos showreel-ie32 showreel-ie64 showreel-m68k showreel-z80 showreel-6502 showreel-x86
	@echo "Verifying showreel outputs..."
	@missing=""; \
	for path in $(SHOWREEL_ALL_ARTIFACTS) $(SHOWREEL_RUNTIME_INPUTS); do \
		if [ ! -f "$$path" ]; then \
			missing="$$missing\n  - $$path"; \
		fi; \
	done; \
	if [ -n "$$missing" ]; then \
		echo "Error: build-showreel-deps completed with missing files."; \
		printf '%b\n' "$$missing"; \
		exit 1; \
	fi; \
	echo "Showreel dependencies are ready for $(SHOWREEL_SCRIPT)."

run-showreel: build-showreel-deps
	@echo "Running showreel: $(SHOWREEL_SCRIPT)"
	@$(BIN_DIR)/IntuitionEngine -script $(SHOWREEL_SCRIPT)

.PHONY: gem-rotozoomer
gem-rotozoomer:
	@echo "Building GEM rotozoomer .PRG..."
	@$(MKDIR) -p sdk/examples/prebuilt
	vasmm68k_mot -Ftos -m68020 -devpac -Isdk/include \
	  -o sdk/examples/prebuilt/rotozoomer_gem.prg \
	  sdk/examples/asm/rotozoomer_gem.asm
	@echo "GEM rotozoomer built: sdk/examples/prebuilt/rotozoomer_gem.prg"

.PHONY: emutos-rom
emutos-rom:
	@if [ ! -d "$(EMUTOS_SRC_DIR)" ]; then \
		if ! command -v git >/dev/null 2>&1; then \
			echo "Error: git is required to clone EmuTOS source."; \
			echo "Install git or provide a local source tree at $(EMUTOS_SRC_DIR)."; \
			exit 1; \
		fi; \
		echo "Cloning EmuTOS source..."; \
		echo "  URL: $(EMUTOS_GIT_URL)"; \
		echo "  REF: $(EMUTOS_GIT_REF)"; \
		git clone --depth 1 --branch "$(EMUTOS_GIT_REF)" "$(EMUTOS_GIT_URL)" "$(EMUTOS_SRC_DIR)"; \
	fi
	@echo "Building EmuTOS ROM from source tree: $(EMUTOS_SRC_DIR)"
	@if command -v m68k-atari-mint-gcc >/dev/null 2>&1; then \
		$(MAKE) -C "$(EMUTOS_SRC_DIR)" clean >/dev/null; \
	elif command -v $(EMUTOS_LINUX_GCC) >/dev/null 2>&1 || command -v m68k-linux-gnu-gcc >/dev/null 2>&1 || command -v m68k-linux-gnu-gcc-13 >/dev/null 2>&1 || command -v m68k-suse-linux-gcc >/dev/null 2>&1; then \
		$(MAKE) -C "$(EMUTOS_SRC_DIR)" LINUX=1 clean >/dev/null; \
	else \
		$(MAKE) -C "$(EMUTOS_SRC_DIR)" clean >/dev/null; \
	fi
	@mkdir -p "$(EMUTOS_SRC_DIR)/obj"
	@if [ "$(EMUTOS_BUILD_CMD)" = "auto" ]; then \
		if command -v m68k-atari-mint-gcc >/dev/null 2>&1; then \
			BUILD_CMD='$(MAKE) -C $(EMUTOS_SRC_DIR) CPUFLAGS=$(EMUTOS_CPUFLAGS) DEF="$(EMUTOS_MACHINE_DEF)" EXTRA_BIOS_SRC="$(EMUTOS_EXTRA_BIOS_SRC)" $(EMUTOS_BUILD_TARGET)'; \
			echo "Using MiNT toolchain (m68k-atari-mint-gcc)"; \
		elif command -v m68k-elf-gcc >/dev/null 2>&1; then \
			BUILD_CMD='$(MAKE) -C $(EMUTOS_SRC_DIR) ELF=1 CPUFLAGS=$(EMUTOS_CPUFLAGS) DEF="$(EMUTOS_MACHINE_DEF)" EXTRA_BIOS_SRC="$(EMUTOS_EXTRA_BIOS_SRC)" $(EMUTOS_BUILD_TARGET)'; \
			echo "Using ELF toolchain (m68k-elf-gcc)"; \
		elif command -v $(EMUTOS_LINUX_GCC) >/dev/null 2>&1 || command -v m68k-linux-gnu-gcc >/dev/null 2>&1 || command -v m68k-linux-gnu-gcc-13 >/dev/null 2>&1 || command -v m68k-suse-linux-gcc >/dev/null 2>&1; then \
			if command -v $(EMUTOS_LINUX_GCC) >/dev/null 2>&1; then \
				BUILD_CMD='$(MAKE) -C $(EMUTOS_SRC_DIR) ELF=1 TOOLCHAIN_PREFIX=m68k-linux-gnu- CC=$(EMUTOS_LINUX_GCC) CPUFLAGS=$(EMUTOS_CPUFLAGS) DEF="$(EMUTOS_MACHINE_DEF)" EXTRA_BIOS_SRC="$(EMUTOS_EXTRA_BIOS_SRC)" OPTFLAGS="$(EMUTOS_LINUX_OPTFLAGS)" WARNFLAGS="$(EMUTOS_LINUX_WARNFLAGS)" $(EMUTOS_BUILD_TARGET)'; \
				echo "Using GNU/Linux M68K cross toolchain ($(EMUTOS_LINUX_GCC))"; \
			elif command -v m68k-linux-gnu-gcc-13 >/dev/null 2>&1; then \
				BUILD_CMD='$(MAKE) -C $(EMUTOS_SRC_DIR) ELF=1 TOOLCHAIN_PREFIX=m68k-linux-gnu- CC=m68k-linux-gnu-gcc-13 CPUFLAGS=$(EMUTOS_CPUFLAGS) DEF="$(EMUTOS_MACHINE_DEF)" EXTRA_BIOS_SRC="$(EMUTOS_EXTRA_BIOS_SRC)" OPTFLAGS="$(EMUTOS_LINUX_OPTFLAGS)" WARNFLAGS="$(EMUTOS_LINUX_WARNFLAGS)" $(EMUTOS_BUILD_TARGET)'; \
				echo "Using GNU/Linux M68K cross toolchain (m68k-linux-gnu-gcc-13)"; \
			elif command -v m68k-suse-linux-gcc >/dev/null 2>&1; then \
				BUILD_CMD='$(MAKE) -C $(EMUTOS_SRC_DIR) ELF=1 TOOLCHAIN_PREFIX=m68k-suse-linux- CC=m68k-suse-linux-gcc CPUFLAGS=$(EMUTOS_CPUFLAGS) DEF="$(EMUTOS_MACHINE_DEF)" EXTRA_BIOS_SRC="$(EMUTOS_EXTRA_BIOS_SRC)" OPTFLAGS="$(EMUTOS_LINUX_OPTFLAGS)" WARNFLAGS="$(EMUTOS_LINUX_WARNFLAGS)" $(EMUTOS_BUILD_TARGET)'; \
				echo "Using openSUSE M68K cross toolchain (m68k-suse-linux-gcc)"; \
			else \
				BUILD_CMD='$(MAKE) -C $(EMUTOS_SRC_DIR) ELF=1 TOOLCHAIN_PREFIX=m68k-linux-gnu- CPUFLAGS=$(EMUTOS_CPUFLAGS) DEF="$(EMUTOS_MACHINE_DEF)" EXTRA_BIOS_SRC="$(EMUTOS_EXTRA_BIOS_SRC)" OPTFLAGS="$(EMUTOS_LINUX_OPTFLAGS)" WARNFLAGS="$(EMUTOS_LINUX_WARNFLAGS)" $(EMUTOS_BUILD_TARGET)'; \
				echo "Using GNU/Linux M68K cross toolchain (m68k-linux-gnu-gcc)"; \
			fi; \
		else \
			echo "Error: EmuTOS requires a M68K cross-compiler."; \
			echo "Missing m68k-atari-mint-gcc, m68k-elf-gcc, m68k-linux-gnu-gcc, and m68k-suse-linux-gcc."; \
			echo "Install one, then re-run make emutos."; \
			echo "Or override with a custom command:"; \
			echo "  make emutos EMUTOS_BUILD_CMD='make -C $(EMUTOS_SRC_DIR) <target>'"; \
			exit 1; \
		fi; \
	else \
		BUILD_CMD='$(EMUTOS_BUILD_CMD)'; \
	fi; \
	echo "Build command: $$BUILD_CMD"; \
	eval "$$BUILD_CMD"
	@ROM_CANDIDATE=""; \
	if [ -f "$(EMUTOS_SRC_DIR)/etos256us.img" ]; then \
		ROM_CANDIDATE="$(EMUTOS_SRC_DIR)/etos256us.img"; \
	elif ls "$(EMUTOS_SRC_DIR)"/etos*.img >/dev/null 2>&1; then \
		ROM_CANDIDATE=$$(ls "$(EMUTOS_SRC_DIR)"/etos*.img | head -n 1); \
	elif [ -f "$(EMUTOS_SRC_DIR)/emutos.img" ]; then \
		ROM_CANDIDATE="$(EMUTOS_SRC_DIR)/emutos.img"; \
	else \
		ROM_CANDIDATE=$$(find "$(EMUTOS_SRC_DIR)" -type f -name '*.tos' | head -n 1); \
	fi; \
	if [ -z "$$ROM_CANDIDATE" ]; then \
		echo "Error: build completed but no ROM image was found in $(EMUTOS_SRC_DIR)."; \
		echo "Set EMUTOS_BUILD_CMD and/or copy ROM to $(EMUTOS_ROM)."; \
		exit 1; \
	fi; \
	mkdir -p "$$(dirname "$(EMUTOS_ROM)")"; \
	cp "$$ROM_CANDIDATE" "$(EMUTOS_ROM)"; \
	echo "EmuTOS ROM prepared: $(EMUTOS_ROM) (from $$ROM_CANDIDATE)"

# Build the IE64 disassembler
ie64dis: setup
	@echo "Building IE64 disassembler..."
	@$(GO) build $(GO_FLAGS) -tags ie64dis -o ie64dis assembler/ie64dis.go
	@echo "Stripping debug symbols..."
	@$(SSTRIP) -z ie64dis
	@echo "Applying UPX compression..."
	@$(UPX) --lzma ie64dis
	@$(MKDIR) -p $(SDK_BIN_DIR)
	@mv ie64dis $(SDK_BIN_DIR)/
	@echo "IE64 disassembler build complete"

# Build the IE65 data generator tool
gen-65-data: setup
	@echo "Building IE65 data generator..."
	@$(GO) build $(GO_FLAGS) -o $(BIN_DIR)/gen_65_data ./tools/gen_65_data
	@echo "IE65 data generator build complete"

# Generate and assemble bare-metal M68K CPU test suite (flat binary for Go tests)
.PHONY: cputest-bin
cputest-bin:
	@echo "Generating M68K CPU test cases..."
	@$(GO) run ./cmd/gen_m68k_cputest
	@echo "Assembling bare-metal CPU test suite..."
	@vasmm68k_mot -Fbin -m68020 -m68881 -devpac \
		-I sdk/cputest/include \
		-o sdk/cputest/cputest_suite.bin \
		sdk/cputest/cputest_suite_bare.asm
	@echo "Bare-metal CPU test binary: sdk/cputest/cputest_suite.bin"

# Validate CPU test expected values against Musashi reference 68020 core
.PHONY: cputest-musashi
cputest-musashi:
	@echo "Validating CPU test suite against Musashi reference core..."
	@CGO_ENABLED=1 $(GO) test -tags "headless musashi m68k_test" -v -run TestM68KCPUTestSuiteMusashi -timeout 300s

# Assemble an IE65 (6502) program using ca65/ld65
# Usage: make ie65asm SRC=assembler/robocop_intro_65.asm
ie65asm:
	@if [ -z "$(SRC)" ]; then \
		echo "Usage: make ie65asm SRC=<source.asm>"; \
		exit 1; \
	fi
	@echo "Assembling IE65 program: $(SRC)..."
	@BASENAME=$$(basename $(SRC) .asm); \
	SRCDIR=$$(dirname $(SRC)); \
	ca65 -I sdk/include -o $${SRCDIR}/$${BASENAME}.o $(SRC) && \
	ld65 -C sdk/include/ie65.cfg -o $${SRCDIR}/$${BASENAME}.ie65 $${SRCDIR}/$${BASENAME}.o && \
	rm -f $${SRCDIR}/$${BASENAME}.o && \
	echo "Output: $${SRCDIR}/$${BASENAME}.ie65"

# Build the Robocop IE65 (6502) demo (requires ca65/ld65 from cc65 suite)
.PHONY: robocop-65
robocop-65: $(SHOWREEL_FONT_RGBA)
	@echo "Building Robocop 6502 demo..."
	@if ! command -v ca65 >/dev/null 2>&1; then \
		echo "Error: ca65 not found. Please install the cc65 toolchain."; \
		echo "  Ubuntu/Debian: sudo apt install cc65"; \
		echo "  macOS: brew install cc65"; \
		exit 1; \
	fi
	@$(MKDIR) -p sdk/examples/prebuilt
	@cd sdk/examples/asm && ca65 -I ../../include -o robocop_intro_65.o robocop_intro_65.asm
	@cd sdk/examples/asm && ld65 -C ../../include/ie65_bindata.cfg -o ../prebuilt/robocop_intro_65.ie65 robocop_intro_65.o
	@rm -f sdk/examples/asm/robocop_intro_65.o
	@echo "Output: sdk/examples/prebuilt/robocop_intro_65.ie65"
	@ls -lh sdk/examples/prebuilt/robocop_intro_65.ie65

# Build the rotozoomer IE65 (6502) demo (requires ca65/ld65 from cc65 suite)
.PHONY: rotozoomer-65
rotozoomer-65: rotozoom-textures
	@echo "Building rotozoomer 6502 demo..."
	@if ! command -v ca65 >/dev/null 2>&1; then \
		echo "Error: ca65 not found. Please install the cc65 toolchain."; \
		echo "  Ubuntu/Debian: sudo apt install cc65"; \
		echo "  macOS: brew install cc65"; \
		exit 1; \
	fi
	@$(MKDIR) -p sdk/examples/prebuilt
	@cd sdk/examples/asm && ca65 -I ../../include -o rotozoomer_65.o rotozoomer_65.asm
	@cd sdk/examples/asm && ld65 -C ../../include/ie65.cfg -o ../prebuilt/rotozoomer_65.ie65 rotozoomer_65.o
	@rm -f sdk/examples/asm/rotozoomer_65.o
	@echo "Output: sdk/examples/prebuilt/rotozoomer_65.ie65"
	@ls -lh sdk/examples/prebuilt/rotozoomer_65.ie65

.PHONY: rotozoom-textures
rotozoom-textures: $(ROTOZOOM_VARIANT_TEXTURES)

$(ROTOZOOM_VARIANT_TEXTURES): ./sdk/examples/assets/rotozoomtexture.raw ./tools/gen_roto_textures.go
	@echo "Generating per-CPU rotozoomer textures..."
	@go run ./tools/gen_roto_textures.go

# Build the Robocop IE32 demo (requires ImageMagick for asset conversion)
.PHONY: robocop-32
robocop-32: ie32asm $(SHOWREEL_FONT_RGBA)
	@echo "Building Robocop IE32 demo..."
	@if [ ! -f "sdk/examples/assets/robocop.png" ]; then \
		echo "Error: sdk/examples/assets/robocop.png not found"; \
		exit 1; \
	fi
	@if ! command -v convert >/dev/null 2>&1; then \
		echo "Error: ImageMagick not found. Please install it."; \
		echo "  Ubuntu/Debian: sudo apt install imagemagick"; \
		echo "  macOS: brew install imagemagick"; \
		exit 1; \
	fi
	@$(MKDIR) -p sdk/examples/prebuilt
	@./sdk/scripts/robocop.sh
	@mv sdk/examples/asm/robocop_intro.iex sdk/examples/prebuilt/
	@echo "Output: sdk/examples/prebuilt/robocop_intro.iex"
	@ls -lh sdk/examples/prebuilt/robocop_intro.iex

# Build the Robocop M68K demo (requires vasmm68k_mot from VASM)
.PHONY: robocop-68k
robocop-68k: $(SHOWREEL_FONT_RGBA)
	@echo "Building Robocop M68K demo..."
	@if ! command -v vasmm68k_mot >/dev/null 2>&1; then \
		echo "Error: vasmm68k_mot not found. Please install VASM."; \
		echo "  Download from: http://sun.hasenbraten.de/vasm/"; \
		echo "  Build with: make CPU=m68k SYNTAX=mot"; \
		exit 1; \
	fi
	@$(MKDIR) -p sdk/examples/prebuilt
	@vasmm68k_mot -Fbin -m68020 -devpac \
		-I sdk/include \
		-o sdk/examples/prebuilt/robocop_intro_68k.ie68 \
		sdk/examples/asm/robocop_intro_68k.asm
	@echo "Output: sdk/examples/prebuilt/robocop_intro_68k.ie68"
	@ls -lh sdk/examples/prebuilt/robocop_intro_68k.ie68

# Build the Robocop Z80 demo (requires vasmz80 from VASM)
.PHONY: robocop-z80
robocop-z80: $(SHOWREEL_FONT_RGBA)
	@echo "Building Robocop Z80 demo..."
	@if ! command -v vasmz80_std >/dev/null 2>&1; then \
		echo "Error: vasmz80_std not found. Please install VASM."; \
		echo "  Download from: http://sun.hasenbraten.de/vasm/"; \
		echo "  Build with: make CPU=z80 SYNTAX=std"; \
		exit 1; \
	fi
	@$(MKDIR) -p sdk/examples/prebuilt
	@vasmz80_std -Fbin \
		-I sdk/include \
		-o sdk/examples/prebuilt/robocop_intro_z80.ie80 \
		sdk/examples/asm/robocop_intro_z80.asm
	@echo "Output: sdk/examples/prebuilt/robocop_intro_z80.ie80"
	@ls -lh sdk/examples/prebuilt/robocop_intro_z80.ie80

# Assemble an IE80 (Z80) program using vasmz80
# Usage: make ie80asm SRC=assembler/program.asm
.PHONY: ie80asm
ie80asm:
	@if [ -z "$(SRC)" ]; then \
		echo "Usage: make ie80asm SRC=<source.asm>"; \
		exit 1; \
	fi
	@echo "Assembling IE80 program: $(SRC)..."
	@if ! command -v vasmz80_std >/dev/null 2>&1; then \
		echo "Error: vasmz80_std not found. Please install VASM."; \
		echo "  Download from: http://sun.hasenbraten.de/vasm/"; \
		echo "  Build with: make CPU=z80 SYNTAX=std"; \
		exit 1; \
	fi
	@BASENAME=$$(basename $(SRC) .asm); \
	SRCDIR=$$(dirname $(SRC)); \
	vasmz80_std -Fbin -I sdk/include -o $${SRCDIR}/$${BASENAME}.ie80 $(SRC) && \
	echo "Output: $${SRCDIR}/$${BASENAME}.ie80"

# Build Z80 player routines for tracker format support (requires vasmz80_std)
players:
	@echo "Building Z80 player routines..."
	@if ! command -v vasmz80_std >/dev/null 2>&1; then \
		echo "Error: vasmz80_std not found. Please install VASM."; \
		echo "  Download from: http://sun.hasenbraten.de/vasm/"; \
		echo "  Build with: make CPU=z80 SYNTAX=std"; \
		exit 1; \
	fi
	@vasmz80_std -Fbin -o sdk/players/pt3play.bin sdk/players/pt3play.asm
	@echo "  pt3play.bin: $$(wc -c < sdk/players/pt3play.bin) bytes"
	@vasmz80_std -Fbin -o sdk/players/stcplay.bin sdk/players/stcplay.asm
	@echo "  stcplay.bin: $$(wc -c < sdk/players/stcplay.bin) bytes"
	@vasmz80_std -Fbin -o sdk/players/generic_play.bin sdk/players/generic_play.asm
	@echo "  generic_play.bin: $$(wc -c < sdk/players/generic_play.bin) bytes"
	@echo "Z80 player routines build complete"

# ─── SDK & Release targets ───────────────────────────────────────────────────

# Build SDK: auto-discover and pre-assemble all SDK example .asm files
sdk: clean-sdk ie32asm ie64asm ie32to64 ie64dis
	@echo "=== Building SDK ==="
	@$(MKDIR) -p sdk/examples/prebuilt
	@SDK_BUILT=0; SDK_SKIPPED=0; SDK_FAILED=0; \
	for f in sdk/examples/asm/*.asm; do \
		base=$$(basename "$$f" .asm); \
		if grep -ql 'ie64\.inc\|ie64_fp\.inc' "$$f" 2>/dev/null; then \
			echo "  [IE64] $${base}.asm"; \
			if (cd sdk/examples/asm && ../../../$(SDK_BIN_DIR)/ie64asm -I ../../include $${base}.asm); then \
				SDK_BUILT=$$((SDK_BUILT+1)); \
			else SDK_FAILED=$$((SDK_FAILED+1)); fi; \
		elif grep -ql 'ie68\.inc' "$$f" 2>/dev/null; then \
			if command -v vasmm68k_mot >/dev/null 2>&1; then \
				if grep -q '\-Ftos' "$$f" 2>/dev/null; then \
					echo "  [M68K/TOS] $${base}.asm"; \
					if (cd sdk/examples/asm && vasmm68k_mot -Ftos -m68020 -devpac -I ../../include -o $${base}.prg $${base}.asm); then \
						SDK_BUILT=$$((SDK_BUILT+1)); \
					else SDK_FAILED=$$((SDK_FAILED+1)); fi; \
				else \
					echo "  [M68K] $${base}.asm"; \
					if (cd sdk/examples/asm && vasmm68k_mot -Fbin -m68020 -devpac -I ../../include -o $${base}.ie68 $${base}.asm); then \
						SDK_BUILT=$$((SDK_BUILT+1)); \
					else SDK_FAILED=$$((SDK_FAILED+1)); fi; \
				fi; \
			else SDK_SKIPPED=$$((SDK_SKIPPED+1)); fi; \
		elif grep -ql 'ie80\.inc' "$$f" 2>/dev/null; then \
			if command -v vasmz80_std >/dev/null 2>&1; then \
				echo "  [Z80] $${base}.asm"; \
				if (cd sdk/examples/asm && vasmz80_std -Fbin -I ../../include -o $${base}.ie80 $${base}.asm); then \
					SDK_BUILT=$$((SDK_BUILT+1)); \
				else SDK_FAILED=$$((SDK_FAILED+1)); fi; \
			else SDK_SKIPPED=$$((SDK_SKIPPED+1)); fi; \
		elif grep -ql 'ie65\.inc' "$$f" 2>/dev/null; then \
			if command -v ca65 >/dev/null 2>&1; then \
				echo "  [6502] $${base}.asm"; \
				CFG=ie65.cfg; \
				if grep -q 'ie65_service' "$$f" 2>/dev/null; then CFG=ie65_service.cfg; \
				elif grep -q 'BINDATA' "$$f" 2>/dev/null; then CFG=ie65_bindata.cfg; fi; \
				if (cd sdk/examples/asm && ca65 --cpu 6502 -I ../../include -o $${base}.o $${base}.asm && \
				    ld65 -C ../../include/$${CFG} -o $${base}.ie65 $${base}.o && rm -f $${base}.o); then \
					SDK_BUILT=$$((SDK_BUILT+1)); \
				else rm -f sdk/examples/asm/$${base}.o; SDK_FAILED=$$((SDK_FAILED+1)); fi; \
			else SDK_SKIPPED=$$((SDK_SKIPPED+1)); fi; \
		elif grep -ql 'ie86\.inc\|%include' "$$f" 2>/dev/null; then \
			if command -v nasm >/dev/null 2>&1; then \
				echo "  [x86] $${base}.asm"; \
				if (cd sdk/examples/asm && nasm -f bin -I ../../include/ -o $${base}.ie86 $${base}.asm); then \
					SDK_BUILT=$$((SDK_BUILT+1)); \
				else SDK_FAILED=$$((SDK_FAILED+1)); fi; \
			else SDK_SKIPPED=$$((SDK_SKIPPED+1)); fi; \
		else \
			echo "  [IE32] $${base}.asm"; \
			if (cd sdk/examples/asm && ../../../$(SDK_BIN_DIR)/ie32asm -I ../../include $${base}.asm); then \
				SDK_BUILT=$$((SDK_BUILT+1)); \
			else SDK_FAILED=$$((SDK_FAILED+1)); fi; \
		fi; \
	done; \
	mv sdk/examples/asm/*.iex sdk/examples/prebuilt/ 2>/dev/null || true; \
	mv sdk/examples/asm/*.ie64 sdk/examples/prebuilt/ 2>/dev/null || true; \
	mv sdk/examples/asm/*.ie68 sdk/examples/prebuilt/ 2>/dev/null || true; \
	mv sdk/examples/asm/*.ie80 sdk/examples/prebuilt/ 2>/dev/null || true; \
	mv sdk/examples/asm/*.ie65 sdk/examples/prebuilt/ 2>/dev/null || true; \
	mv sdk/examples/asm/*.ie86 sdk/examples/prebuilt/ 2>/dev/null || true; \
	mv sdk/examples/asm/*.prg sdk/examples/prebuilt/ 2>/dev/null || true; \
	echo ""; \
	echo "SDK build complete: $${SDK_BUILT} assembled, $${SDK_SKIPPED} skipped, $${SDK_FAILED} failed"; \
	ls sdk/examples/prebuilt/ 2>/dev/null || true

# Reusable macro for building a Linux release archive for a given architecture.
# $(1) = GOARCH (amd64 or arm64)
# $(2) = CC
# $(3) = CXX
# $(4) = extra env vars (CGO_CFLAGS, PKG_CONFIG_*, etc. — empty for native)
define build-linux-release
	@RELEASE_NAME=$(APP_NAME)-$(APP_VERSION)-linux-$(1); \
	echo ""; \
	echo "--- $$RELEASE_NAME ---"; \
	echo "Building main binary (GOARCH=$(1), CC=$(2))..."; \
	rm -f IntuitionEngine ie32asm ie64asm ie32to64 ie64dis && \
	CGO_ENABLED=1 CGO_JOBS=$(NCORES) CC=$(2) CXX=$(3) GOARCH=$(1) $(4) \
		$(NICE) -$(NICE_LEVEL) $(GO) build $(GO_FLAGS) -tags "embed_basic embed_emutos embed_aros" -o IntuitionEngine . && \
	if [ "$(1)" = "$(NATIVE_GOARCH)" ]; then \
		command -v $(SSTRIP) >/dev/null 2>&1 && $(SSTRIP) -z IntuitionEngine || true; \
		command -v $(UPX) >/dev/null 2>&1 && $(UPX) --lzma IntuitionEngine || true; \
	fi && \
	echo "Building SDK tools (pure Go, CGO_ENABLED=0)..." && \
	CGO_ENABLED=0 GOARCH=$(1) $(GO) build $(GO_FLAGS) -o ie32asm assembler/ie32asm.go && \
	CGO_ENABLED=0 GOARCH=$(1) $(GO) build $(GO_FLAGS) -tags ie64 -o ie64asm assembler/ie64asm.go && \
	CGO_ENABLED=0 GOARCH=$(1) $(GO) build $(GO_FLAGS) -o ie32to64 ./cmd/ie32to64/ && \
	CGO_ENABLED=0 GOARCH=$(1) $(GO) build $(GO_FLAGS) -tags ie64dis -o ie64dis assembler/ie64dis.go && \
	STAGING=$(RELEASE_DIR)/$$RELEASE_NAME && \
	rm -rf $$STAGING && \
	$(MKDIR) -p $$STAGING && \
	mv IntuitionEngine $$STAGING/ && \
	cp README.md CHANGELOG.md DEVELOPERS.md $$STAGING/ && \
	cp -r sdk $$STAGING/sdk && \
	rm -rf $$STAGING/sdk/.git && \
	rm -rf $$STAGING/sdk/bin && \
	$(MKDIR) -p $$STAGING/sdk/bin && \
	mv ie32asm ie64asm ie32to64 ie64dis $$STAGING/sdk/bin/ && \
	AROS_WB="$(AROS_BUILD_DIR)/bin/ie-m68k/AROS"; \
	if [ -d "$$AROS_WB" ]; then \
		cp -r "$$AROS_WB" $$STAGING/AROS; \
	fi && \
	echo "Creating $$RELEASE_NAME.tar.xz..." && \
	tar -C $(RELEASE_DIR) -cJf $(RELEASE_DIR)/$$RELEASE_NAME.tar.xz $$RELEASE_NAME && \
	rm -rf $$STAGING && \
	echo "Created: $(RELEASE_DIR)/$$RELEASE_NAME.tar.xz"
endef

# Cross-compilation env string for pkg-config isolation + sysroot flags.
CROSS_ENV := CGO_CFLAGS="$(CROSS_CGO_CFLAGS)" CGO_CXXFLAGS="$(CROSS_CGO_CXXFLAGS)" CGO_LDFLAGS="$(CROSS_CGO_LDFLAGS)" PKG_CONFIG_PATH= PKG_CONFIG_LIBDIR="$(CROSS_PKG_CONFIG_LIBDIR)"
ifneq ($(CROSS_PKG_CONFIG_SYSROOT_DIR),)
    CROSS_ENV += PKG_CONFIG_SYSROOT_DIR="$(CROSS_PKG_CONFIG_SYSROOT_DIR)"
endif

# Build Linux release archives for both architectures (native + cross).
release-linux: setup sdk emutos-rom aros-rom
	@echo "=== Building Linux releases (amd64 + arm64) ==="
	@$(MKDIR) -p $(RELEASE_DIR)
	$(call build-linux-release,$(NATIVE_GOARCH),$(CC),$(CXX),)
	$(call build-linux-release,$(CROSS_GOARCH),$(CROSS_CC),$(CROSS_CXX),$(CROSS_ENV))

# Build Linux release archive for amd64 only.
release-linux-amd64: setup sdk emutos-rom aros-rom
	@echo "=== Building Linux release (amd64) ==="
	@$(MKDIR) -p $(RELEASE_DIR)
ifeq ($(NATIVE_GOARCH),amd64)
	$(call build-linux-release,amd64,$(CC),$(CXX),)
else
	$(call build-linux-release,amd64,$(CROSS_CC),$(CROSS_CXX),$(CROSS_ENV))
endif

# Build Linux release archive for arm64 only.
release-linux-arm64: setup sdk emutos-rom aros-rom
	@echo "=== Building Linux release (arm64) ==="
	@$(MKDIR) -p $(RELEASE_DIR)
ifeq ($(NATIVE_GOARCH),arm64)
	$(call build-linux-release,arm64,$(CC),$(CXX),)
else
	$(call build-linux-release,arm64,$(CROSS_CC),$(CROSS_CXX),$(CROSS_ENV))
endif

# Build release archives for Windows (amd64 + arm64, cross-compiled, no Vulkan)
release-windows: setup sdk emutos-rom aros-rom
	@echo "=== Building Windows releases (amd64 + arm64) ==="
	@$(MKDIR) -p $(RELEASE_DIR)
	@for goarch in amd64 arm64; do \
		RELEASE_NAME=$(APP_NAME)-$(APP_VERSION)-windows-$$goarch; \
		echo ""; \
		echo "--- $$RELEASE_NAME ---"; \
		GOOS=windows GOARCH=$$goarch $(GO) build $(GO_FLAGS) -tags "novulkan embed_basic embed_emutos embed_aros" -o IntuitionEngine.exe .; \
		GOOS=windows GOARCH=$$goarch $(GO) build $(GO_FLAGS) -o ie32asm.exe assembler/ie32asm.go; \
		GOOS=windows GOARCH=$$goarch $(GO) build $(GO_FLAGS) -tags ie64 -o ie64asm.exe assembler/ie64asm.go; \
		GOOS=windows GOARCH=$$goarch $(GO) build $(GO_FLAGS) -o ie32to64.exe ./cmd/ie32to64/; \
		GOOS=windows GOARCH=$$goarch $(GO) build $(GO_FLAGS) -tags ie64dis -o ie64dis.exe assembler/ie64dis.go; \
		STAGING=$(RELEASE_DIR)/$$RELEASE_NAME; \
		rm -rf $$STAGING; \
		$(MKDIR) -p $$STAGING; \
		mv IntuitionEngine.exe $$STAGING/; \
		cp README.md CHANGELOG.md DEVELOPERS.md $$STAGING/; \
		cp -r sdk $$STAGING/sdk; \
		rm -rf $$STAGING/sdk/.git; \
		rm -rf $$STAGING/sdk/bin; \
		$(MKDIR) -p $$STAGING/sdk/bin; \
		mv ie32asm.exe ie64asm.exe ie32to64.exe ie64dis.exe $$STAGING/sdk/bin/; \
		AROS_WB="$(AROS_BUILD_DIR)/bin/ie-m68k/AROS"; \
		if [ -d "$$AROS_WB" ]; then \
			cp -r "$$AROS_WB" $$STAGING/AROS; \
		fi; \
		echo "Creating $$RELEASE_NAME.zip..."; \
		(cd $(RELEASE_DIR) && zip -rq $$RELEASE_NAME.zip $$RELEASE_NAME); \
		rm -rf $$STAGING; \
		echo "Created: $(RELEASE_DIR)/$$RELEASE_NAME.zip"; \
	done

# Commented out: ebiten/oto require CGO on macOS/BSD. Re-enable when upstream goes purego.
# release-macos: ...
# release-freebsd: ...
# release-netbsd: ...
# release-openbsd: ...

# Clean stale SDK prebuilt artifacts
clean-sdk:
	@rm -rf sdk/examples/prebuilt
	@rm -rf $(SDK_BIN_DIR)

# Create source archive from git
release-src:
	@mkdir -p $(RELEASE_DIR)
	git archive --format=tar --prefix=IntuitionEngine-$(APP_VERSION)/ HEAD | xz -9 > $(RELEASE_DIR)/IntuitionEngine-$(APP_VERSION)-src.tar.xz
	@echo "Source archive: $(RELEASE_DIR)/IntuitionEngine-$(APP_VERSION)-src.tar.xz"

# Create standalone SDK archive
release-sdk: sdk
	@mkdir -p $(RELEASE_DIR)
	@cp -r sdk IntuitionEngine-SDK-$(APP_VERSION)
	zip -r $(RELEASE_DIR)/IntuitionEngine-SDK-$(APP_VERSION).zip IntuitionEngine-SDK-$(APP_VERSION)/
	@rm -rf IntuitionEngine-SDK-$(APP_VERSION)
	@echo "SDK archive: $(RELEASE_DIR)/IntuitionEngine-SDK-$(APP_VERSION).zip"

# Build all release archives and generate checksums
release-all: release-src release-sdk release-linux release-windows
	@echo ""
	@echo "=== Generating SHA256 checksums ==="
	@cd $(RELEASE_DIR) && sha256sum *.tar.xz *.zip 2>/dev/null > SHA256SUMS
	@echo "Checksums:"
	@cat $(RELEASE_DIR)/SHA256SUMS
	@echo ""
	@echo "All release archives:"
	@ls -lh $(RELEASE_DIR)/*.tar.xz $(RELEASE_DIR)/*.zip 2>/dev/null
	@echo ""
	@echo "Release build complete!"

# Clean build artifacts
clean:
	@echo "Cleaning build artifacts..."
	@rm -rf $(BIN_DIR)
	@rm -rf $(SDK_BIN_DIR)
	@rm -rf $(RELEASE_DIR)
	@rm -rf sdk/examples/prebuilt
	@echo "Clean complete"

# List compiled binaries with their sizes
list:
	@echo "Compiled binaries:"
	@ls -alh $(BIN_DIR)/ 2>/dev/null || true
	@ls -alh $(SDK_BIN_DIR)/ 2>/dev/null || true

# Install binaries to system (requires binaries to be built first)
install:
	@if [ ! -f "$(BIN_DIR)/IntuitionEngine" ] || [ ! -f "$(SDK_BIN_DIR)/ie32asm" ]; then \
		echo "Error: Binaries not found. Please run 'make' first to build."; \
		exit 1; \
	fi
	@echo "Installing binaries to $(INSTALL_BIN_DIR)..."
	@sudo $(INSTALL) -d $(INSTALL_BIN_DIR)
	@sudo $(INSTALL) -m 755 $(BIN_DIR)/IntuitionEngine $(INSTALL_BIN_DIR)/
	@sudo $(INSTALL) -m 755 $(SDK_BIN_DIR)/ie32asm $(INSTALL_BIN_DIR)/
	@if [ -f "$(SDK_BIN_DIR)/ie64asm" ]; then sudo $(INSTALL) -m 755 $(SDK_BIN_DIR)/ie64asm $(INSTALL_BIN_DIR)/; fi
	@if [ -f "$(SDK_BIN_DIR)/ie32to64" ]; then sudo $(INSTALL) -m 755 $(SDK_BIN_DIR)/ie32to64 $(INSTALL_BIN_DIR)/; fi
	@if [ -f "$(SDK_BIN_DIR)/ie64dis" ]; then sudo $(INSTALL) -m 755 $(SDK_BIN_DIR)/ie64dis $(INSTALL_BIN_DIR)/; fi
	@echo "Installation complete"

# Remove installed binaries
uninstall:
	@echo "Uninstalling binaries from $(INSTALL_BIN_DIR)..."
	@sudo rm -f $(INSTALL_BIN_DIR)/IntuitionEngine
	@sudo rm -f $(INSTALL_BIN_DIR)/ie32asm
	@sudo rm -f $(INSTALL_BIN_DIR)/ie64asm
	@sudo rm -f $(INSTALL_BIN_DIR)/ie32to64
	@sudo rm -f $(INSTALL_BIN_DIR)/ie64dis
	@echo "Uninstallation complete"

# Test data directories
TESTDATA_DIR := testdata
HARTE_TEST_DIR := $(TESTDATA_DIR)/68000/v1
HARTE_REPO_URL := https://github.com/SingleStepTests/680x0
OPL_TEST_DIR := $(TESTDATA_DIR)/external/opl

# Download Tom Harte 68000 test files
.PHONY: testdata-harte
testdata-harte:
	@echo "Downloading Tom Harte 68000 test files..."
	@$(MKDIR) -p $(HARTE_TEST_DIR)
	@if command -v git >/dev/null 2>&1; then \
		if [ ! -d "$(TESTDATA_DIR)/680x0" ]; then \
			echo "Cloning 680x0 test repository (this may take a while)..."; \
			git clone --depth 1 $(HARTE_REPO_URL) $(TESTDATA_DIR)/680x0; \
		fi; \
		echo "Copying 68000 v1 test files..."; \
		cp $(TESTDATA_DIR)/680x0/68000/v1/*.json.gz $(HARTE_TEST_DIR)/ 2>/dev/null || true; \
	else \
		echo "Git not found. Please install git and try again."; \
		exit 1; \
	fi
	@echo "Test files downloaded to $(HARTE_TEST_DIR)/"
	@ls -1 $(HARTE_TEST_DIR)/*.json.gz 2>/dev/null | wc -l | xargs echo "Total test files:"

# Download pinned OPL test fixtures
testdata-opl:
	@echo "Fetching pinned OPL test fixtures..."
	@./scripts/fetch_opl_testdata.sh
	@echo "OPL fixtures ready in $(OPL_TEST_DIR)/"

# Clean test data
.PHONY: clean-testdata
clean-testdata:
	@echo "Cleaning test data..."
	@rm -rf $(TESTDATA_DIR)
	@echo "Test data cleaned"

# Run M68K tests with Tom Harte test suite
.PHONY: test-harte
test-harte: testdata-harte
	@echo "Running Tom Harte 68000 tests..."
	@$(GO) test -v -run TestHarte68000 -timeout 30m

# Run M68K tests in short mode (sampling)
.PHONY: test-harte-short
test-harte-short: testdata-harte
	@echo "Running Tom Harte 68000 tests (short mode)..."
	@$(GO) test -v -short -run TestHarte68000 -timeout 5m

# Install desktop entry and MIME type for file association
.PHONY: install-desktop-entry
install-desktop-entry:
	@echo "Installing desktop entry and MIME type..."
	@$(INSTALL) -D assets/intuition-engine.desktop $(DESTDIR)$(PREFIX)/share/applications/intuition-engine.desktop
	@$(INSTALL) -D assets/intuition-engine-mime.xml $(DESTDIR)$(PREFIX)/share/mime/packages/intuition-engine-mime.xml
	-update-mime-database $(DESTDIR)$(PREFIX)/share/mime 2>/dev/null || true
	-update-desktop-database $(DESTDIR)$(PREFIX)/share/applications 2>/dev/null || true
	@echo "Desktop entry and MIME type installed"

# Set Intuition Engine as default handler for .ie* files (per-user)
.PHONY: set-default-handler
set-default-handler:
	@xdg-mime default intuition-engine.desktop application/x-intuition-engine
	@echo "Intuition Engine set as default handler for .ie* files"

# Help target
help:
	@echo "Intuition Engine Build System"
	@echo ""
	@echo "Available targets:"
	@echo "  all              - Build Intuition Engine + assemblers (default, full)"
	@echo "  intuition-engine - Build only the Intuition Engine VM (full)"
	@echo "  novulkan         - Build without Vulkan (software Voodoo only)"
	@echo "  headless         - Build without display/audio (CI/testing)"
	@echo "  headless-novulkan - Fully portable CGO_ENABLED=0 build"
	@echo "  ie32asm          - Build only the IE32 assembler"
	@echo "  ie64asm          - Build only the IE64 assembler"
	@echo "  ie64dis          - Build only the IE64 disassembler"
	@echo "  basic            - Build with embedded EhBASIC interpreter"
	@echo "  emutos           - Build with embedded EmuTOS ROM (embed_emutos tag)"
	@echo "  aros             - Build with embedded AROS ROM (embed_aros tag)"
	@echo "  install          - Install binaries to $(INSTALL_BIN_DIR)"
	@echo "  uninstall        - Remove installed binaries from $(INSTALL_BIN_DIR)"
	@echo "  clean            - Remove all build artifacts"
	@echo "  list             - List compiled binaries with sizes"
	@echo "  help             - Show this help message"
	@echo ""
	@echo "SDK & Release targets:"
	@echo "  sdk              - Sync includes and pre-assemble SDK demos"
	@echo "  build-showreel-deps - Build the exact artifacts needed by sdk/scripts/ie_product_demo.ies"
	@echo "  run-showreel     - Build showreel dependencies, then launch sdk/scripts/ie_product_demo.ies"
	@echo "  check-showreel-prereqs - Validate showreel toolchains, runtime inputs, and EmuTOS availability"
	@echo "  release-src      - Create source archive from git"
	@echo "  release-sdk      - Create standalone SDK archive"
	@echo "  release-linux       - Build Linux release archives (amd64 + arm64)"
	@echo "  release-linux-amd64 - Build Linux release archive (amd64 only)"
	@echo "  release-linux-arm64 - Build Linux release archive (arm64 only)"
	@echo "  release-windows  - Build Windows release archives (amd64 + arm64)"
	@echo "  release-all      - Build all release archives + SHA256SUMS"
	@echo ""
	@echo "Demo targets:"
	@echo "  robocop-32     - Build the Robocop IE32 demo (requires ImageMagick)"
	@echo "  robocop-65     - Build the Robocop 6502 demo (requires cc65)"
	@echo "  robocop-68k    - Build the Robocop M68K demo (requires vasm)"
	@echo "  robocop-z80    - Build the Robocop Z80 demo (requires vasm)"
	@echo "  gem-rotozoomer - Build the GEM rotozoomer .PRG (requires vasm)"
	@echo "  showreel-ie32  - Build the IE32 binaries used by the product demo"
	@echo "  showreel-ie64  - Build the IE64 binaries used by the product demo"
	@echo "  showreel-m68k  - Build the M68K binaries used by the product demo"
	@echo "  showreel-z80   - Build the Z80 binaries used by the product demo"
	@echo "  showreel-6502  - Build the 6502 binaries used by the product demo"
	@echo "  showreel-x86   - Build the x86 binaries used by the product demo"
	@echo ""
	@echo "IEScript:"
	@echo "  sdk/scripts/ie_product_demo.ies expects these assets; use 'make build-showreel-deps' first"
	@echo ""
	@echo "IE65 (6502) targets:"
	@echo "  gen-65-data    - Build the IE65 data generator tool"
	@echo "  ie65asm        - Assemble an IE65 program (SRC=file.asm)"
	@echo "  cputest-bin      - Generate and assemble bare-metal M68K CPU test binary"
	@echo "  cputest-musashi  - Validate CPU test expected values against Musashi 68020"
	@echo ""
	@echo "IE80 (Z80) targets:"
	@echo "  ie80asm        - Assemble an IE80 program (SRC=file.asm)"
	@echo "  players        - Rebuild Z80 player routines for tracker formats"
	@echo ""
	@echo "Test targets:"
	@echo "  testdata-harte   - Download Tom Harte 68000 test files"
	@echo "  test-harte       - Run full Tom Harte test suite"
	@echo "  test-harte-short - Run Tom Harte tests (sampling mode)"
	@echo "  clean-testdata   - Remove downloaded test data"
	@echo ""
	@echo "Desktop integration:"
	@echo "  install-desktop-entry - Install .desktop and MIME type for file association"
	@echo "  set-default-handler   - Set as default handler for .ie* files (per-user)"
	@echo ""
	@echo "Build flags:"
	@echo "  GO_FLAGS       = $(GO_FLAGS)"
	@echo "  NCORES        = $(NCORES)"
	@echo "  NICE_LEVEL    = $(NICE_LEVEL)"
	@echo ""
	@echo "Installation paths:"
	@echo "  PREFIX        = $(PREFIX)"
	@echo "  INSTALL_BIN_DIR = $(INSTALL_BIN_DIR)"
