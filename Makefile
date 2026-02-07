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
# - AppImage packaging for Linux distribution

# Directory structure
BIN_DIR := ./bin

# Detect number of CPU cores for parallel compilation
NCORES := $(shell nproc)

# Detect architecture for AppImage
ARCH := $(shell uname -m)
ifeq ($(ARCH),x86_64)
    APPIMAGE_TOOL_URL := https://github.com/AppImage/AppImageKit/releases/download/continuous/appimagetool-x86_64.AppImage
    APPIMAGE_TOOL := appimagetool-x86_64.AppImage
else ifeq ($(ARCH),aarch64)
    APPIMAGE_TOOL_URL := https://github.com/AppImage/AppImageKit/releases/download/continuous/appimagetool-aarch64.AppImage
    APPIMAGE_TOOL := appimagetool-aarch64.AppImage
endif

# Go build flags
GO_FLAGS := -ldflags "-s -w"

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

# AppImage related directories and files
APPIMAGE_DIR := ./AppDir
APPIMAGE_BIN_DIR := $(APPIMAGE_DIR)/usr/bin
APPIMAGE_DESKTOP_DIR := $(APPIMAGE_DIR)/usr/share/applications
APPIMAGE_ICON_DIR := $(APPIMAGE_DIR)/usr/share/icons/hicolor/256x256/apps

# Application metadata
APP_NAME := IntuitionEngine
APP_VERSION := 1.0.0

# Main targets
.PHONY: all clean list install uninstall

# Default target builds everything
all: setup intuition-engine ie32asm ie64asm
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

# Download AppImage Tool if not present
$(APPIMAGE_TOOL):
	@echo "Downloading AppImage Tool for $(ARCH)..."
	@wget $(APPIMAGE_TOOL_URL)
	@chmod +x $(APPIMAGE_TOOL)

# Create AppImage directory structure
appimage-structure: setup
	@echo "Creating AppImage directory structure..."
	@$(MKDIR) -p $(APPIMAGE_BIN_DIR)
	@$(MKDIR) -p $(APPIMAGE_DESKTOP_DIR)
	@$(MKDIR) -p $(APPIMAGE_ICON_DIR)

# Create desktop entry file
.PHONY: desktop-entry
desktop-entry: appimage-structure
	@echo "Creating desktop entry..."
	@echo "Desktop entry path: $(APPIMAGE_DESKTOP_DIR)/$(APP_NAME).desktop"
	@echo "[Desktop Entry]" > $(APPIMAGE_DESKTOP_DIR)/$(APP_NAME).desktop
	@echo "Name=$(APP_NAME)" >> $(APPIMAGE_DESKTOP_DIR)/$(APP_NAME).desktop
	@echo "Exec=IntuitionEngine" >> $(APPIMAGE_DESKTOP_DIR)/$(APP_NAME).desktop
	@echo "Icon=$(APP_NAME)" >> $(APPIMAGE_DESKTOP_DIR)/$(APP_NAME).desktop
	@echo "Type=Application" >> $(APPIMAGE_DESKTOP_DIR)/$(APP_NAME).desktop
	@echo "Categories=Development;" >> $(APPIMAGE_DESKTOP_DIR)/$(APP_NAME).desktop
	@echo "Version=1.0" >> $(APPIMAGE_DESKTOP_DIR)/$(APP_NAME).desktop
	@echo "Terminal=false" >> $(APPIMAGE_DESKTOP_DIR)/$(APP_NAME).desktop
	@# Create symlink in AppDir root
	@ln -sf usr/share/applications/$(APP_NAME).desktop $(APPIMAGE_DIR)/$(APP_NAME).desktop
	@echo "Desktop entry file contents:"
	@cat $(APPIMAGE_DESKTOP_DIR)/$(APP_NAME).desktop

# Copy binaries and resources to AppImage structure
.PHONY: copy-binaries
copy-binaries: intuition-engine ie32asm appimage-structure
	@echo "Copying binaries and resources to AppImage structure..."
	@cp $(BIN_DIR)/IntuitionEngine $(APPIMAGE_BIN_DIR)/
	@cp $(BIN_DIR)/ie32asm $(APPIMAGE_BIN_DIR)/
	@cp IntuitionEngine.png $(APPIMAGE_ICON_DIR)/$(APP_NAME).png
	@cp IntuitionEngine.png $(APPIMAGE_DIR)/$(APP_NAME).png

