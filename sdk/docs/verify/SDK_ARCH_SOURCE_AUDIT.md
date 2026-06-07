# SDK Architecture Source Audit

| Surface | Kind | Name | Executable evidence |
|---------|------|------|---------------------|
| Architecture | architecture claim | `Bare .ie68 uses the active-visible RAM ceiling; EmuTOS and AROS M68K loader modes use profile bounds.` | `boot_guest_ram.go` `resolveModeCaps`/`resolveActiveVisibleCeiling` cases for `modeM68KBare`, `modeEmuTOS`, and `modeAros` |
| Architecture | architecture claim | `Darwin RAM sizing uses a page-aligned conservative half of hw.memsize as the detected base before applying the per-platform reserve.` | `memory_sizing_usable_darwin.go` `unix.SysctlUint64("hw.memsize")`, `pageAlignDown(total / 2)`, and `memory_sizing.go` `ReserveFor` |
| Architecture | architecture claim | `EXEC_CTRL operation values: 1=Execute, 2=EmuTOS, 3=AROS, 4=IntuitionOS IExec, 5=Hard reset` | `program_executor_constants.go` `EXEC_OP_*`, `program_executor.go` `HandleWrite` dispatch, `program_executor_test.go` operation-value pins |
| Architecture | architecture claim | `Each ring has 16 descriptor slots but uses one slot to distinguish full from empty, so it can hold 15 queued requests at once.` | `coprocessor_constants.go` ring constants and coprocessor queue implementation |
| Architecture | architecture claim | `Mutable devices join the snapshot contract through MachineMonitor.RegisterSnapshotDevice.` | `debug_monitor.go` `RegisterSnapshotDevice`, `main.go` registrations |
| Architecture | architecture claim | `Video compositor default scale mode is stretch-fill; F11 toggles non-16:9 sources to aspect-fit.` | `video_compositor.go` `NewVideoCompositor`/`ToggleScaleModeIfNonNative`, `video_compositor_test.go` default-scale regression |
| Architecture | architecture claim | `mem.* helpers are raw 32-bit bus helpers, not an above-4GiB IE64 RAM or CPU-virtual-address API.` | `script_engine.go` mem helpers cast addresses to `uint32` |
| Architecture | cpu bridge row | `$A0-$AD \| VGA \| 0xF1000 \| Direct register map (MODE, STATUS, CTRL, SEQ, CRTC, GC, DAC, DAC read index, DAC mask, VRAM bank)` | `vga_constants.go` `Z80_VGA_PORT_*`, `cpu_z80_runner.go` `Z80BusAdapter.In`/`Out` |
| Architecture | cpu bridge row | `$D620-$D632 \| TED Video \| 0xF0F20+offset x4 \| Stride-4 register mapping including raster compare registers` | `ted_video_constants.go` `C6502_TED_V_*`, `cpu_six5go2.go` `readTEDPage`/`writeTEDPage` |
| Architecture | cpu bridge row | `$D700-$D70D \| VGA \| 0xF1000 \| Direct handler call plus DAC read index, DAC mask, and VRAM bank` | `vga_constants.go` `C6502_VGA_*`, `cpu_six5go2.go` `Bus6502Adapter.Read`/`Write` |
| Architecture | cpu bridge row | `$E4/$E5 \| SN76489 \| 0xF0C30/0xF0C31 \| Data write / last-written read and ready-status read` | `sn76489_constants.go` `Z80_SN_PORT_*`, `cpu_z80_runner.go` `Z80BusAdapter.In`/`Out` |
| Architecture | cpu bridge row | `$F2/$F3 \| TED \| 0xF0F00 / 0xF0F20-0xF0F6B \| Register select / data (audio indices $00-$05, video indices $20-$32 x4 stride)` | `ted_constants.go` `TED_REG_COUNT`, `ted_video_constants.go` `TED_V_INDEX_*`/`Z80_TED_V_INDEX_*`, `cpu_x86_runner.go` `X86_TED_V_INDEX_*` |
| Architecture | cpu bridge row | `$FE \| ULA \| 0xF2000 \| Border colour only (bits 0-2)` | `cpu_x86_runner.go` `X86BusAdapter.In`/`Out` ULA border-port case |
| Architecture | cpu bridge row | `$FE/$FD/$BE/$FA/$FB/$FC \| ULA \| 0xF2000-0xF2014 \| Border, control, status, VRAM address latch low/high, and paged VRAM data` | `ula_constants.go` `Z80_ULA_PORT_*`, `cpu_z80_runner.go` `Z80BusAdapter.In`/`Out` |
| Architecture | cpu bridge row | `x86 does not implement standard PC VGA I/O ports; VGA access is through the shared bus MMIO aperture and the direct $A0000-$AFFFF VRAM memory window.` | `cpu_x86_runner.go` `X86BusAdapter.In`/`Out` omit VGA port cases and `translateVRAM` handles the VRAM window |
| Architecture | jit matrix row | `Linux amd64 \| IE64, 6502, M68K, Z80, x86` | `jit_dispatch.go`, `jit_6502_dispatch.go`, `jit_m68k_dispatch.go`, `jit_z80_dispatch.go`, `jit_x86_dispatch.go` build tags |
| Architecture | jit matrix row | `Linux arm64 \| IE64` | `jit_dispatch.go`, `jit_z80_dispatch.go` runtime amd64 guard, non-IE64 stubs |
| Architecture | jit matrix row | `Windows amd64 \| IE64, 6502, M68K, Z80, x86` | `jit_dispatch.go`, `jit_6502_dispatch.go`, `jit_m68k_dispatch.go`, `jit_z80_dispatch.go`, `jit_x86_dispatch.go` build tags |
| Architecture | jit matrix row | `Windows arm64 \| IE64` | `jit_dispatch.go` arm64 windows tag plus non-IE64 stubs |
| Architecture | jit matrix row | `macOS amd64 \| IE64, 6502, M68K, Z80, x86` | `jit_dispatch.go`, amd64 per-core dispatch files |
| Architecture | jit matrix row | `macOS arm64 \| IE64` | `jit_dispatch.go` arm64 darwin tag plus non-IE64 stubs |
| Architecture | memory map row | `0x00000-0x9EFFF` | `machine_bus.go` `VECTOR_TABLE`/`PROG_START`/`STACK_START`/`IO_REGION_START` low-RAM boundary constants |
| Architecture | memory map row | `0x100000-0x5FFFFF` | `video_chip.go` `VRAM_START`/`VRAM_SIZE`, `main.go` `MapIO` |
| Architecture | memory map row | `0x1E00000-0x5DFFFFF` | `main.go` AROS profile video-memory allocation convention |
| Architecture | memory map row | `0x200000-0x27FFFF` | `coprocessor_constants.go` IE32 worker-memory constants |
| Architecture | memory map row | `0x280000-0x2FFFFF` | `coprocessor_constants.go` M68K worker-memory constants |
| Architecture | memory map row | `0x300000-0x30FFFF` | `coprocessor_constants.go` 6502 worker-memory constants |
| Architecture | memory map row | `0x310000-0x31FFFF` | `coprocessor_constants.go` Z80 worker-memory constants |
| Architecture | memory map row | `0x320000-0x39FFFF` | `coprocessor_constants.go` x86 worker-memory constants |
| Architecture | memory map row | `0x3A0000-0x41FFFF` | `coprocessor_constants.go` IE64 worker-memory constants |
| Architecture | memory map row | `0x790000-0x7917FF` | `coprocessor_constants.go` `MAILBOX_BASE`/`MAILBOX_END` |
| Architecture | memory map row | `0x800000-0x1DFFFFF` | `main.go` AROS profile fast-memory allocation convention |
| Architecture | memory map row | `0x800000-0x80FFFF` | `media_loader_constants.go` `MEDIA_STAGING_BASE`/`MEDIA_STAGING_END` |
| Architecture | memory map row | `0x9F000-0x9FFFF` | `cpu_ie32.go` and `cpu_ie64.go` reset stack seed convention |
| Architecture | memory map row | `0xA0000-0xAFFFF` | `registers.go` `VGA_VRAM_BASE`/`VGA_VRAM_END`, `main.go` `MapIO` |
| Architecture | memory map row | `0xB8000-0xBFFFF` | `registers.go` `VGA_TEXT_BASE`/`VGA_TEXT_END`, `main.go` `MapIO` |
| Architecture | memory map row | `0xD0000-0xDFFFF` | `voodoo_constants.go` texture-memory constants, `main.go` `MapIO` |
| Architecture | memory map row | `0xF0000-0xF049B` | `video_chip.go` `VIDEO_CTRL`/`VIDEO_REG_END`, `main.go` `MapIO` |
| Architecture | memory map row | `0xF0700-0xF07FF` | `registers.go` `TERMINAL_REGION_BASE`/`TERMINAL_REGION_END`, `main.go` `MapIO` |
| Architecture | memory map row | `0xF0800-0xF0B7F` | `audio_chip.go` `AUDIO_CTRL`/`AUDIO_REG_END`, `main.go` `MapIO` |
| Architecture | memory map row | `0xF0B80-0xF0B91` | `ahx_constants.go` `AHX_BASE`/`AHX_SUBSONG`, `main.go` `MapIO` |
| Architecture | memory map row | `0xF0BA0-0xF0BBF` | `midi_constants.go` `MIDI_PLAY_PTR`/`MIDI_END`, `main.go` `MapIO` |
| Architecture | memory map row | `0xF0BC0-0xF0BD7` | `mod_constants.go` `MOD_PLAY_PTR`/`MOD_END`, `main.go` `MapIO` |
| Architecture | memory map row | `0xF0BD8-0xF0BF3` | `wav_constants.go` `WAV_PLAY_PTR`/`WAV_END`, `main.go` `MapIO` |
| Architecture | memory map row | `0xF0C00-0xF0C0F` | `psg_constants.go` `PSG_BASE`/`PSG_END`, `main.go` `MapIO` |
| Architecture | memory map row | `0xF0C10-0xF0C1F` | `psg_constants.go` `PSG_PLAY_PTR`/`PSG_PLAY_STATUS`, `main.go` `MapIO` |
| Architecture | memory map row | `0xF0C20` | `psg_constants.go` `PSG_PLUS_CTRL`, `main.go` `MapIO` |
| Architecture | memory map row | `0xF0C30-0xF0C3F` | `sn76489_constants.go` `SN_BASE`/`SN_END`, `main.go` `MapIO` |
| Architecture | memory map row | `0xF0C40-0xF0CFF` | `audio_chip.go` SID2 FLEX constants, `main.go` `MapIO` |
| Architecture | memory map row | `0xF0D00-0xF0D0A` | `pokey_constants.go` `POKEY_BASE`/`POKEY_END`, `main.go` `MapIO` |
| Architecture | memory map row | `0xF0D10-0xF0D20` | `pokey_constants.go` SAP player constants, `main.go` `MapIO` |
| Architecture | memory map row | `0xF0D40-0xF0DFF` | `audio_chip.go` SID3 FLEX constants, `main.go` `MapIO` |
| Architecture | memory map row | `0xF0E00-0xF0E1C` | `sid_constants.go` `SID_BASE`/`SID_END`, `main.go` `MapIO` |
| Architecture | memory map row | `0xF0E20-0xF0E2D` | `sid_constants.go` SID player constants, `main.go` `MapIO` |
| Architecture | memory map row | `0xF0E30-0xF0E4C` | `sid_constants.go` `SID2_BASE`/`SID2_END`, `main.go` `MapIO` |
| Architecture | memory map row | `0xF0E50-0xF0E6C` | `sid_constants.go` `SID3_BASE`/`SID3_END`, `main.go` `MapIO` |
| Architecture | memory map row | `0xF0E80-0xF0EFF` | `sfx_constants.go` SFX constants, `main.go` `MapIO` |
| Architecture | memory map row | `0xF0F00-0xF0F05` | `ted_constants.go` `TED_BASE`/`TED_END`, `main.go` `MapIO` |
| Architecture | memory map row | `0xF0F10-0xF0F1F` | `ted_constants.go` TED player constants, `main.go` `MapIO` |
| Architecture | memory map row | `0xF0F20-0xF0F6B` | `ted_video_constants.go` `TED_VIDEO_BASE`/`TED_VIDEO_END`, `main.go` `MapIO` |
| Architecture | memory map row | `0xF1000-0xF13FF` | `vga_constants.go` `VGA_BASE`/`VGA_REG_END`, `main.go` `MapIO` |
| Architecture | memory map row | `0xF1400-0xF140F` | `registers.go` host-helper constants, `main.go` host helper registration |
| Architecture | memory map row | `0xF2000-0xF2017` | `ula_constants.go` `ULA_BASE`/`ULA_REG_END`, `main.go` `MapIO` |
| Architecture | memory map row | `0xF2100-0xF213F` | `antic_constants.go` `ANTIC_BASE`/`ANTIC_END`, `main.go` `MapIO` |
| Architecture | memory map row | `0xF2140-0xF21FB` | `antic_constants.go` `GTIA_BASE`/`GTIA_END`, `main.go` `MapIO` |
| Architecture | memory map row | `0xF2200-0xF221F` | `file_io_constants.go` `FILE_IO_BASE`/`FILE_IO_END`, `main.go` `MapIO` |
| Architecture | memory map row | `0xF2220-0xF225F` | `aros_dos_constants.go` `AROS_DOS_BASE`/`AROS_DOS_END`, `main.go` `MapIO` |
| Architecture | memory map row | `0xF2260-0xF22AF` | `aros_audio_constants.go` `AROS_AUD_REGION_BASE`/`AROS_AUD_REGION_END`, `main.go` `MapIO` |
| Architecture | memory map row | `0xF22B0-0xF22B7` | `file_io_constants.go` `FILE_DATA_PTR64`/`FILE_DATA_PTR64_END`, `main.go` `MapIO64` |
| Architecture | memory map row | `0xF2300-0xF231F` | `media_loader_constants.go` `MEDIA_LOADER_BASE`/`MEDIA_LOADER_END`, `main.go` `MapIO` |
| Architecture | memory map row | `0xF2320-0xF233F` | `program_executor_constants.go` `EXEC_BASE`/`EXEC_END`, `main.go` `MapIO` |
| Architecture | memory map row | `0xF2340-0xF238F` | `coprocessor_constants.go` `COPROC_BASE`/`COPROC_END`, `main.go` `MapIO` |
| Architecture | memory map row | `0xF2390-0xF23AF` | `clipboard_bridge_constants.go` `CLIP_REGION_BASE`/`CLIP_REGION_END`, `main.go` `MapIO` |
| Architecture | memory map row | `0xF23B0-0xF23BF` | `coprocessor_constants.go` `COPROC_EXT_BASE`/`COPROC_EXT_END`, `main.go` `MapIO` |
| Architecture | memory map row | `0xF23C0-0xF23DF` | `registers.go` IRQ diagnostic constants; `aros_loader.go` `MapIRQDiagnostics`; `main.go` AROS call sites; `machine_lifecycle.go` AROS reset loader call site; `aros_audio_dma.go` `UnmapIO` teardown |
| Architecture | memory map row | `0xF23E0-0xF23FF` | `bootstrap_hostfs_constants.go` `BOOT_HOSTFS_BASE`/`BOOT_HOSTFS_END`, `main.go` `MapIO` |
| Architecture | memory map row | `0xF2400-0xF24FF` | `sysinfo_mmio.go` `RegisterSysInfoMMIOFromBus`, `main.go` registration |
| Architecture | memory map row | `0xF3000-0xF6FFF` | `ted_video_constants.go` `TED_V_VRAM_BASE`/`TED_V_VRAM_SIZE`, `main.go` `MapIO` |
| Architecture | memory map row | `0xF8000-0xF87FF` | `voodoo_constants.go` `VOODOO_BASE`/`VOODOO_END`, `main.go` `MapIO` |
| Architecture | memory map row | `0xF8140-0xF823F` | `voodoo_constants.go` fog-table constants |
| Architecture | memory map row | `0xFA000-0xFBAFF` | `ula_constants.go` ULA VRAM aperture constants, `main.go` `MapIO` |
| Architecture | memory map subrange | `0xF0058-0xF0074` | `video_chip.go` Mode7 register offsets |
| Architecture | memory map subrange | `0xF0820-0xF0830` | `audio_chip.go` global filter register constants |
| Architecture | memory map subrange | `0xF0900-0xF093F except 0xF0914 and 0xF0918` | `audio_chip.go` square legacy register constants and sweep dispatch exceptions |
| Architecture | memory map subrange | `0xF0940-0xF097F plus 0xF0914` | `audio_chip.go` triangle legacy register constants and `TRI_SWEEP` |
| Architecture | memory map subrange | `0xF0980-0xF09BF plus 0xF0918` | `audio_chip.go` sine legacy register constants and `SINE_SWEEP` |
| Architecture | memory map subrange | `0xF09C0-0xF09FF` | `audio_chip.go` noise legacy register constants |
| Architecture | memory map subrange | `0xF0A00-0xF0A1C` | `audio_chip.go` sync/ring-mod source constants |
| Architecture | memory map subrange | `0xF0A20-0xF0A6F` | `audio_chip.go` sawtooth legacy register constants |
| Architecture | memory map subrange | `0xF0A80-0xF0B7F` | `audio_chip.go` primary FLEX channel constants |
| Architecture | memory map subrange | `0xF0C40-0xF0CFF` | `audio_chip.go` SID2 FLEX channel constants |
| Architecture | memory map subrange | `0xF0D40-0xF0DFF` | `audio_chip.go` SID3 FLEX channel constants |
| Architecture | public architecture category | `Audio Subsystem` | `ahx_constants.go`, `ahx_engine.go`, `ahx_parser.go`, `ahx_player.go`, `ahx_replayer.go`, `ahx_tables.go`, `ahx_waves.go`, `audio_backend_headless.go`, `audio_backend_null.go`, `audio_backend_oto.go`, `audio_chip.go`, `audio_interface.go`, `audio_lut.go`, `audio_master_normalizer.go`, `midi_constants.go`, `midi_engine.go`, `midi_parser.go`, `midi_player.go`, `mod_constants.go`, `mod_engine.go`, `mod_player.go`, `mod_replayer.go`, `pokey_constants.go`, `pokey_engine.go`, `pokey_player.go`, `sid_6502_player.go`, `sid_constants.go`, `sid_engine.go`, `sid_parser.go`, `sid_playback_bus_6502.go`, `sid_player.go`, `wav_constants.go`, `wav_engine.go`, `wav_parser.go`, `wav_player.go` |
| Architecture | public architecture category | `Bus and RAM` | `boot_guest_ram.go`, `machine_bus.go`, `machine_bus_alloc_darwin.go`, `machine_bus_alloc_linux.go`, `machine_bus_alloc_other.go`, `machine_bus_alloc_unix.go`, `machine_bus_phys.go`, `memory_sizing.go`, `memory_sizing_usable_darwin.go`, `memory_sizing_usable_linux.go`, `memory_sizing_usable_other.go`, `memory_sizing_usable_windows.go`, `profile_bounds.go` |
| Architecture | public architecture category | `CPU Subsystem` | `cpu_6502_interp_amd64.go`, `cpu_6502_interp_meta.go`, `cpu_6502_interp_meta_generated.go`, `cpu_6502_interp_stub.go`, `cpu_6502_interp_trace.go`, `cpu_6502_intsink.go`, `cpu_6502_opcode_table_gen.go`, `cpu_6502_runner.go`, `cpu_ie32.go`, `cpu_ie32_intsink.go`, `cpu_ie64.go`, `cpu_ie64_intsink.go`, `cpu_ie64_opcodes_gen.go`, `cpu_m68k.go`, `cpu_m68k_intsink.go`, `cpu_m68k_runner.go`, `cpu_six5go2.go`, `cpu_six5go2_fast.go`, `cpu_x86.go`, `cpu_x86_grp.go`, `cpu_x86_intsink.go`, `cpu_x86_ops.go`, `cpu_x86_poll_match_jit.go`, `cpu_x86_poll_match_stub.go`, `cpu_x86_runner.go`, `cpu_z80.go`, `cpu_z80_intsink.go`, `cpu_z80_runner.go`, `debug_cpu_6502.go`, `debug_cpu_ie32.go`, `debug_cpu_ie64.go`, `debug_cpu_m68k.go`, `debug_cpu_x86.go`, `debug_cpu_z80.go` |
| Architecture | public architecture category | `Debug monitor` | `debug_access.go`, `debug_access_resume.go`, `debug_adapter_helpers.go`, `debug_asm.go`, `debug_asm_write.go`, `debug_backtrace.go`, `debug_bp_expr.go`, `debug_breakin.go`, `debug_commands.go`, `debug_conditions.go`, `debug_cpu_6502.go`, `debug_cpu_ie32.go`, `debug_cpu_ie64.go`, `debug_cpu_m68k.go`, `debug_cpu_x86.go`, `debug_cpu_z80.go`, `debug_device_snapshot.go`, `debug_device_snapshot_extra.go`, `debug_device_snapshot_host.go`, `debug_disasm_6502.go`, `debug_disasm_ie32.go`, `debug_disasm_ie64.go`, `debug_disasm_ie64_opcodes_gen.go`, `debug_disasm_m68k.go`, `debug_disasm_x86.go`, `debug_disasm_z80.go`, `debug_dwarf.go`, `debug_event_sink.go`, `debug_faults.go`, `debug_fmt.go`, `debug_hotkey.go`, `debug_interface.go`, `debug_ioview.go`, `debug_ioview_read.go`, `debug_monitor.go`, `debug_overlay.go`, `debug_overlay_headless.go`, `debug_rc.go`, `debug_snapshot.go`, `debug_symbols.go`, `debug_symbols_sidecar.go`, `debug_symbols_vice.go`, `debug_trace_ring.go`, `debug_watchpoints.go` |
| Architecture | public architecture category | `File I/O` | `file_io.go`, `file_io_constants.go`, `gemdos_intercept.go`, `gemdos_intercept_constants.go`, `host_helper_console.go`, `host_helper_parser.go`, `host_helper_picker.go`, `host_helper_status.go`, `media_loader.go`, `media_loader_constants.go` |
| Architecture | public architecture category | `JIT` | `jit_6502_abi.go`, `jit_6502_common.go`, `jit_6502_dispatch.go`, `jit_6502_dispatch_stub.go`, `jit_6502_emit_amd64.go`, `jit_6502_exec.go`, `jit_6502_flags_liveness.go`, `jit_6502_fusion_match.go`, `jit_6502_turbo.go`, `jit_6502_turbo_fast.go`, `jit_abi_common.go`, `jit_amd64_registers_stub.go`, `jit_call.go`, `jit_chain_ordering.go`, `jit_common.go`, `jit_common_amd64.go`, `jit_common_other.go`, `jit_dispatch.go`, `jit_dispatch_stub.go`, `jit_emit_amd64.go`, `jit_emit_arm64.go`, `jit_exec.go`, `jit_exec_protect_darwin_arm64.go`, `jit_exec_protect_stub.go`, `jit_fastpath_backends.go`, `jit_fastpath_bitmaps.go`, `jit_flags_common.go`, `jit_helper_dispatch.go`, `jit_icache_amd64.go`, `jit_icache_amd64_darwin.go`, `jit_icache_amd64_windows.go`, `jit_icache_arm64.go`, `jit_icache_arm64_darwin.go`, `jit_icache_arm64_windows.go`, `jit_ie64_abi.go`, `jit_ie64_bench_turbo_amd64.go`, `jit_ie64_bench_turbo_stub.go`, `jit_ie64_flags_liveness.go`, `jit_ie64_turbo.go`, `jit_ie64_turbo_stub.go`, `jit_m68k_abi.go`, `jit_m68k_ccr_liveness.go`, `jit_m68k_common.go`, `jit_m68k_dispatch.go`, `jit_m68k_dispatch_stub.go`, `jit_m68k_emit_amd64.go`, `jit_m68k_exec.go`, `jit_m68k_turbo.go`, `jit_mmap.go`, `jit_mmap_darwin_amd64.go`, `jit_mmap_darwin_arm64.go`, `jit_mmap_stub.go`, `jit_mmap_windows.go`, `jit_mmio_poll_backends.go`, `jit_mmio_poll_common.go`, `jit_mmio_poll_exec_amd64.go`, `jit_mmio_poll_exec_arm64_stub.go`, `jit_mmio_poll_wiring.go`, `jit_region_backends.go`, `jit_region_common.go`, `jit_syscalls_darwin.go`, `jit_tier_backends.go`, `jit_tier_common.go`, `jit_x86_abi.go`, `jit_x86_common.go`, `jit_x86_cpuid.go`, `jit_x86_cpuid_stub.go`, `jit_x86_dispatch.go`, `jit_x86_dispatch_stub.go`, `jit_x86_eflags_liveness.go`, `jit_x86_emit_amd64.go`, `jit_x86_exec.go`, `jit_x86_terminator_stub.go`, `jit_x86_tier.go`, `jit_x86_turbo.go`, `jit_z80_abi.go`, `jit_z80_common.go`, `jit_z80_dispatch.go`, `jit_z80_dispatch_stub.go`, `jit_z80_emit_amd64.go`, `jit_z80_emit_arm64.go`, `jit_z80_exec.go`, `jit_z80_flags_liveness.go`, `jit_z80_turbo.go`, `jit_z80_turbo_amd64.go`, `jit_z80_turbo_native_stub.go`, `jit_z80_turbo_type_stub.go`, `jit_z80_unroll.go` |
| Architecture | public architecture category | `Lua Scripting` | `script_engine.go` |
| Architecture | public architecture category | `Snapshot` | `debug_snapshot.go` |
| Architecture | public architecture category | `Video Subsystem` | `antic_constants.go`, `antic_dlist.go`, `antic_modes.go`, `antic_pmg.go`, `ted_video_constants.go`, `ula_constants.go`, `ula_irq_adapter.go`, `vga_constants.go`, `video_antic.go`, `video_backend_ebiten.go`, `video_backend_headless.go`, `video_chip.go`, `video_compositor.go`, `video_cursor_policy.go`, `video_interface.go`, `video_lifecycle.go`, `video_recorder.go`, `video_screen_buffer.go`, `video_ted.go`, `video_terminal.go`, `video_terminal_clipboard.go`, `video_terminal_clipboard_headless.go`, `video_ula.go`, `video_vga.go`, `video_voodoo.go`, `voodoo_constants.go`, `voodoo_depth.go`, `voodoo_novulkan.go`, `voodoo_shaders.go`, `voodoo_software.go`, `voodoo_software_wrapper.go`, `voodoo_vulkan.go`, `voodoo_vulkan_headless.go` |
