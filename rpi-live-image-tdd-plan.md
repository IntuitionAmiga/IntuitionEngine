# KISS Raspberry Pi Live Image Full TDD Plan

## Goal

Add live images for Raspberry Pi 4, Raspberry Pi 400 and Raspberry Pi 5. Each image must provide every user-facing feature of `x64-live`: automatic Intuition Engine launch, embedded systems, demos, AROS, AB3D2, IEDoom, manuals, the existing x64 HOST SDK, networking, audio, security controls and IESHARE.

Build all three images on openSUSE Tumbleweed x64. Follow the working IntuitionSubtractor design where appropriate: cross-compile the application, copy a known-good Raspberry Pi OS ARM64 golden image with its working Raspberry Pi firmware, PREEMPT_RT kernel, JACK installation and real-time audio configuration, then customise the copy with rootless libguestfs operations.

Use one builder and one payload implementation. Board targets supply only the binary, CPU tuning, optional PGO profile and output name. Do not create three copied image scripts.

## Supported Targets

| Board | Compiler tuning | Binary target | Image target | Acceptance |
|---|---|---|---|---|
| Raspberry Pi 4 | Cortex-A72 | `rpi-4-arm64` | `build-image-pi4` | QEMU smoke test |
| Raspberry Pi 400 | Cortex-A72 | `rpi-400-arm64` | `build-image-pi400` | Physical Pi 400 |
| Raspberry Pi 5 | Cortex-A76 | `rpi-5-arm64` | `build-image-pi5` | Physical Pi 5 |

Add `rpi-live-images` as the aggregate image target. Keep separate Pi 4 and Pi 400 binaries and filenames so they can gain independent PGO profiles without changing public interfaces.

## TDD Implementation

### 1. Define the observable contracts first

- Extend `scripts/test-makefile.sh` before adding implementation. Test the three binary targets, three image targets, three payload-check targets, aggregate target and `rpi4-live-qemu` target. Prove that a Pi target selects `../IntuitionSubtractor/sysroot-arm64` by default and that an explicit command-line `CROSS_SYSROOT` takes precedence.
- Test behaviour and required inputs rather than the exact internal prerequisite order.
- Add one focused shell test for the parameterised Pi image builder. Cover missing tools, missing or invalid golden image, wrong binary architecture, payload validation, immutable source image, output naming and safe failure.
- Add one payload parity test proving that the x64 and Raspberry Pi staging results contain the same guest images, systems, manuals, SDK sources, x64 HOST SDK archive and IESHARE layout.
- Add one sysroot contract proving that `validate-rpi-sysroot` accepts the existing `../IntuitionSubtractor/sysroot-arm64` only when it contains the ARM64 loader, JACK header, library and pkg-config file, and when an ARM64 JACK link probe's interpreter, libraries and versioned symbols are available in the golden root filesystem. Build the probe with the exact `CROSS_CC`, `--sysroot`, `PKG_CONFIG_LIBDIR` and `PKG_CONFIG_SYSROOT_DIR` used by the Pi recipe. Cover missing inputs, poisoned host pkg-config paths and an incompatible golden root filesystem.
- Use temporary directories and small command fixtures. Do not add large image fixtures or a general mocking framework.

### 2. Add the native JACK backend with full TDD

