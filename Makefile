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

# Main targets
.PHONY: all clean list install uninstall

# Default target builds everything
all: setup intuition-engine ie32asm
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

# Clean build artifacts
clean:
	@echo "Cleaning build artifacts..."
	@rm -rf $(BIN_DIR)
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
	@echo "Installation complete"

# Remove installed binaries
uninstall:
	@echo "Uninstalling binaries from $(INSTALL_BIN_DIR)..."
	@sudo rm -f $(INSTALL_BIN_DIR)/IntuitionEngine
	@sudo rm -f $(INSTALL_BIN_DIR)/ie32asm
	@echo "Uninstallation complete"

# Help target
help:
	@echo "Intuition Engine Build System"
	@echo ""
	@echo "Available targets:"
	@echo "  all            - Build both Intuition Engine and ie32asm (default)"
	@echo "  intuition-engine - Build only the Intuition Engine VM"
	@echo "  ie32asm        - Build only the IE32 assembler"
	@echo "  install        - Install binaries to $(INSTALL_BIN_DIR)"
	@echo "  uninstall      - Remove installed binaries from $(INSTALL_BIN_DIR)"
	@echo "  clean          - Remove all build artifacts"
	@echo "  list           - List compiled binaries with sizes"
	@echo "  help           - Show this help message"
	@echo ""
	@echo "Build flags:"
	@echo "  GO_FLAGS       = $(GO_FLAGS)"
	@echo "  NCORES        = $(NCORES)"
	@echo "  NICE_LEVEL    = $(NICE_LEVEL)"
	@echo ""
	@echo "Installation paths:"
	@echo "  PREFIX        = $(PREFIX)"
	@echo "  INSTALL_BIN_DIR = $(INSTALL_BIN_DIR)"