# Create AppRun script
.PHONY: apprun
apprun: appimage-structure
	@echo "Creating AppRun script..."
	@echo '#!/bin/bash' > $(APPIMAGE_DIR)/AppRun
	@echo 'SELF="$$(dirname "$$(readlink -f "$$0")")"' >> $(APPIMAGE_DIR)/AppRun
	@echo 'export PATH="$$SELF/usr/bin:$$PATH"' >> $(APPIMAGE_DIR)/AppRun
	@echo 'exec "$$SELF/usr/bin/IntuitionEngine" "$$@"' >> $(APPIMAGE_DIR)/AppRun
	@chmod +x $(APPIMAGE_DIR)/AppRun

# Build AppImage package
.PHONY: appimage
appimage: $(APPIMAGE_TOOL) copy-binaries desktop-entry apprun
	@echo "Building AppImage package..."
	@echo "Verifying AppDir structure..."
	@# Check required files
	@if [ ! -f "$(APPIMAGE_DIR)/AppRun" ]; then \
		echo "Error: AppRun file missing"; \
		exit 1; \
	fi
	@if [ ! -f "$(APPIMAGE_DESKTOP_DIR)/$(APP_NAME).desktop" ]; then \
		echo "Error: Desktop file missing"; \
		ls -la $(APPIMAGE_DESKTOP_DIR); \
		exit 1; \
	fi
	@if [ ! -f "$(APPIMAGE_BIN_DIR)/IntuitionEngine" ]; then \
		echo "Error: Main binary missing"; \
		exit 1; \
	fi
	@if [ ! -f "$(APPIMAGE_ICON_DIR)/$(APP_NAME).png" ]; then \
		echo "Error: Application icon missing"; \
		exit 1; \
	fi
	@echo "AppDir structure verified"
	@echo "AppDir contents:"
	@find $(APPIMAGE_DIR) -type f
	@echo "Using architecture $(ARCH)"
	@# Create the AppImage
	@ARCH=$(ARCH) ./$(APPIMAGE_TOOL) $(APPIMAGE_DIR) $(APP_NAME)-$(APP_VERSION)-$(ARCH).AppImage
	@echo "AppImage created: $(APP_NAME)-$(APP_VERSION)-$(ARCH).AppImage"

# Clean build artifacts
clean: clean-appimage
	@echo "Cleaning build artifacts..."
	@rm -rf $(BIN_DIR)
	@echo "Clean complete"

# Clean AppImage artifacts
.PHONY: clean-appimage
clean-appimage:
	@echo "Cleaning AppImage artifacts..."
	@rm -rf $(APPIMAGE_DIR)
	@rm -f $(APP_NAME)-$(APP_VERSION)-*.AppImage
	@rm -f $(APPIMAGE_TOOL)

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
	@echo "Installation complete"

# Remove installed binaries
uninstall:
	@echo "Uninstalling binaries from $(INSTALL_BIN_DIR)..."
	@sudo rm -f $(INSTALL_BIN_DIR)/IntuitionEngine
	@sudo rm -f $(INSTALL_BIN_DIR)/ie32asm
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

# Help target
help:
	@echo "Intuition Engine Build System"
	@echo ""
	@echo "Available targets:"
	@echo "  all            - Build both Intuition Engine and ie32asm (default)"
	@echo "  intuition-engine - Build only the Intuition Engine VM"
	@echo "  ie32asm        - Build only the IE32 assembler"
	@echo "  ie64asm        - Build only the IE64 assembler"
	@echo "  ie64dis        - Build only the IE64 disassembler"
	@echo "  appimage       - Create AppImage package"
	@echo "  install        - Install binaries to $(INSTALL_BIN_DIR)"
	@echo "  uninstall      - Remove installed binaries from $(INSTALL_BIN_DIR)"
	@echo "  clean          - Remove all build artifacts"
	@echo "  list           - List compiled binaries with sizes"
	@echo "  help           - Show this help message"
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
	@echo "Build flags:"
	@echo "  GO_FLAGS       = $(GO_FLAGS)"
	@echo "  NCORES        = $(NCORES)"
	@echo "  NICE_LEVEL    = $(NICE_LEVEL)"
	@echo ""
	@echo "Installation paths:"
	@echo "  PREFIX        = $(PREFIX)"
	@echo "  INSTALL_BIN_DIR = $(INSTALL_BIN_DIR)"