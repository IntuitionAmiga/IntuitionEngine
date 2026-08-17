# Windows x64 Host SDK TDD Plan

## Summary

Add `intuition-engine-host-sdk-windows-amd64.zip` alongside the existing Linux
SDKs.

Build it on Linux using MinGW-w64, package Windows `.exe` tools without a
MinGW runtime DLL dependency, validate PE output, and inject the ZIP archive
plus checksum into the shared IESHARE payload used by x64 and Raspberry Pi
images.

Preserve the existing Linux archive names, layouts and behaviour.

## Implementation changes

- Parameterise the existing Host SDK distributor for host platform, executable
  suffix and package format. Keep `scripts/dist-host-sdk-windows-amd64.sh` as a
  thin wrapper that supplies the Windows settings. Do not duplicate the Linux
  distributor:
  - `CGO_ENABLED=0`, `GOOS=windows`, `GOARCH=amd64` for Go tools.
  - `x86_64-w64-mingw32-gcc` for QBE and cproc without a MinGW runtime DLL
    dependency.
  - A separate temporary Windows build directory.
  - Package host executables with `.exe` suffixes.
  - Use the package name `intuition-engine-host-sdk-windows-amd64.zip`.
  - Put the same layout under the ZIP root
    `intuition-engine-host-sdk-windows-amd64/`:
    `bin/`, `examples/`, `include/`, `lib/` and `share/`.

- Require only the MinGW-w64 cross compiler as an additional Windows build
  prerequisite. The existing Go, make, Python, file, zip, unzip and pinned
  source checkout requirements remain in force. Fail clearly when MinGW-w64 is
  unavailable.

- Extend installed-layout validation with an explicit Windows contract requiring
  the same SDK tools, headers, libraries, examples, documentation and licences
  as Linux, with `.exe` suffixes on every Windows host executable. Keep the
  existing Linux executable-name contract unchanged.

- Validate Windows binaries as PE/COFF x86-64 files with no MinGW runtime DLL
  dependency. Use the matching MinGW `objdump -p`, allow only the documented
  Windows system DLL allowlist (`ADVAPI32`, `API-MS-WIN-*`, `BCRYPT`, `COMBASE`,
  `CRYPT32`, `GDI32`, `KERNEL32`, `MSVCRT`, `NTDLL`, `OLE32`, `OLEAUT32`,
  `RPCRT4`, `SECHOST`, `SHELL32`, `USER32`, `VERSION`, `WINHTTP`, `WINMM` and
  `WS2_32`), and fail the build on every other DLL.

- Keep one explicit Host SDK package table in `build_x64_ie_img.sh`, containing
  each package's archive path, checksum path and extraction command. Use it for
  payload input checks, staging, size accounting and extracted validation. Do
  not derive formats or paths from package names:

  | Package | Archive | Checksum | Extraction |
  | --- | --- | --- | --- |
  | Linux x64 | `intuition-engine-host-sdk-linux-amd64.tar.xz` | `.sha256` beside archive | `tar` |
  | Linux ARM64 | `intuition-engine-host-sdk-linux-arm64.tar.xz` | `.sha256` beside archive | `tar` |
  | Windows x64 | `intuition-engine-host-sdk-windows-amd64.zip` | `.sha256` beside archive | `unzip` |

  Each package and checksum must be:
  - required before staging;
  - checksum-verified;
  - extracted with its declared tool and layout-verified;
  - included in the IESHARE size budget;
  - copied to `SDK/Toolchains/`.

  Replace the existing `HOST_SDK_NAMES` and `.tar.xz` assumptions everywhere in
  the shared payload path. Make keeps explicit prerequisites for
  `dist-host-sdk-linux-amd64`, `dist-host-sdk-linux-arm64` and
  `dist-host-sdk-windows-amd64`; the shell table owns only payload artefact
  handling.

- Add Make targets for building and testing the Windows SDK. Keep the existing
  Linux SDK distribution targets and package behaviour unchanged. Update shared
  live-image prerequisites so the payload depends on all three SDK distributions.

- Add Windows Host SDK download and checksum links to
  `./intuitionengine.com/index.html`, update `.gitignore`, copy the generated
  Windows package and checksum into `./intuitionengine.com/assets/`, and retain
  all Host SDK packages outside the browser filesystem manifest.

- Update only affected documentation and source comments. Use British English,
  avoid em dashes, and describe the package as Windows x64 or Windows x86-64
  consistently.

## TDD sequence

1. Add failing contracts covering:
   - Windows distribution and packaging targets.
   - `CGO_ENABLED=0`, `GOOS=windows`, `GOARCH=amd64`, MinGW-w64 and `.exe` naming.
   - PE x86-64 validation.
   - Platform-aware installed SDK layout checks.
   - Shared IESHARE requirements for the Windows package and checksum.
   - Combined package size accounting.
   - Website links, checksum retention and browser-manifest exclusion.
   - Unchanged Linux archive paths and contracts.

2. Implement the smallest distributor changes needed to make the focused
   contracts pass.

3. Add mandatory Windows build and packaging checks:
   - cross-compilation of every Windows `.exe`;
   - native Linux bootstrap tools for guest-library generation only;
   - PE/COFF x86-64 validation;
   - Windows system-DLL allowlist validation;
   - ZIP extraction and installed-layout validation;
   - create the ZIP twice and compare the results byte-for-byte.

## Acceptance tests

- Existing Linux x64 and Linux ARM64 Host SDK behaviour remains passing, with
  shared tests extended for Windows.
- Windows x64 cross-build produces valid PE/COFF executables.
- The Windows ZIP is created twice from the normalised staging tree and the
  two files are byte-for-byte identical.
- Windows ZIP extraction passes the complete installed-layout contract.
- All three Host SDK packages and checksums are present and verified in the staged
  IESHARE payload.
- x64, Pi 4, Pi 400 and Pi 5 payload checks continue to use the shared staging
  directory.
- `scripts/test-makefile.sh`, focused headless Go tests, `make check-docs`, and
  `git diff --check` pass.
- `./intuitionengine.com/index.html` contains download links for the Windows
  package and its SHA-256 checksum alongside the Linux SDK links, and the
  generated files in `./intuitionengine.com/assets/` match the validated output.
- `./intuitionengine.com/assets/intuition-engine-host-sdk-windows-amd64.zip` and
  its `.sha256` file are copied from the validated distribution output.
- No changes are committed.

## Assumptions

- MinGW-w64 is the only additional Windows cross-compilation prerequisite.
  Standard host tools already used by the repository, including `zip`, `unzip`,
  `sha256sum` and the matching MinGW `objdump`, are also required for packaging
  and validation. The package must not require separately installed MinGW
  runtime DLLs.
- The Windows SDK contains Windows host tools but the same IE64 guest headers,
  libraries, examples and documentation as the Linux SDKs.
- Windows binaries are cross-compiled and packaged but never executed by this
  Linux workflow. Temporary native Linux tools generate guest libraries and
  startup objects only.
- The Windows package is injected into IESHARE but remains excluded from the
  browser filesystem manifest.
