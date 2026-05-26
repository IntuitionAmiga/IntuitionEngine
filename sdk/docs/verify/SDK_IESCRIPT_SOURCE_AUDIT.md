# SDK IEScript Source Audit

| Surface | Kind | Name | Executable evidence |
|---------|------|------|---------------------|
| IEScript | api claim | `raw memory access requires cpu.freeze()` | `script_engine.go` `requireFrozenForRange` error path |
| IEScript | api contract | `bit32.arshift(x, disp) masks disp to 0..31, sign-extends, and returns number` | `script_engine.go` `registerBit32` `arshift` |
| IEScript | api contract | `bit32.btest(...) returns boolean true when the bitwise AND result is non-zero` | `script_engine.go` `registerBit32` `btest` |
| IEScript | api contract | `bit32.extract(x, field[, width]) raises an error for field < 0, width <= 0, or field + width > 32` | `script_engine.go` `registerBit32` `extract` range check |
| IEScript | api contract | `bit32.lrotate(x, disp) masks disp to 0..31 and returns number` | `script_engine.go` `registerBit32` `lrotate` |
| IEScript | api contract | `bit32.lshift(x, disp) masks disp to 0..31 and returns number` | `script_engine.go` `registerBit32` `lshift` |
| IEScript | api contract | `bit32.replace(x, v, field[, width]) raises an error for field < 0, width <= 0, or field + width > 32` | `script_engine.go` `registerBit32` `replace` range check |
| IEScript | api contract | `bit32.rrotate(x, disp) masks disp to 0..31 and returns number` | `script_engine.go` `registerBit32` `rrotate` |
| IEScript | api contract | `bit32.rshift(x, disp) masks disp to 0..31 and returns number` | `script_engine.go` `registerBit32` `rshift` |
| IEScript | api contract | `dbg.history_config([opts]) accepts delta_interval, delta_mib, checkpoints, and snapshots as positive table fields` | `script_engine.go` `luaDbgHistoryConfig` option fields and positive-value check |
| IEScript | api contract | `dbg.history_config([opts]) returns delta_interval, delta_mib, checkpoints, and snapshots` | `script_engine.go` `luaDbgHistoryConfig` return table fields |
| IEScript | api contract | `dbg.history_horizon() returns snapshots, checkpoints, deltas, capacity, delta_bytes, checkpoint_interval, checkpoint_mib, retained_checkpoints, and devices` | `script_engine.go` `luaDbgHistoryHorizon` table fields |
| IEScript | api contract | `media.type() returns sid, psg, ted, ahx, pokey, mod, wav, midi, or none` | `script_engine.go` `mediaTypeToString`, `media_loader.go` MIDI extension detection |
| IEScript | api contract | `mem.fill(addr, len, value) fills bytes, returns nothing, and requires len >= 0` | `script_engine.go` `luaMemFill` length check and write loop |
| IEScript | api contract | `mem.read16(addr) returns number and truncates addr to uint32` | `script_engine.go` `luaMemRead16` `uint32(L.CheckInt(1))` |
| IEScript | api contract | `mem.read32(addr) returns number and truncates addr to uint32` | `script_engine.go` `luaMemRead32` `uint32(L.CheckInt(1))` |
| IEScript | api contract | `mem.read8(addr) returns number and truncates addr to uint32` | `script_engine.go` `luaMemRead8` `uint32(L.CheckInt(1))` |
| IEScript | api contract | `mem.read_block(addr, len) returns a raw byte string; len must be >= 0` | `script_engine.go` `luaMemReadBlock` length check and `lua.LString` return |
| IEScript | api contract | `mem.write16(addr, value) returns nothing and truncates addr to uint32` | `script_engine.go` `luaMemWrite16` `uint32(L.CheckInt(1))` |
| IEScript | api contract | `mem.write32(addr, value) returns nothing and truncates addr to uint32` | `script_engine.go` `luaMemWrite32` `uint32(L.CheckInt(1))` |
| IEScript | api contract | `mem.write8(addr, value) returns nothing and truncates addr to uint32` | `script_engine.go` `luaMemWrite8` `uint32(L.CheckInt(1))` |
| IEScript | api contract | `mem.write_block(addr, bytes) writes a raw byte string and returns nothing` | `script_engine.go` `luaMemWriteBlock` byte loop |
| IEScript | binding | `audio.ahx_is_playing` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `audio.ahx_load` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `audio.ahx_play` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `audio.ahx_stop` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `audio.configure_master_auto_level` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `audio.configure_master_compressor` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `audio.freeze` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `audio.get_master_gain_db` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `audio.midi_is_playing` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `audio.midi_load` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `audio.midi_metadata` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `audio.midi_pause` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `audio.midi_play` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `audio.midi_resume` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `audio.midi_set_volume` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `audio.midi_stop` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `audio.pokey_is_playing` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `audio.pokey_load` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `audio.pokey_play` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `audio.pokey_stop` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `audio.psg_is_playing` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `audio.psg_load` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `audio.psg_metadata` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `audio.psg_play` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `audio.psg_stop` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `audio.reset` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `audio.reset_master_dynamics` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `audio.resume` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `audio.set_master_auto_level_enabled` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `audio.set_master_compressor_enabled` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `audio.set_master_gain_db` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `audio.sid_is_playing` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `audio.sid_load` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `audio.sid_metadata` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `audio.sid_play` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `audio.sid_stop` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `audio.start` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `audio.stop` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `audio.ted_is_playing` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `audio.ted_load` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `audio.ted_play` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `audio.ted_stop` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `audio.use_showreel_normalizer_preset` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `audio.write_reg` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `bit32.arshift` | `script_engine.go` `registerBit32` binding |
| IEScript | binding | `bit32.band` | `script_engine.go` `registerBit32` binding |
| IEScript | binding | `bit32.bnot` | `script_engine.go` `registerBit32` binding |
| IEScript | binding | `bit32.bor` | `script_engine.go` `registerBit32` binding |
| IEScript | binding | `bit32.btest` | `script_engine.go` `registerBit32` binding |
| IEScript | binding | `bit32.bxor` | `script_engine.go` `registerBit32` binding |
| IEScript | binding | `bit32.extract` | `script_engine.go` `registerBit32` binding |
| IEScript | binding | `bit32.lrotate` | `script_engine.go` `registerBit32` binding |
| IEScript | binding | `bit32.lshift` | `script_engine.go` `registerBit32` binding |
| IEScript | binding | `bit32.replace` | `script_engine.go` `registerBit32` binding |
| IEScript | binding | `bit32.rrotate` | `script_engine.go` `registerBit32` binding |
| IEScript | binding | `bit32.rshift` | `script_engine.go` `registerBit32` binding |
| IEScript | binding | `coproc.enqueue` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `coproc.poll` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `coproc.response` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `coproc.start` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `coproc.stats` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `coproc.stop` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `coproc.wait` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `coproc.workers` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `cpu.execution_mode` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `cpu.freeze` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `cpu.is_running` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `cpu.jit_enabled` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `cpu.load` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `cpu.load_stopped` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `cpu.mode` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `cpu.reset` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `cpu.resume` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `cpu.set_jit_enabled` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `cpu.start` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `cpu.stop` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `dbg.accesslog` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `dbg.accesslog_off` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `dbg.accesslog_on` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `dbg.backstep` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `dbg.backtrace` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `dbg.backtrace_frames` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `dbg.bfirst` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `dbg.bug_report` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `dbg.clear_all_bp` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `dbg.clear_all_wp` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `dbg.clear_bp` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `dbg.clear_wp` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `dbg.close` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `dbg.command` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `dbg.command_output` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `dbg.compare_mem` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `dbg.continue` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `dbg.cpu_focus` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `dbg.cpu_list` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `dbg.cpu_offline` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `dbg.cpu_online` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `dbg.device_diff` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `dbg.device_list` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `dbg.device_snapshot` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `dbg.disasm` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `dbg.fault_break` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `dbg.fault_clear` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `dbg.fault_list` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `dbg.fault_off` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `dbg.fault_on` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `dbg.fill_mem` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `dbg.freeze` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `dbg.freeze_all` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `dbg.freeze_audio` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `dbg.freeze_cpu` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `dbg.get_pc` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `dbg.get_reg` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `dbg.get_regs` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `dbg.guard_add` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `dbg.guard_del` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `dbg.guard_list` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `dbg.help` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `dbg.history_config` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `dbg.history_horizon` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `dbg.hunt_mem` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `dbg.io` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `dbg.io_devices` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `dbg.is_open` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `dbg.layout` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `dbg.list_bp` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `dbg.list_wp` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `dbg.load_mem_file` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `dbg.load_state` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `dbg.macro` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `dbg.on_fault` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `dbg.open` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `dbg.poll_faults` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `dbg.read_mem` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `dbg.request_break_in` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `dbg.resume` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `dbg.reverse_continue` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `dbg.reverse_until` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `dbg.run_script` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `dbg.run_until` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `dbg.save_mem_file` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `dbg.save_state` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `dbg.set_bp` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `dbg.set_conditional_bp` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `dbg.set_pc` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `dbg.set_reg` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `dbg.set_wp` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `dbg.set_wp_ex` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `dbg.source_at` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `dbg.step` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `dbg.thaw_all` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `dbg.thaw_audio` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `dbg.thaw_cpu` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `dbg.timeline` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `dbg.trace` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `dbg.trace_file` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `dbg.trace_file_off` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `dbg.trace_history` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `dbg.trace_history_clear` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `dbg.trace_watch_add` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `dbg.trace_watch_del` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `dbg.trace_watch_list` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `dbg.tracering_off` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `dbg.tracering_on` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `dbg.tracering_show` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `dbg.transfer_mem` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `dbg.who` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `dbg.write_mem` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `keys.A` | `script_engine.go` `keys` table binding |
| IEScript | binding | `keys.B` | `script_engine.go` `keys` table binding |
| IEScript | binding | `keys.BACKSPACE` | `script_engine.go` `keys` table binding |
| IEScript | binding | `keys.C` | `script_engine.go` `keys` table binding |
| IEScript | binding | `keys.CAPSLOCK` | `script_engine.go` `keys` table binding |
| IEScript | binding | `keys.D` | `script_engine.go` `keys` table binding |
| IEScript | binding | `keys.DIGIT_0` | `script_engine.go` `keys` table binding |
| IEScript | binding | `keys.DIGIT_1` | `script_engine.go` `keys` table binding |
| IEScript | binding | `keys.DIGIT_2` | `script_engine.go` `keys` table binding |
| IEScript | binding | `keys.DIGIT_3` | `script_engine.go` `keys` table binding |
| IEScript | binding | `keys.DIGIT_4` | `script_engine.go` `keys` table binding |
| IEScript | binding | `keys.DIGIT_5` | `script_engine.go` `keys` table binding |
| IEScript | binding | `keys.DIGIT_6` | `script_engine.go` `keys` table binding |
| IEScript | binding | `keys.DIGIT_7` | `script_engine.go` `keys` table binding |
| IEScript | binding | `keys.DIGIT_8` | `script_engine.go` `keys` table binding |
| IEScript | binding | `keys.DIGIT_9` | `script_engine.go` `keys` table binding |
| IEScript | binding | `keys.DOWN` | `script_engine.go` `keys` table binding |
| IEScript | binding | `keys.E` | `script_engine.go` `keys` table binding |
| IEScript | binding | `keys.ENTER` | `script_engine.go` `keys` table binding |
| IEScript | binding | `keys.EQUAL` | `script_engine.go` `keys` table binding |
| IEScript | binding | `keys.ESCAPE` | `script_engine.go` `keys` table binding |
| IEScript | binding | `keys.F` | `script_engine.go` `keys` table binding |
| IEScript | binding | `keys.F1` | `script_engine.go` `keys` table binding |
| IEScript | binding | `keys.F10` | `script_engine.go` `keys` table binding |
| IEScript | binding | `keys.F2` | `script_engine.go` `keys` table binding |
| IEScript | binding | `keys.F3` | `script_engine.go` `keys` table binding |
| IEScript | binding | `keys.F4` | `script_engine.go` `keys` table binding |
| IEScript | binding | `keys.F5` | `script_engine.go` `keys` table binding |
| IEScript | binding | `keys.F6` | `script_engine.go` `keys` table binding |
| IEScript | binding | `keys.F7` | `script_engine.go` `keys` table binding |
| IEScript | binding | `keys.F8` | `script_engine.go` `keys` table binding |
| IEScript | binding | `keys.F9` | `script_engine.go` `keys` table binding |
| IEScript | binding | `keys.G` | `script_engine.go` `keys` table binding |
| IEScript | binding | `keys.H` | `script_engine.go` `keys` table binding |
| IEScript | binding | `keys.I` | `script_engine.go` `keys` table binding |
| IEScript | binding | `keys.J` | `script_engine.go` `keys` table binding |
| IEScript | binding | `keys.K` | `script_engine.go` `keys` table binding |
| IEScript | binding | `keys.L` | `script_engine.go` `keys` table binding |
| IEScript | binding | `keys.LCTRL` | `script_engine.go` `keys` table binding |
| IEScript | binding | `keys.LEFT` | `script_engine.go` `keys` table binding |
| IEScript | binding | `keys.LSHIFT` | `script_engine.go` `keys` table binding |
| IEScript | binding | `keys.M` | `script_engine.go` `keys` table binding |
| IEScript | binding | `keys.MINUS` | `script_engine.go` `keys` table binding |
| IEScript | binding | `keys.N` | `script_engine.go` `keys` table binding |
| IEScript | binding | `keys.O` | `script_engine.go` `keys` table binding |
| IEScript | binding | `keys.P` | `script_engine.go` `keys` table binding |
| IEScript | binding | `keys.Q` | `script_engine.go` `keys` table binding |
| IEScript | binding | `keys.R` | `script_engine.go` `keys` table binding |
| IEScript | binding | `keys.RIGHT` | `script_engine.go` `keys` table binding |
| IEScript | binding | `keys.RSHIFT` | `script_engine.go` `keys` table binding |
| IEScript | binding | `keys.S` | `script_engine.go` `keys` table binding |
| IEScript | binding | `keys.SPACE` | `script_engine.go` `keys` table binding |
| IEScript | binding | `keys.T` | `script_engine.go` `keys` table binding |
| IEScript | binding | `keys.TAB` | `script_engine.go` `keys` table binding |
| IEScript | binding | `keys.U` | `script_engine.go` `keys` table binding |
| IEScript | binding | `keys.UP` | `script_engine.go` `keys` table binding |
| IEScript | binding | `keys.V` | `script_engine.go` `keys` table binding |
| IEScript | binding | `keys.W` | `script_engine.go` `keys` table binding |
| IEScript | binding | `keys.X` | `script_engine.go` `keys` table binding |
| IEScript | binding | `keys.Y` | `script_engine.go` `keys` table binding |
| IEScript | binding | `keys.Z` | `script_engine.go` `keys` table binding |
| IEScript | binding | `media.error` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `media.load` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `media.load_subsong` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `media.play` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `media.status` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `media.stop` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `media.type` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `mem.fill` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `mem.read16` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `mem.read32` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `mem.read8` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `mem.read_block` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `mem.write16` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `mem.write32` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `mem.write8` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `mem.write_block` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `rec.frame_count` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `rec.is_recording` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `rec.screenshot` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `rec.start` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `rec.start_screen` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `rec.stop` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `regions.list` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `regions.lookup` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `repl.clear` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `repl.hide` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `repl.is_open` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `repl.line_count` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `repl.print` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `repl.scroll_down` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `repl.scroll_up` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `repl.show` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `sym.add` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `sym.autoload` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `sym.list` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `sym.load_dwarf` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `sym.load_elf` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `sym.load_vice` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `sym.lookup` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `sym.resolve` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `sys.capture_output` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `sys.capture_output_off` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `sys.copy_file` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `sys.emutos_drive` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `sys.exit` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `sys.fps` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `sys.frame_count` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `sys.frame_time` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `sys.log` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `sys.mkdir` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `sys.print` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `sys.quit` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `sys.read_file` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `sys.time_ms` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `sys.wait_frames` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `sys.wait_ms` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `sys.write_file` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `term.clear` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `term.echo` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `term.key_press` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `term.mouse_click` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `term.mouse_delta` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `term.mouse_double_click` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `term.mouse_move` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `term.mouse_release` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `term.read` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `term.scancode` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `term.type` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `term.type_line` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `term.wait_output` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `video.antic_charset` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `video.antic_dlist` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `video.antic_dma` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `video.antic_enable` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `video.antic_get_dimensions` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `video.antic_is_enabled` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `video.antic_pmbase` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `video.antic_scroll` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `video.blit_copy` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `video.blit_fill` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `video.blit_line` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `video.blit_wait` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `video.copper_enable` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `video.copper_is_running` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `video.copper_set_program` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `video.frame_hash` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `video.get_dimensions` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `video.get_pixel` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `video.get_region` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `video.gtia_color` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `video.gtia_player_gfx` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `video.gtia_player_pos` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `video.gtia_player_size` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `video.gtia_priority` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `video.is_enabled` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `video.read_reg` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `video.ted_charset` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `video.ted_colors` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `video.ted_cursor` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `video.ted_enable` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `video.ted_get_dimensions` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `video.ted_is_enabled` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `video.ted_mode` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `video.ted_video_base` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `video.ula_border` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `video.ula_enable` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `video.ula_get_dimensions` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `video.ula_is_enabled` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `video.vga_enable` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `video.vga_get_dimensions` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `video.vga_get_palette` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `video.vga_set_mode` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `video.vga_set_palette` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `video.voodoo_alpha` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `video.voodoo_chromakey` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `video.voodoo_clear` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `video.voodoo_clip` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `video.voodoo_color` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `video.voodoo_depth` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `video.voodoo_dither` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `video.voodoo_draw` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `video.voodoo_enable` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `video.voodoo_fog` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `video.voodoo_get_dimensions` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `video.voodoo_is_enabled` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `video.voodoo_resolution` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `video.voodoo_swap` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `video.voodoo_texcoord` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `video.voodoo_texture` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `video.voodoo_vertex` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `video.voodoo_zbuffer` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `video.wait_condition` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `video.wait_pixel` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `video.wait_stable` | `script_engine.go` `registerModules` binding |
| IEScript | binding | `video.write_reg` | `script_engine.go` `registerModules` binding |