- Write failing audio-interface and JACK-backend tests before implementation. Cover backend construction, explicit selection, fallback, start, stop, idempotent close, server shutdown, sample conversion, mono routing, arbitrary JACK period sizes and underrun reporting.
- Add `AUDIO_BACKEND_JACK` to the existing `AudioOutput` interface selection without changing the interface used by Oto and null output.
- Implement JACK in its own `audio_backend_jack.go` file, following the existing one-backend-per-file layout. Use the `linux && cgo && jack` build constraint and the same `github.com/xthexder/go-jack` dependency proven by IntuitionSubtractor.
- Add `audio_backend_jack_unavailable.go` with the complementary `!linux || !cgo || !jack` constraint. It keeps other builds compiling and returns a clear error when JACK is explicitly requested. Do not add JACK dependencies to x64, headless, WebAssembly or other existing builds.
- Put a fixed-capacity, preallocated single-producer, single-consumer float32 ring between IE rendering and JACK. A dedicated rendering goroutine calls `SoundChip.ReadSamples()` and writes complete sample blocks into the ring. The JACK process callback only copies available samples into the JACK buffers.
- Size the ring in JACK periods and make the period count a named, tested constant. Start with three periods. During construction, publish two complete periods of zeroes as readable data and leave exactly one complete period free. This deliberate startup silence is not rendered audio and must be recorded separately from steady-state latency. Change the ring depth only from physical measurements.
- Define one producer block as exactly one current JACK period, initially 64 frames. The rendering goroutine checks that one complete period will fit before calling `SoundChip.ReadSamples()` with that period-sized slice. When space is unavailable it waits for a bounded fraction of the JACK period, checks the stop flag and polls the atomic read cursor again without advancing any IE audio state. It never overwrites unread samples, drops rendered samples or busy-spins on a full ring.
- After playback has started, fill any ring underrun remainder in both JACK output buffers with silence and increment an atomic counter. Before playback starts, return intentional silence without consuming the prefill. Reset JACK xrun and IE underrun acceptance counters when `Start()` enables playback so construction-time activity cannot contaminate results. The JACK callback never waits for the producer.
- The JACK process callback must not call `SoundChip.ReadSamples()`, allocate, take locks, log, access files or perform device management. Preallocate every callback buffer and ring slot during construction.
- Duplicate each mono IE sample into the left and right JACK output ports. Connect both when two physical playback ports exist and use the sole available port when only one exists.
- Test ring wraparound, ordering, underrun silence, full-ring producer pacing, mono duplication, one-port routing, two-port routing and clean renderer shutdown. Exercise the pure-Go ring consumer with requested chunks smaller than, equal to and larger than one period even though the real appliance callback is pinned to one period. In a deterministic ring test with the producer disabled or manually stepped, pin the initial read and write cursors and their values after each of the first three simulated JACK callbacks. Test the concurrent producer separately with ordering and race checks.
- Register JACK sample-rate, buffer-size, shutdown and xrun callbacks. Require the server to run at IE's fixed 44,100 Hz rate and pin the appliance period size at server startup. Reject any other sample rate or a period larger than the tested maximum before allocating the ring or activating the client. Treat any runtime period-size change as backend failure: publish failure atomically, make the process callback return silence and let the supervisor request controlled termination. Do not resize the ring or replace the backend while running. Publish bounded underrun and xrun diagnostics through atomics for reporting outside the real-time path.
- Connect the IE output port to the selected physical JACK playback ports after activation. Keep device and port selection configurable, with deterministic appliance defaults and useful errors.
- Move audio-output construction to the end of `NewSoundChip`, after every internal sound-chip buffer is initialised. During JACK construction, start or locate the server, open the client, validate 44,100 Hz, query and validate its period size, allocate the three-period ring, publish two zero periods, register ports and callbacks, activate the client and connect the selected playback ports in that order. Do not start the rendering goroutine or call `SoundChip.ReadSamples()` during construction because the external PSG, SID, POKEY, TED, AHX and other audio engines are attached later.
- On `SoundChip.Start()`, start the rendering goroutine and atomically enable JACK consumption. The callback consumes the initial zero periods while the producer begins filling the remaining ring with fully configured IE audio. Test that audio begins without a discontinuity after the deliberate startup silence.
- Close every acquired JACK resource on failure. Test failure after client creation, each port registration, callback registration and activation so no client, port, goroutine or device ownership leaks into fallback.
- Follow IntuitionSubtractor's proven ownership model with a small Linux JACK launcher file separate from `audio_backend_jack.go`. If no server exists, IE starts direct JACK2 with `jackd -R -P95 -dalsa`, the selected `-d` ALSA device, `-r44100 -p64 -n3`, waits until it accepts clients and retains the process handle. If a compatible external 44,100 Hz JACK server exists, connect to it without retaining ownership. Require its reported period size to match the supported configuration. Reject an incompatible server and never stop a server which IE did not start. Change the initial 64-frame, three-period configuration only after physical measurements and corresponding test updates.
- Make `newRuntimeSoundChip` the sole runtime backend selector. It reads `IE_AUDIO_BACKEND` with the exact values `oto`, `jack` and `null`, and rejects unknown values. Leave `NewSoundChip(backend)` as an explicit constructor for tests and callers which already select a backend. The Raspberry Pi appliance launch configuration sets `IE_AUDIO_BACKEND=jack`; an unset value retains Oto as the default.
- Test selection through `newRuntimeSoundChip`: unset and `oto` try Oto then null, `jack` tries JACK then Oto then null, and `null` constructs null directly. Test invalid values without constructing an output. When JACK is selected, start or connect to the server and construct the complete JACK backend. If owned server startup or JACK construction fails during initial startup, close every JACK resource, stop the owned `jackd` process group, wait for the ALSA device to be released and then try Oto. If Oto also fails, retain the existing null fallback. Never stop an external JACK server.
- Add focused race, callback-body allocation and lifecycle tests using a minimal internal wrapper around the required `go-jack` client operations. Tests use a small fake wrapper and do not require a running JACK server or reproduce the complete JACK API.
- Prove automatically that the IE callback body allocates nothing. Treat complete CGO callback timing, scheduling and xrun behaviour as JACK server integration evidence which must be measured on physical hardware.
- State the scheduling boundary accurately: the JACK callback is the real-time consumer, while `SoundChip.ReadSamples()` runs on an ordinary Go rendering goroutine. The bounded ring absorbs normal scheduler jitter. PREEMPT_RT does not make the Go renderer hard real-time, so physical latency and xrun results decide whether the initial three-period depth is sufficient.
- On stop or close, signal the rendering goroutine, wait for it to exit, then deactivate and close JACK resources and finally stop an owned server. Make close idempotent and terminal. A closed audio output is never restarted; a new `SoundChip` is required. Test shutdown while the producer is rendering, waiting for ring space, already stopped and never started.
- Test JACK ownership and cleanup when construction succeeds, construction fails after starting an owned server, an external server is used, IE receives SIGTERM, JACK shuts down unexpectedly, close is repeated and initial Oto fallback follows owned-server termination. Each test proves whether the server remains or terminates according to ownership.
- Change `SoundChip.Stop()` so it always closes an existing output, even when playback never started. Only the state transition from enabled to disabled remains conditional. Register one idempotent cleanup function for the active `SoundChip` immediately after runtime sound construction. Invoke it from `exitProfiled()`, signal shutdown and normal shutdown before process termination, because `exitProfiled()` calls `os.Exit` and cannot rely on deferred functions. Test JACK, Oto and null cleanup before and after `Start()`, plus JACK construction followed by exit before the first `SoundChip.Start()`.
- In the JACK shutdown callback, publish failure state atomically and make the process callback return silence. One small supervisor goroutine observes that state and requests termination through `exitProfiled()`. The registered process cleanup hook is the sole owner of resource release. Never clean up, launch fallback or wait for goroutines inside a JACK callback. Restrict Oto fallback to initial startup failures.
- Cross-compile the JACK backend against the ARM64 sysroot and run its pure-Go callback and selection tests under QEMU user mode. Run server integration and underrun acceptance on physical Pi 400 and Pi 5 hardware.

