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

# Version metadata
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
BUILD_DATE := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

# Go build flags with version injection
GO_FLAGS := -ldflags "-s -w -X main.Version=$(APP_VERSION) -X main.Commit=$(COMMIT) -X main.BuildDate=$(BUILD_DATE)"

# Commands and tools
GO := go
SSTRIP := sstrip
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

# Build profiles:
#   make                      Full build (Vulkan + Ebiten + OTO + liblhasa)
#   make novulkan             Software Voodoo only (no Vulkan SDK needed)
#   make headless             No display, no audio, no Vulkan (CI/testing)
#   make headless-novulkan    CGO_ENABLED=0 portable build (cross-compile safe)

# Release directories
RELEASE_DIR := ./release

# Main targets
.PHONY: all clean list install uninstall novulkan headless headless-novulkan
.PHONY: sdk clean-sdk release-src release-sdk release-linux release-windows release-all

# Default target builds everything
all: setup intuition-engine ie32asm ie64asm ie32to64 ie64dis
	@echo "Build complete! Executables are in $(BIN_DIR)/"
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

# Build headless (no display, no audio, no Vulkan — for CI/testing)
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
	@mv ie32asm $(BIN_DIR)/
	@echo "IE32 assembler build complete"

# Build the IE64 assembler
ie64asm: setup
	@echo "Building IE64 assembler..."
	@$(GO) build $(GO_FLAGS) -tags ie64 -o ie64asm assembler/ie64asm.go
	@echo "Stripping debug symbols..."
	@$(SSTRIP) -z ie64asm
	@echo "Applying UPX compression..."
	@$(UPX) --lzma ie64asm
	@mv ie64asm $(BIN_DIR)/
	@echo "IE64 assembler build complete"

# Build the IE32-to-IE64 converter
ie32to64: setup
	@echo "Building IE32-to-IE64 converter..."
	@$(GO) build $(GO_FLAGS) -o ie32to64 ./cmd/ie32to64/
	@mv ie32to64 $(BIN_DIR)/
	@echo "IE32-to-IE64 converter build complete"

# Build with embedded EhBASIC BASIC interpreter
.PHONY: basic
basic: ie64asm
	@echo "Assembling EhBASIC IE64 interpreter..."
	@$(BIN_DIR)/ie64asm assembler/ehbasic_ie64.asm
	@echo "Building Intuition Engine with embedded BASIC..."
	@CGO_JOBS=$(NCORES) $(NICE) -$(NICE_LEVEL) $(GO) build $(GO_FLAGS) -tags embed_basic .
	@echo "Stripping debug symbols..."
	@$(NICE) -$(NICE_LEVEL) $(SSTRIP) -z IntuitionEngine
	@echo "Applying UPX compression..."
	@$(NICE) -$(NICE_LEVEL) $(UPX) --lzma IntuitionEngine
	@mv IntuitionEngine $(BIN_DIR)/
	@echo "EhBASIC build complete — run with: $(BIN_DIR)/IntuitionEngine -basic"

# Build the IE64 disassembler
ie64dis: setup
	@echo "Building IE64 disassembler..."
	@$(GO) build $(GO_FLAGS) -tags ie64dis -o ie64dis assembler/ie64dis.go
	@echo "Stripping debug symbols..."
	@$(SSTRIP) -z ie64dis
	@echo "Applying UPX compression..."
	@$(UPX) --lzma ie64dis
	@mv ie64dis $(BIN_DIR)/
	@echo "IE64 disassembler build complete"

# Build the IE65 data generator tool
gen-65-data: setup
	@echo "Building IE65 data generator..."
	@$(GO) build $(GO_FLAGS) -o $(BIN_DIR)/gen_65_data ./tools/gen_65_data
	@echo "IE65 data generator build complete"

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
	ca65 -I assembler -o $${SRCDIR}/$${BASENAME}.o $(SRC) && \
	ld65 -C assembler/ie65.cfg -o $${SRCDIR}/$${BASENAME}.ie65 $${SRCDIR}/$${BASENAME}.o && \
	rm -f $${SRCDIR}/$${BASENAME}.o && \
	echo "Output: $${SRCDIR}/$${BASENAME}.ie65"

