# Repository Guidelines

## Project Structure & Module Organization
- Root Go module (see `go.mod`) with the main entry at `main.go` and most subsystems split into focused files (CPU/audio/video/sound chips, etc.).
- `assembler/` holds the IE32 assembler, sample assembly, and CPU-specific demos.
- `tools/` contains auxiliary builders like the IE65 data generator.
- `testdata/` stores fixtures used by tests.
- `bin/` is the build output directory (created by Makefile targets).
- Asset/data files live at the repo root (e.g., `*.sid`, `*.ay`, `*.sap`, `*.png`, `*.bin`).

## Build, Test, and Development Commands
- `make all` builds the VM and IE32 assembler into `bin/`.
- `make intuition-engine` builds only the main VM binary.
- `make ie32asm` builds the IE32 assembler (`assembler/ie32asm.go`).
- `make gen-65-data` builds `tools/gen_65_data` into `bin/`.
- `go test ./...` runs the full Go test suite.
- Demo builds: `make robocop-65`, `make robocop-32`, `make robocop-68k`, `make robocop-z80` (require external toolchains as noted in `Makefile`).

## Coding Style & Naming Conventions
- Go formatting uses `gofmt` (tabs for indentation, standard Go layout).
- File names are lower_snake_case for multi-word components (e.g., `cpu_z80_runner.go`).
- Tests follow Go conventions with `*_test.go` and `*_integration_test.go` for integration coverage.

## Testing Guidelines
- Tests use Go’s standard `testing` package.
- Prefer adding unit tests alongside the component file; integration tests use `*_integration_test.go` naming.
- Run targeted tests during development (e.g., `go test ./... -run TestZ80`), then full suite before PRs.

## Commit & Pull Request Guidelines
- Commit messages are descriptive and sentence-style, often noting phases or specific file updates (e.g., `Phase 5: ...`, `Updated 5 ASM include files ...`).
- PRs should include: a short summary, key files touched, and test commands run. Include screenshots or media when changes affect visual output or demos.

## Notes on External Tools
- Some Makefile targets rely on `sstrip`, `upx`, `cc65`, `vasm`, or ImageMagick (`convert`). Confirm local availability before running demo builds.