### 3. Reuse the existing ARM64 cross-build support

- Use the existing `CROSS_CC`, `CROSS_CXX`, `CROSS_SYSROOT`, isolated pkg-config settings and `build-linux-vm-binary` helper. Do not add a second compiler discovery or sysroot system.
- Capture any command-line `CROSS_SYSROOT` before the existing global default is assigned. Each Raspberry Pi target uses that captured value when present, otherwise uses `../IntuitionSubtractor/sysroot-arm64`. Do not use a later `CROSS_SYSROOT ?=` assignment, because the existing global assignment has already set it. Do not use the openSUSE system sysroot for Raspberry Pi release binaries.
- Add `validate-rpi-sysroot`, backed by one small read-only script. It verifies the selected sysroot's ARM64 loader, JACK headers, `libjack` and `jack.pc`, then builds one minimal ARM64 JACK link probe with the exact `CROSS_CC`, `--sysroot`, `PKG_CONFIG_LIBDIR` and `PKG_CONFIG_SYSROOT_DIR` used by the Pi recipe. It rejects a non-ARM64 probe and resolves the probe's interpreter, libraries and required versioned symbols against `rpi-ie-golden.img`. It never copies or modifies the sysroot, executes ARM64 code or installs packages on the host.
- Make every Raspberry Pi binary and QEMU user-mode target depend on `validate-rpi-sysroot`. Fail before an IE build when the selected sysroot is missing, incomplete or incompatible with the golden root filesystem.
- Extend `build-linux-vm-binary` with only the extra environment and Go arguments needed for CPU tuning, PGO and build tags. Preserve all existing callers by passing empty values where appropriate.
- Add one small shared Raspberry Pi binary recipe parameterised by board, CPU flags, PGO profile and output path.
- Build with `GOOS=linux`, `GOARCH=arm64`, embedded BASIC, EmuTOS and AROS, the normal Vulkan-capable Linux tags and the `jack` tag.
- Use Cortex-A72 C and C++ tuning with `GOARM64=v8.0` for Pi 4 and Pi 400. Use Cortex-A76 tuning with `GOARM64=v8.2` for Pi 5.
- Use `default.pgo.rpi400` for both Pi 4 and Pi 400 builds, and `default.pgo.rpi5` for Pi 5, only when the respective file exists and is non-empty. Otherwise build with `-pgo=off`.
- Require the selected ARM64 sysroot to contain JACK headers and libraries as well as the existing graphics, X11, Wayland, audio and Vulkan dependencies.
- Validate each result as an ARM64 ELF. Resolve its interpreter, `libjack` and every other shared library first against `CROSS_SYSROOT`, then prove that the same interpreter, libraries and required versioned symbols exist in the golden root filesystem. Test the exact board build variables rather than claiming that ELF metadata proves the complete processor instruction baseline.
- Run the existing bounded ARM64 JIT correctness tests with `qemu-aarch64 -L $(CROSS_SYSROOT)`, using the same selected sysroot as the release build. This proves executable correctness, not board performance.