# Build the Robocop IE65 (6502) demo (requires ca65/ld65 from cc65 suite)
.PHONY: robocop-65
robocop-65:
	@echo "Building Robocop 6502 demo..."
	@if ! command -v ca65 >/dev/null 2>&1; then \
		echo "Error: ca65 not found. Please install the cc65 toolchain."; \
		echo "  Ubuntu/Debian: sudo apt install cc65"; \
		echo "  macOS: brew install cc65"; \
		exit 1; \
	fi
	@cd assembler && ca65 -o robocop_intro_65.o robocop_intro_65.asm
	@cd assembler && ld65 -C ie65.cfg -o robocop_intro_65.ie65 robocop_intro_65.o
	@rm -f assembler/robocop_intro_65.o
	@echo "Output: assembler/robocop_intro_65.ie65"
	@ls -lh assembler/robocop_intro_65.ie65

# Build the rotozoomer IE65 (6502) demo (requires ca65/ld65 from cc65 suite)
.PHONY: rotozoomer-65
rotozoomer-65:
	@echo "Building rotozoomer 6502 demo..."
	@if ! command -v ca65 >/dev/null 2>&1; then \
		echo "Error: ca65 not found. Please install the cc65 toolchain."; \
		echo "  Ubuntu/Debian: sudo apt install cc65"; \
		echo "  macOS: brew install cc65"; \
		exit 1; \
	fi
	@cd assembler && ca65 -o rotozoomer_65.o rotozoomer_65.asm
	@cd assembler && ld65 -C ie65.cfg -o rotozoomer_65.ie65 rotozoomer_65.o
	@rm -f assembler/rotozoomer_65.o
	@echo "Output: assembler/rotozoomer_65.ie65"
	@ls -lh assembler/rotozoomer_65.ie65

# Build the Robocop IE32 demo (requires ImageMagick for asset conversion)
.PHONY: robocop-32
robocop-32:
	@echo "Building Robocop IE32 demo..."
	@if [ ! -f "robocop.png" ]; then \
		echo "Error: robocop.png not found"; \
		exit 1; \
	fi
	@if ! command -v convert >/dev/null 2>&1; then \
		echo "Error: ImageMagick not found. Please install it."; \
		echo "  Ubuntu/Debian: sudo apt install imagemagick"; \
		echo "  macOS: brew install imagemagick"; \
		exit 1; \
	fi
	@./robocop.sh
	@ls -lh assembler/robocop_intro.iex

# Build the Robocop M68K demo (requires vasmm68k_mot from VASM)
.PHONY: robocop-68k
robocop-68k:
	@echo "Building Robocop M68K demo..."
	@if ! command -v vasmm68k_mot >/dev/null 2>&1; then \
		echo "Error: vasmm68k_mot not found. Please install VASM."; \
		echo "  Download from: http://sun.hasenbraten.de/vasm/"; \
		echo "  Build with: make CPU=m68k SYNTAX=mot"; \
		exit 1; \
	fi
	@vasmm68k_mot -Fbin -m68020 -devpac \
		-o assembler/robocop_intro_68k.ie68 \
		assembler/robocop_intro_68k.asm
	@echo "Output: assembler/robocop_intro_68k.ie68"
	@ls -lh assembler/robocop_intro_68k.ie68

# Build the Robocop Z80 demo (requires vasmz80 from VASM)
.PHONY: robocop-z80
robocop-z80:
	@echo "Building Robocop Z80 demo..."
	@if ! command -v vasmz80_std >/dev/null 2>&1; then \
		echo "Error: vasmz80_std not found. Please install VASM."; \
		echo "  Download from: http://sun.hasenbraten.de/vasm/"; \
		echo "  Build with: make CPU=z80 SYNTAX=std"; \
		exit 1; \
	fi
	@vasmz80_std -Fbin \
		-I assembler \
		-o assembler/robocop_intro_z80.ie80 \
		assembler/robocop_intro_z80.asm
	@echo "Output: assembler/robocop_intro_z80.ie80"
	@ls -lh assembler/robocop_intro_z80.ie80

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
	vasmz80_std -Fbin -I assembler -o $${SRCDIR}/$${BASENAME}.ie80 $(SRC) && \
	echo "Output: $${SRCDIR}/$${BASENAME}.ie80"