### 4. Reuse payload behaviour without moving x64 outputs

- Extract only the existing `stage_ieshare_payload` and payload-validation operations that both builders need into one small shell module accepting a destination directory. Do not refactor unrelated x64 image construction or packaging.
- Keep x64 staging at `build/x64-live/work/ieshare-payload`. This preserves the existing `web-demos` input and browser build behaviour.
- Stage Raspberry Pi payloads beneath the selected Pi image work directory. Never make a Pi build consume stale x64 staging.
- Keep target-specific binary checks, boot configuration, HOST helper, partition handling and packaging in their existing target-specific builders.
- Reuse the existing architecture-neutral payload producer targets for SDK examples, embedded systems, AROSVision, AROS demos, AB3D2, IEDoom and PDFs. Do not introduce a second dependency graph for the same files.
- Keep `wasm` as an x64 release prerequisite only. Raspberry Pi images do not build or package the browser application.
- Continue placing the existing `dist-host-sdk-linux-amd64` archive and checksum on IESHARE. Do not create an ARM64 HOST SDK.
- Add `rpi4-live-payload-check`, `rpi400-live-payload-check` and `rpi5-live-payload-check`. Each target builds its board binary and invokes the parameterised image builder with `--check-payload`.

### 5. Establish one verified Raspberry Pi golden image

- Maintain the reviewed Raspberry Pi OS Trixie image at the ignored repository-root path `rpi-ie-golden.img`.
- Never modify `rpi-ie-golden.img` in place. Every image build starts with a fresh copy and leaves the source byte-identical after success or failure.
- Add a small tracked manifest with the golden image SHA-256, partition layout, operating-system release, ARM64 architecture, expected PREEMPT_RT kernel identity and required appliance runtime packages.
- Validate the checksum, DOS partitions, Raspberry Pi boot files, kernel files and operating-system identity before copying the image.
- Preserve the working Raspberry Pi firmware, bootloader, PREEMPT_RT kernel, JACK packages, real-time limits and audio-group configuration. Do not replace them during normal image construction.
- Validate that the golden image contains JACK, `jackd2`, `rtkit` and the packages required for the IE appliance session, including Cage, Xwayland, greetd, NetworkManager, fonts, input support, AppArmor, polkit and partition-growth tools.
- Validate golden-image JACK state only: `jackd2`, `libjack`, real-time limits, audio-group membership and required audio-device permissions. Pin the exact direct JACK2 launcher arguments in source tests rather than treating them as golden-image configuration.
- Configure the appliance launch environment with `IE_AUDIO_BACKEND=jack`. IE launches its owned `jackd` at 44,100 Hz with the tested direct JACK2 arguments and runtime-selected ALSA device when no compatible external server exists. Prevent competing sound servers from claiming the selected appliance audio device. Oto fallback starts only after failed JACK resources and any owned server have stopped and released that device.
- Before constructing Oto fallback, generate a minimal Raspberry Pi-only ALSA configuration in the IE runtime directory which points the default PCM to the same hardware selected by the JACK launcher, set `ALSA_CONFIG_PATH` for the Oto process, and validate the file before use. Give the confined IE profile access only to that runtime file and the selected device. Do not change normal x64 Oto routing or the system-wide ALSA configuration.
- Add a dedicated AppArmor child profile for `/usr/bin/jackd` and an explicit transition from the confined IE profile. Permit only the JACK configuration, runtime socket and shared-memory paths, selected audio devices, required ARM64 libraries and the capabilities needed for the reviewed real-time configuration. Do not run `jackd` unconfined.
- Generate the Raspberry Pi IE and JACK AppArmor profiles with ARM64 library paths. Do not copy x86-64 multiarch paths into the Pi images. Validate both the required ARM64 rules and the absence of x86-only library rules.
- Permit and test only the signals required for IE to terminate its confined owned `jackd` process group. Do not grant signal access to unrelated profiles or processes.
- Use the maintained Raspberry Pi OS golden image only when it already contains the complete Pi base: Raspberry Pi firmware, PREEMPT_RT kernel, JACK2 and required appliance packages. The golden image does not contain the current IE binary, IE services, board configuration or IESHARE payload. Otherwise run one short, idempotent native Raspberry Pi preparation script on a copy.
- The preparation script adds only missing graphical or appliance packages and removes transient package state.
- The native preparation script verifies that the PREEMPT_RT kernel, JACK installation, real-time configuration, Raspberry Pi firmware and bootloader remain unchanged, then shuts down cleanly.
- Copy the prepared image back as `rpi-ie-golden.img` and update its manifest. Native preparation is a maintenance operation, not part of normal image builds. It must never reinstall or replace the RT kernel, JACK or the Raspberry Pi boot stack.
- Do not support `--rebuild-golden`. Replacing the golden image and its manifest is an explicit maintenance operation.

### 6. Build all images with one parameterised builder