# ─── SDK & Release targets ───────────────────────────────────────────────────

# Build SDK: sync includes from canonical source and pre-assemble demos
sdk: clean-sdk ie32asm ie64asm
	@echo "=== Building SDK ==="
	@# Sync include files from canonical source
	@echo "Syncing include files..."
	@cp assembler/ie32.inc assembler/ie64.inc assembler/ie65.inc assembler/ie65.cfg \
	    assembler/ie68.inc assembler/ie80.inc assembler/ie86.inc sdk/include/
	@$(MKDIR) -p sdk/examples/prebuilt
	@SDK_BUILT=0; SDK_SKIPPED=0; \
	echo "Assembling IE32 examples..."; \
	for f in rotozoomer vga_text_hello vga_mode13h_fire copper_vga_bands \
	         coproc_caller_ie32; do \
		echo "  [IE32] $${f}.asm"; \
		(cd sdk/examples/asm && ../../../$(BIN_DIR)/ie32asm -I ../../include $${f}.asm) && \
		SDK_BUILT=$$((SDK_BUILT+1)); \
	done; \
	echo "Assembling IE64 examples..."; \
	for f in rotozoomer_ie64; do \
		echo "  [IE64] $${f}.asm"; \
		(cd sdk/examples/asm && ../../../$(BIN_DIR)/ie64asm -I ../../include $${f}.asm) && \
		SDK_BUILT=$$((SDK_BUILT+1)); \
	done; \
	mv sdk/examples/asm/*.iex sdk/examples/prebuilt/ 2>/dev/null || true; \
	mv sdk/examples/asm/*.ie64 sdk/examples/prebuilt/ 2>/dev/null || true; \
	if command -v vasmm68k_mot >/dev/null 2>&1; then \
		echo "Assembling M68K examples..."; \
		for f in rotozoomer_68k ted_121_colors_68k voodoo_cube_68k; do \
			echo "  [M68K] $${f}.asm"; \
			(cd sdk/examples/asm && vasmm68k_mot -Fbin -m68020 -devpac -I ../../include -o $${f}.ie68 $${f}.asm) && \
			SDK_BUILT=$$((SDK_BUILT+1)); \
		done; \
		mv sdk/examples/asm/*.ie68 sdk/examples/prebuilt/ 2>/dev/null || true; \
	else \
		echo "Skipping M68K examples (vasmm68k_mot not found)"; \
		SDK_SKIPPED=$$((SDK_SKIPPED+3)); \
	fi; \
	if command -v vasmz80_std >/dev/null 2>&1; then \
		echo "Assembling Z80 examples..."; \
		for f in rotozoomer_z80; do \
			echo "  [Z80] $${f}.asm"; \
			(cd sdk/examples/asm && vasmz80_std -Fbin -I ../../include -o $${f}.ie80 $${f}.asm) && \
			SDK_BUILT=$$((SDK_BUILT+1)); \
		done; \
		mv sdk/examples/asm/*.ie80 sdk/examples/prebuilt/ 2>/dev/null || true; \
	else \
		echo "Skipping Z80 examples (vasmz80_std not found)"; \
		SDK_SKIPPED=$$((SDK_SKIPPED+1)); \
	fi; \
	if command -v ca65 >/dev/null 2>&1; then \
		echo "Assembling 6502 examples..."; \
		for f in rotozoomer_65 ula_rotating_cube_65; do \
			echo "  [6502] $${f}.asm"; \
			(cd sdk/examples/asm && ca65 --cpu 6502 -I ../../include -o $${f}.o $${f}.asm && \
			 ld65 -C ../../include/ie65.cfg -o $${f}.ie65 $${f}.o && rm -f $${f}.o) && \
			SDK_BUILT=$$((SDK_BUILT+1)); \
		done; \
		mv sdk/examples/asm/*.ie65 sdk/examples/prebuilt/ 2>/dev/null || true; \
	else \
		echo "Skipping 6502 examples (ca65 not found)"; \
		SDK_SKIPPED=$$((SDK_SKIPPED+2)); \
	fi; \
	if command -v nasm >/dev/null 2>&1; then \
		echo "Assembling x86 examples..."; \
		for f in rotozoomer_x86 antic_plasma_x86; do \
			echo "  [x86] $${f}.asm"; \
			(cd sdk/examples/asm && nasm -f bin -I ../../include/ -o $${f}.ie86 $${f}.asm) && \
			SDK_BUILT=$$((SDK_BUILT+1)); \
		done; \
		mv sdk/examples/asm/*.ie86 sdk/examples/prebuilt/ 2>/dev/null || true; \
	else \
		echo "Skipping x86 examples (nasm not found)"; \
		SDK_SKIPPED=$$((SDK_SKIPPED+2)); \
	fi; \
	echo ""; \
	echo "SDK build complete: $${SDK_BUILT} assembled, $${SDK_SKIPPED} skipped"; \
	ls sdk/examples/prebuilt/ 2>/dev/null || true

# Build release archive for Linux (native architecture only)
# Ebiten/Oto require CGO (GLFW/X11/ALSA), so cross-compilation is not supported.
release-linux: setup sdk
	@echo "=== Building Linux release ($(NATIVE_GOARCH)) ==="
	@$(MKDIR) -p $(RELEASE_DIR)
	@echo "Assembling EhBASIC IE64 ROM..."
	@$(BIN_DIR)/ie64asm assembler/ehbasic_ie64.asm
	@RELEASE_NAME=$(APP_NAME)-$(APP_VERSION)-linux-$(NATIVE_GOARCH); \
	echo ""; \
	echo "--- $$RELEASE_NAME ---"; \
	echo "Building (native, full)..."; \
	CGO_JOBS=$(NCORES) $(NICE) -$(NICE_LEVEL) $(GO) build $(GO_FLAGS) -tags embed_basic -o IntuitionEngine .; \
	command -v $(SSTRIP) >/dev/null 2>&1 && $(SSTRIP) -z IntuitionEngine || true; \
	command -v $(UPX) >/dev/null 2>&1 && $(UPX) --lzma IntuitionEngine || true; \
	$(GO) build $(GO_FLAGS) -o ie32asm assembler/ie32asm.go; \
	$(GO) build $(GO_FLAGS) -tags ie64 -o ie64asm assembler/ie64asm.go; \
	$(GO) build $(GO_FLAGS) -o ie32to64 ./cmd/ie32to64/; \
	STAGING=$(RELEASE_DIR)/$$RELEASE_NAME; \
	rm -rf $$STAGING; \
	$(MKDIR) -p $$STAGING; \
	mv IntuitionEngine ie32asm ie64asm ie32to64 $$STAGING/; \
	cp README.md CHANGELOG.md DEVELOPERS.md $$STAGING/; \
	cp -r docs $$STAGING/docs; \
	cp -r sdk $$STAGING/sdk; \
	rm -rf $$STAGING/sdk/.git; \
	echo "Creating $$RELEASE_NAME.tar.xz..."; \
	tar -C $(RELEASE_DIR) -cJf $(RELEASE_DIR)/$$RELEASE_NAME.tar.xz $$RELEASE_NAME; \
	rm -rf $$STAGING; \
	echo "Created: $(RELEASE_DIR)/$$RELEASE_NAME.tar.xz"

# Build release archives for Windows (amd64 + arm64, cross-compiled, no Vulkan)
release-windows: setup sdk
	@echo "=== Building Windows releases (amd64 + arm64) ==="
	@$(MKDIR) -p $(RELEASE_DIR)
	@echo "Assembling EhBASIC IE64 ROM..."
	@$(BIN_DIR)/ie64asm assembler/ehbasic_ie64.asm
	@for goarch in amd64 arm64; do \
		RELEASE_NAME=$(APP_NAME)-$(APP_VERSION)-windows-$$goarch; \
		echo ""; \
		echo "--- $$RELEASE_NAME ---"; \
		GOOS=windows GOARCH=$$goarch $(GO) build $(GO_FLAGS) -tags "novulkan embed_basic" -o IntuitionEngine.exe .; \
		GOOS=windows GOARCH=$$goarch $(GO) build $(GO_FLAGS) -o ie32asm.exe assembler/ie32asm.go; \
		GOOS=windows GOARCH=$$goarch $(GO) build $(GO_FLAGS) -tags ie64 -o ie64asm.exe assembler/ie64asm.go; \
		GOOS=windows GOARCH=$$goarch $(GO) build $(GO_FLAGS) -o ie32to64.exe ./cmd/ie32to64/; \
		STAGING=$(RELEASE_DIR)/$$RELEASE_NAME; \
		rm -rf $$STAGING; \
		$(MKDIR) -p $$STAGING; \
		mv IntuitionEngine.exe ie32asm.exe ie64asm.exe ie32to64.exe $$STAGING/; \
		cp README.md CHANGELOG.md DEVELOPERS.md $$STAGING/; \
		cp -r docs $$STAGING/docs; \
		cp -r sdk $$STAGING/sdk; \
		rm -rf $$STAGING/sdk/.git; \
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
	@rm -rf $(RELEASE_DIR)
	@rm -rf sdk/examples/prebuilt
	@echo "Clean complete"

# List compiled binaries with their sizes
list:
	@echo "Compiled binaries:"
	@ls -alh $(BIN_DIR)/

# Install binaries to system (requires binaries to be built first)
install:
	@if [ ! -f "$(BIN_DIR)/IntuitionEngine" ] || [ ! -f "$(BIN_DIR)/ie32asm" ]; then \
		echo "Error: Binaries not found in $(BIN_DIR)/"; \
		echo "Please run 'make' first to build the binaries."; \
		exit 1; \
	fi
	@echo "Installing binaries to $(INSTALL_BIN_DIR)..."
	@sudo $(INSTALL) -d $(INSTALL_BIN_DIR)
	@sudo $(INSTALL) -m 755 $(BIN_DIR)/IntuitionEngine $(INSTALL_BIN_DIR)/
	@sudo $(INSTALL) -m 755 $(BIN_DIR)/ie32asm $(INSTALL_BIN_DIR)/
	@if [ -f "$(BIN_DIR)/ie64asm" ]; then sudo $(INSTALL) -m 755 $(BIN_DIR)/ie64asm $(INSTALL_BIN_DIR)/; fi
	@if [ -f "$(BIN_DIR)/ie32to64" ]; then sudo $(INSTALL) -m 755 $(BIN_DIR)/ie32to64 $(INSTALL_BIN_DIR)/; fi
	@if [ -f "$(BIN_DIR)/ie64dis" ]; then sudo $(INSTALL) -m 755 $(BIN_DIR)/ie64dis $(INSTALL_BIN_DIR)/; fi
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
	@echo "  install          - Install binaries to $(INSTALL_BIN_DIR)"
	@echo "  uninstall        - Remove installed binaries from $(INSTALL_BIN_DIR)"
	@echo "  clean            - Remove all build artifacts"
	@echo "  list             - List compiled binaries with sizes"
	@echo "  help             - Show this help message"
	@echo ""
	@echo "SDK & Release targets:"
	@echo "  sdk              - Sync includes and pre-assemble SDK demos"
	@echo "  release-src      - Create source archive from git"
	@echo "  release-sdk      - Create standalone SDK archive"
	@echo "  release-linux    - Build Linux release archive (native arch)"
	@echo "  release-windows  - Build Windows release archives (amd64 + arm64)"
	@echo "  release-all      - Build all release archives + SHA256SUMS"
	@echo ""
	@echo "Demo targets:"
	@echo "  robocop-32     - Build the Robocop IE32 demo (requires ImageMagick)"
	@echo "  robocop-65     - Build the Robocop 6502 demo (requires cc65)"
	@echo "  robocop-68k    - Build the Robocop M68K demo (requires vasm)"
	@echo "  robocop-z80    - Build the Robocop Z80 demo (requires vasm)"
	@echo ""
	@echo "IE65 (6502) targets:"
	@echo "  gen-65-data    - Build the IE65 data generator tool"
	@echo "  ie65asm        - Assemble an IE65 program (SRC=file.asm)"
	@echo ""
	@echo "IE80 (Z80) targets:"
	@echo "  ie80asm        - Assemble an IE80 program (SRC=file.asm)"
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