- Add one Raspberry Pi image builder accepting only a validated board identifier, ARM64 binary path and output path.
- Use rootless libguestfs and architecture-neutral filesystem tools. Do not require loop mounts, root-owned files, registered binfmt support or ARM64 package execution during image construction.
- Inject the same Cage and Xwayland session, unprivileged `ie` account, firewall, HOST-helper broker, disabled virtual-terminal switching and persistent state used by `x64-live`, with the Raspberry Pi JACK startup, AppArmor child profile and ALSA fallback configuration.
- Build `cmd/host-helper` as a pure-Go ARM64 executable for the image root filesystem and validate its ELF architecture. This is separate from the unchanged x64 HOST SDK on IESHARE.
- Replace only x86-specific GRUB, UEFI, boot and multiarch configuration with Raspberry Pi equivalents under `/boot/firmware`.
- Disable the Raspberry Pi OS setup wizard and enable the required IE services with offline files and systemd symlinks. Do not install packages, download files or require an automatic reboot at first boot.
- Copy the golden image to the selected output, enlarge it to the same 20 GiB release size as `x64-live`, and append DOS partition 3 in unused space without changing partitions 1 or 2.
- Format partition 3 as FAT32 `IESHARE` using rootless tools and populate it from the Raspberry Pi payload staging directory.
- Mount `LABEL=IESHARE` at `/var/ie/share`. Reuse the proven x64 first-boot growth behaviour, adapted only where Raspberry Pi device naming requires it. Never recreate or reformat an existing IESHARE filesystem.
- Support `--check-payload` and `--no-share`. Keep work, output and log directories overridable for tests and local builds.
- Produce the following board-specific files:
  - `build/rpi4-live/intuition-engine-rpi4.img`
  - `build/rpi4-live/intuition-engine-rpi4.img.sha256`
  - `build/rpi4-live/intuition-engine-rpi4.zip`
  - `build/rpi4-live/intuition-engine-rpi4.zip.sha256`
  - `build/rpi400-live/intuition-engine-rpi400.img`
  - `build/rpi400-live/intuition-engine-rpi400.img.sha256`
  - `build/rpi400-live/intuition-engine-rpi400.zip`
  - `build/rpi400-live/intuition-engine-rpi400.zip.sha256`
  - `build/rpi5-live/intuition-engine-rpi5.img`
  - `build/rpi5-live/intuition-engine-rpi5.img.sha256`
  - `build/rpi5-live/intuition-engine-rpi5.zip`
  - `build/rpi5-live/intuition-engine-rpi5.zip.sha256`
- Mirror the x64 ZIP layout. Include the board image, companion PDFs under `Docs/` and the complete reference-manual tree under `Docs/IEProgRefMan/`.

### 7. Add an optional Pi 4 QEMU smoke test

- Add `rpi4-live-qemu` using the generated Pi 4 image with `qemu-system-aarch64 -M raspi4b`.
- Detect whether the installed QEMU provides `raspi4b`. Skip with a clear message when it does not.
- Boot the image without modifying it. Use conservative emulated devices, serial logging and a finite timeout.
- Report unsupported QEMU configuration, failure to reach userspace, IE service failure and success as distinct outcomes. Retain the serial log for diagnosis.
- Treat reaching systemd and starting the IE service as useful smoke evidence. Permit the existing null fallback when QEMU provides neither usable JACK nor ALSA audio. Do not require emulated graphics, audio, networking or USB devices that QEMU does not implement reliably.
- Keep this target outside `build-image-pi4`, `rpi-live-images` and release acceptance. QEMU Pi 4 emulation does not prove Pi 400 or Pi 5 hardware support.

## Verification

### Red-to-green checks

- Add each failing contract before its implementation slice and make it pass before starting the next slice.
- Run `bash -n` on the x64 builder, Raspberry Pi builder, shared payload module and golden preparation script.
- Run the focused Makefile, payload and builder tests after every relevant change.
- Build all three ARM64 binaries and the ARM64 HOST helper with the selected Subtractor sysroot. Validate their ELF architecture, interpreter, dynamic dependencies and required versioned symbols against both that sysroot and the golden root filesystem, including `libjack` for IE. Prove that existing builds do not acquire a JACK dependency.
- Run the bounded ARM64 JIT tests and pure-Go JACK ring, callback and selection tests under QEMU user mode.
- Run each Raspberry Pi payload check, the existing x64 payload check and the existing browser build tests.
- Run `git diff --check` and scan changed prose and comments for British English, em dashes and en dashes.

### Offline image checks

- Build each image from a fresh golden-image copy and prove that the source golden image remains byte-identical.
- Inspect each partition table and prove that boot and root partitions are preserved and IESHARE occupies only appended space.
- Verify the firmware, PREEMPT_RT kernel files, JACK packages, 44,100 Hz `jackd` configuration and ordering, declared real-time limits, audio-group configuration, ARM64 executables, runtime packages, permissions, ARM64 AppArmor policies and transition, fstab, services and IESHARE growth ordering.
- Compare every Raspberry Pi payload inventory with the x64 payload inventory.
- Verify image and ZIP checksums, run `unzip -t`, and compare documentation archive entries with `x64-live`.
- Build twice and compare filesystem layout and payload inventories. Do not require byte-identical filesystem images where timestamps or filesystem identifiers legitimately differ.

### Boot acceptance

- Run the optional Pi 4 QEMU smoke test and retain its serial log when QEMU can boot the image.
- Flash and boot the Pi 400 image on the physical Pi 400.
- Flash and boot the Pi 5 image on the physical Pi 5.
- On each physical board, verify that `uname -a` reports the expected PREEMPT_RT kernel and that JACK tools, `jackd2`, `rtkit`, real-time limits and audio-group membership are present.
- From inside the actual greetd appliance session, verify the effective real-time priority limit, locked-memory limit and audio-group membership. Inspect the running IE-owned `jackd` command line, owner, groups, selected ALSA device, locked memory, scheduling class and priority. If a separate manual `jackd -R` start test is needed, first stop IE and its owned server so the ALSA device is free.
- Load both generated AppArmor profiles with `apparmor_parser`, confirm they are in enforce mode, launch owned `jackd` through the intended child transition and inspect either the kernel journal or audit log for denied operations. Prove that IE can signal and terminate only its owned confined server.
- Force one controlled IE termination and prove that greetd recreates the IE session, Cage and Xwayland. Prove that the previous owned `jackd` has terminated and the replacement IE owns exactly one new `jackd` process.
- Force repeated JACK failures and prove that no `jackd`, IE, Cage or Xwayland processes overlap across restarts. Make the appliance session wrapper run Cage without replacing the wrapper process, capture its exit status, wait for a fixed bounded delay after abnormal exit, then exit so greetd restarts the session. This prevents a persistent fault from creating an uncontrolled rapid restart loop.
- Prove that IE selected the JACK backend, connected to the intended playback ports and produced low-latency audio without xruns during representative demonstrations. Record the JACK period count, JACK period size, IE ring depth, deliberate startup-silence prefill, hardware buffering and measured steady-state end-to-end latency. Calculate configured steady-state latency from the JACK, IE and hardware buffering chain without counting the one-time zero prefill. Verify fallback separately without accepting fallback as the normal physical-board result.
- Stop IE and its owned `jackd`, then exercise the initial-startup Oto fallback and prove that it opens the selected Raspberry Pi hardware through ALSA. Prove that `ALSA_CONFIG_PATH` names the confined runtime configuration and that this configuration selects the same hardware as the JACK launcher.
- Verify automatic launch, HDMI display, keyboard, controller, networking, HOST actions, restart behaviour and IESHARE growth on media larger than 20 GiB.
- Run representative IE64, M68K, 6502, Z80 and x86 JIT demonstrations.
- Confirm that IESHARE contains the unchanged x64 HOST SDK archive and matching checksum.
- Record board model, RAM, firmware and display mode for physical tests. Document Pi 4 as QEMU-tested rather than hardware-verified until it is tested on a physical Pi 4.

## Constraints

- The supported build host is openSUSE Tumbleweed x64.
- Supported boards are Raspberry Pi 4, Raspberry Pi 400 and Raspberry Pi 5 only.
- No agreed `x64-live` feature may be removed to simplify implementation.
- QEMU is optional correctness evidence and never performance or hardware evidence.
- Existing unrelated worktree files remain untouched.
- All new documentation and source comments use British English, contain no em dashes or en dashes, and avoid formulaic generated wording.
