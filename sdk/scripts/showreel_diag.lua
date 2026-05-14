local M = {}

local floor = math.floor

local function path_join(...)
    local parts = {...}
    return table.concat(parts, "/")
end

local function ensure_dir(path)
    return sys.mkdir(path)
end

local function read_file(path)
    local ok, data = pcall(sys.read_file, path)
    if ok then return data end
    return nil
end

local function write_file(path, data)
    sys.write_file(path, data)
end

local function is_array(tbl)
    local n = 0
    for k, _ in pairs(tbl) do
        if type(k) ~= "number" or k < 1 or floor(k) ~= k then
            return false
        end
        if k > n then
            n = k
        end
    end
    for i = 1, n do
        if tbl[i] == nil then
            return false
        end
    end
    return true
end

local function json_escape(s)
    return tostring(s)
        :gsub("\\", "\\\\")
        :gsub("\"", "\\\"")
        :gsub("\b", "\\b")
        :gsub("\f", "\\f")
        :gsub("\n", "\\n")
        :gsub("\r", "\\r")
        :gsub("\t", "\\t")
end

local function json_encode(v)
    local tv = type(v)
    if tv == "nil" then
        return "null"
    end
    if tv == "boolean" then
        return v and "true" or "false"
    end
    if tv == "number" then
        return tostring(v)
    end
    if tv == "string" then
        return "\"" .. json_escape(v) .. "\""
    end
    if tv ~= "table" then
        return "\"" .. json_escape(tostring(v)) .. "\""
    end
    if is_array(v) then
        local out = {}
        for i = 1, #v do
            out[#out + 1] = json_encode(v[i])
        end
        return "[" .. table.concat(out, ",") .. "]"
    end
    local keys = {}
    for k, _ in pairs(v) do
        keys[#keys + 1] = tostring(k)
    end
    table.sort(keys)
    local out = {}
    for _, k in ipairs(keys) do
        out[#out + 1] = "\"" .. json_escape(k) .. "\":" .. json_encode(v[k])
    end
    return "{" .. table.concat(out, ",") .. "}"
end

local function write_json(path, value)
    write_file(path, json_encode(value) .. "\n")
end

local function count_keys(tbl)
    local n = 0
    for _, _ in pairs(tbl) do
        n = n + 1
    end
    return n
end

local function distinct_count(items)
    local seen = {}
    for _, item in ipairs(items) do
        seen[tostring(item)] = true
    end
    return count_keys(seen)
end

local function first_repeated(items)
    local seen = {}
    for _, item in ipairs(items) do
        local key = tostring(item)
        seen[key] = (seen[key] or 0) + 1
        if seen[key] == 2 then
            return item, seen[key]
        end
    end
    return nil, 0
end

local function be16(v)
    local lo = v % 256
    local hi = floor(v / 256) % 256
    return hi + lo * 256
end

local function be32(v)
    local b0 = v % 256
    v = floor(v / 256)
    local b1 = v % 256
    v = floor(v / 256)
    local b2 = v % 256
    v = floor(v / 256)
    local b3 = v % 256
    return b3 + b2 * 256 + b1 * 65536 + b0 * 16777216
end

local function read_be16(addr)
    return be16(mem.read16(addr))
end

local function read_s16be(addr)
    local v = read_be16(addr)
    if v >= 0x8000 then
        return v - 0x10000
    end
    return v
end

local function read_be32(addr)
    return be32(mem.read32(addr))
end

local function with_snapshot(fn)
    dbg.freeze()
    cpu.freeze()
    local ok, a, b, c, d = pcall(fn)
    cpu.resume()
    dbg.resume()
    if not ok then
        error(a)
    end
    return a, b, c, d
end

local function sample_frame()
    local w, h = video.get_dimensions()
    local points = {}
    local colors = {}
    local step_x = math.max(1, floor(w / 6))
    local step_y = math.max(1, floor(h / 6))
    for y = 0, h - 1, step_y do
        for x = 0, w - 1, step_x do
            local r, g, b = video.get_pixel(x, y)
            local key = string.format("%d,%d,%d", r, g, b)
            colors[key] = true
            points[#points + 1] = {x = x, y = y, r = r, g = g, b = b}
        end
    end
    return {
        width = w,
        height = h,
        hash = video.frame_hash(),
        distinct_colors = count_keys(colors),
        non_uniform = count_keys(colors) > 1,
        points = points,
    }
end

local function snapshot_devices(device_names)
    local devices = {}
    for _, name in ipairs(device_names or {}) do
        devices[name] = dbg.io(name)
    end
    return devices
end

local function snapshot_monitor(device_names, disasm_count)
    return with_snapshot(function()
        local pc = dbg.get_pc()
        local start = pc >= 16 and (pc - 16) or 0
        return {
            pc = pc,
            regs = dbg.get_regs(),
            disasm = dbg.disasm(start, disasm_count or 12),
            devices = snapshot_devices(device_names),
            frame = sample_frame(),
        }
    end)
end

local function device_activity(before, after)
    local out = {}
    for name, before_regs in pairs(before or {}) do
        local after_regs = (after or {})[name] or {}
        local changed = 0
        for i, reg in ipairs(before_regs) do
            local next_reg = after_regs[i]
            if next_reg and reg.value ~= next_reg.value then
                changed = changed + 1
            end
        end
        out[name] = {
            changed = changed,
            active = changed > 0,
        }
    end
    return out
end

local function repeated_pc_info(pcs)
    local repeated = first_repeated(pcs)
    return {
        distinct = distinct_count(pcs),
        first_repeated_pc = repeated,
        stuck = distinct_count(pcs) <= 2 and #pcs >= 3,
    }
end

local function summarize_console_log(path)
    local data = read_file(path) or ""
    local counts = {}
    local ordered = {}
    for line in (data .. "\n"):gmatch("(.-)\n") do
        if line ~= "" then
            counts[line] = (counts[line] or 0) + 1
            if counts[line] == 1 then
                ordered[#ordered + 1] = line
            end
        end
    end
    local first_repeated_line = nil
    for _, line in ipairs(ordered) do
        if counts[line] and counts[line] > 1 then
            first_repeated_line = line
            break
        end
    end
    local histogram = {}
    for _, line in ipairs(ordered) do
        histogram[#histogram + 1] = {
            line = line,
            count = counts[line],
        }
    end
    return {
        first_repeated_error = first_repeated_line,
        unique_error_count = #histogram,
        histogram = histogram,
    }
end

local function wait_for(deadline_ms, interval_ms, predicate)
    local last = nil
    while sys.time_ms() < deadline_ms do
        last = predicate()
        if last then
            return true, last
        end
        sys.wait_ms(interval_ms or 100)
    end
    return false, last
end

local function default_logger(path)
    local lines = {}
    return function(msg)
        local line = string.format("[%d] %s", sys.time_ms(), msg)
        lines[#lines + 1] = line
    end, function()
        write_file(path, table.concat(lines, "\n") .. "\n")
    end
end

local function final_bucket(jit_run, interp_run, fallback)
    if jit_run.pass and not interp_run.pass then
        return "M68K interpreter / core execute bug"
    end
    if interp_run.pass and not jit_run.pass then
        return "M68K JIT bug"
    end
    if not jit_run.pass and not interp_run.pass then
        return fallback or "needs-triage"
    end
    return "none"
end

function M.pass_video(report)
    return report.final.frame.non_uniform
        and report.progress.frame_hashes.distinct > 1
        and report.progress.pcs.distinct > 1
        and (report.device_activity.video == nil or report.device_activity.video.active)
end

function M.pass_ted(report)
    return report.final.frame.non_uniform
        and report.progress.pcs.distinct > 1
        and report.device_activity.ted
        and report.device_activity.ted.active
end

function M.pass_voodoo(report)
    return report.final.frame.non_uniform
        and report.progress.pcs.distinct > 1
        and report.device_activity.voodoo
        and report.device_activity.voodoo.active
end

function M.run_raw_ab(cfg)
    local root = cfg.output_root or "showreel_diag"
    local base_dir = path_join(root, cfg.demo_name)
    ensure_dir(base_dir)
    local interpreter_only = cfg.interpreter_only and true or false

    local function run_one(use_jit)
        local mode_name = use_jit and "jit" or "interpreter"
        local out_dir = path_join(base_dir, mode_name)
        ensure_dir(out_dir)
        local log, close_log = default_logger(path_join(out_dir, "run.log"))

        local report = {
            demo_name = cfg.demo_name,
            program_path = cfg.program_path,
            execution_mode = mode_name,
            requested_jit = use_jit,
            started_at_ms = sys.time_ms(),
            pass = false,
            suspected_subsystem = cfg.failure_bucket or "needs-triage",
        }

        log("instantiating target cpu")
        cpu.stop()
        local ok_load, load_err = pcall(cpu.load, cfg.program_path)
        if not ok_load then
            report.load_error = tostring(load_err)
            write_json(path_join(out_dir, "summary.json"), report)
            close_log()
            return report
        end

        cpu.stop()

        log("configuring target cpu mode")
        local ok_mode, mode_err = pcall(cpu.set_jit_enabled, use_jit)
        if not ok_mode then
            report.mode_error = tostring(mode_err)
            write_json(path_join(out_dir, "summary.json"), report)
            close_log()
            return report
        end
        report.actual_execution_mode = cpu.execution_mode()

        local console_path = path_join(out_dir, "console.log")
        sys.capture_output(console_path)
        report.console_log_path = console_path

        local trace_path = path_join(out_dir, "trace.log")
        local ok_trace, trace_err = pcall(dbg.trace_file, trace_path)
        if not ok_trace then
            log("trace file setup failed: " .. tostring(trace_err))
        end

        log("reloading program for clean capture")
        local ok_reload, reload_err = pcall(cpu.load, cfg.program_path)
        if not ok_reload then
            report.load_error = tostring(reload_err)
            pcall(sys.capture_output_off)
            report.console_summary = summarize_console_log(console_path)
            write_json(path_join(out_dir, "summary.json"), report)
            close_log()
            return report
        end
        report.actual_execution_mode = cpu.execution_mode()

        sys.wait_ms(cfg.warmup_ms or 250)
        report.initial = snapshot_monitor(cfg.devices, cfg.disasm_count or 12)

        local pcs = {}
        local frame_hashes = {}
        local deadline = sys.time_ms() + (cfg.duration_ms or 4000)
        while sys.time_ms() < deadline do
            sys.wait_ms(cfg.sample_interval_ms or 150)
            pcs[#pcs + 1] = dbg.get_pc()
            frame_hashes[#frame_hashes + 1] = video.frame_hash()
        end

        report.final = snapshot_monitor(cfg.devices, cfg.disasm_count or 12)
        report.device_activity = device_activity(report.initial.devices, report.final.devices)
        report.progress = {
            pcs = repeated_pc_info(pcs),
            frame_hashes = {
                distinct = distinct_count(frame_hashes),
                first_repeated_hash = first_repeated(frame_hashes),
            },
        }

        local shot_ok, shot_err = pcall(rec.screenshot, path_join(out_dir, "final.png"))
        if not shot_ok then
            log("screenshot failed: " .. tostring(shot_err))
        end

        local pass_fn = cfg.pass_fn or M.pass_video
        local ok_pass, pass_value = pcall(pass_fn, report)
        if ok_pass then
            report.pass = pass_value and true or false
        else
            report.pass = false
            report.pass_eval_error = tostring(pass_value)
        end

        if not report.pass then
            report.first_failing_pc = report.progress.pcs.first_repeated_pc or report.final.pc
        end
        report.ended_at_ms = sys.time_ms()

        local trace_off_ok, trace_off_err = pcall(dbg.trace_file_off)
        if not trace_off_ok then
            log("trace file shutdown failed: " .. tostring(trace_off_err))
        end
        local cap_off_ok, cap_off_err = pcall(sys.capture_output_off)
        if not cap_off_ok then
            log("console capture shutdown failed: " .. tostring(cap_off_err))
        end
        report.console_summary = summarize_console_log(console_path)
        write_json(path_join(out_dir, "summary.json"), report)
        write_json(path_join(out_dir, "snapshot.json"), report.final)
        cpu.stop()
        close_log()
        return report
    end

    local overall
    if interpreter_only then
        local interp_run = run_one(false)
        overall = {
            demo_name = cfg.demo_name,
            interpreter = interp_run,
            final_bucket = interp_run.pass and "none" or (interp_run.suspected_subsystem or cfg.failure_bucket),
            pass = interp_run.pass,
        }
        overall.first_failing_pc = (not overall.pass) and interp_run.first_failing_pc or nil
    else
        local jit_run = run_one(true)
        local interp_run = run_one(false)
        overall = {
            demo_name = cfg.demo_name,
            jit = jit_run,
            interpreter = interp_run,
            final_bucket = final_bucket(jit_run, interp_run, cfg.failure_bucket),
            pass = jit_run.pass and interp_run.pass,
        }
        overall.first_failing_pc = (not overall.pass) and (jit_run.first_failing_pc or interp_run.first_failing_pc) or nil
    end
    write_json(path_join(base_dir, "ab_summary.json"), overall)
    sys.exit(overall.pass and 0 or 1)
end

local EMUTOS = {
    addr_dev_tab0 = 0x1E12,
    addr_dev_tab1 = 0x1E14,
    addr_gcurx = 0x1E6C,
    addr_gcury = 0x1E6E,
    addr_mouse_bt = 0x1E72,
    addr_gl_mntree = 0xD120,
    addr_gl_ctwait = 0x9614,
}

local function sample_desktop_colors()
    local points = {
        {320, 120}, {320, 240}, {320, 360},
        {160, 240}, {480, 240}, {35, 44},
    }
    local colors = {}
    for _, p in ipairs(points) do
        local r, g, b = video.get_pixel(p[1], p[2])
        colors[string.format("%d,%d,%d", r, g, b)] = true
    end
    return count_keys(colors)
end

local function boot_oracle()
    return with_snapshot(function()
        local x = read_s16be(EMUTOS.addr_dev_tab0)
        local y = read_s16be(EMUTOS.addr_dev_tab1)
        if x == 639 and y == 479 then
            return {
                oracle = "DEV_TAB dimensions present",
                oracle_type = "memory-backed",
                detail = {dev_tab_x = x, dev_tab_y = y},
            }
        end
        return nil
    end)
end

local function desktop_oracle()
    return with_snapshot(function()
        local menu_tree = read_be32(EMUTOS.addr_gl_mntree)
        if menu_tree ~= 0 then
            return {
                oracle = "gl_mntree installed",
                oracle_type = "memory-backed",
                detail = {
                    gl_mntree = menu_tree,
                    sampled_colors = sample_desktop_colors(),
                },
            }
        end
        return nil
    end)
end

local function mouse_stage()
    term.mouse_move(400, 300)
    sys.wait_ms(500)
    local after_move = with_snapshot(function()
        return {
            x = read_s16be(EMUTOS.addr_gcurx),
            y = read_s16be(EMUTOS.addr_gcury),
        }
    end)

    term.mouse_click(400, 300, 1)
    sys.wait_ms(300)
    local after_click = with_snapshot(function()
        return {
            buttons = read_s16be(EMUTOS.addr_mouse_bt),
        }
    end)
    term.mouse_release()

    if after_move.x == 400 and after_move.y == 300 and after_click.buttons == 1 then
        return true, {
            oracle = "GCURX/GCURY and MOUSE_BT reflect injected mouse state",
            oracle_type = "memory-backed",
            detail = {
                gcurx = after_move.x,
                gcury = after_move.y,
                mouse_bt = after_click.buttons,
            },
        }
    end
    return false, {
        oracle = "GCURX/GCURY and MOUSE_BT reflect injected mouse state",
        oracle_type = "memory-backed",
        detail = {
            gcurx = after_move.x,
            gcury = after_move.y,
            mouse_bt = after_click.buttons,
        },
    }
end

local function drive_open_stage(coords)
    local before_hash = video.frame_hash()
    term.mouse_move(coords.drive_x, coords.drive_y)
    sys.wait_ms(300)
    term.mouse_double_click(coords.drive_x, coords.drive_y, 1)
    sys.wait_ms(2500)
    local after_hash = video.frame_hash()
    local changed = before_hash ~= after_hash
    return changed, {
        oracle = "frame hash changes after drive double-click",
        oracle_type = "visual/automation-backed",
        detail = {
            before_hash = before_hash,
            after_hash = after_hash,
            drive_x = coords.drive_x,
            drive_y = coords.drive_y,
        },
    }
end

local function prg_launch_stage(coords)
    local before_hash = video.frame_hash()
    term.mouse_move(coords.file_x, coords.file_y)
    sys.wait_ms(300)
    term.mouse_double_click(coords.file_x, coords.file_y, 1)
    sys.wait_ms(4000)
    local snap = snapshot_monitor({"video"}, 12)
    local changed = before_hash ~= snap.frame.hash
    local pc_outside_rom = snap.pc < 0x00E00000
    return changed or pc_outside_rom, {
        oracle = "frame hash changes or PC leaves ROM space after .PRG launch",
        oracle_type = pc_outside_rom and "mixed" or "visual/automation-backed",
        detail = {
            before_hash = before_hash,
            after_hash = snap.frame.hash,
            pc = snap.pc,
            file_x = coords.file_x,
            file_y = coords.file_y,
        },
    }
end

function M.run_emutos_rotozoom(cfg)
    local root = cfg.output_root or "showreel_diag"
    local base_dir = path_join(root, "emutos_rotozoomer")
    ensure_dir(base_dir)
    local log, close_log = default_logger(path_join(base_dir, "run.log"))

    local drive_path = cfg.drive_path or path_join(root, "emutos_drive")
    local drive_abs = ensure_dir(drive_path)
    sys.copy_file("sdk/examples/prebuilt/rotozoomer_gem.prg", path_join(drive_path, "ROTOZOOM.PRG"))
    sys.emutos_drive(drive_abs)

    log("booting EmuTOS")
    cpu.stop()
    if cfg.interpreter_only then
        local ok_mode, mode_err = pcall(cpu.set_jit_enabled, false)
        if not ok_mode then
            log("failed to disable m68k jit: " .. tostring(mode_err))
        end
    end
    local console_path = path_join(base_dir, "console.log")
    sys.capture_output(console_path)
    cpu.load("EMUTOS")

    local stages = {}
    local function record_stage(name, passed, oracle)
        stages[#stages + 1] = {
            name = name,
            pass = passed,
            oracle = oracle.oracle,
            oracle_type = oracle.oracle_type,
            detail = oracle.detail,
        }
        local shot_path = path_join(base_dir, name .. ".png")
        pcall(rec.screenshot, shot_path)
        return passed
    end

    local ok_boot, boot_data = wait_for(sys.time_ms() + (cfg.boot_timeout_ms or 8000), 100, boot_oracle)
    if not record_stage("rom_boot", ok_boot, boot_data or {oracle = "boot timeout", oracle_type = "memory-backed", detail = {}}) then
        pcall(sys.capture_output_off)
        write_json(path_join(base_dir, "summary.json"), {
            pass = false,
            stages = stages,
            console_log_path = console_path,
            console_summary = summarize_console_log(console_path),
        })
        close_log()
        sys.exit(1)
    end

    local ok_desktop, desktop_data = wait_for(sys.time_ms() + (cfg.desktop_timeout_ms or 10000), 100, desktop_oracle)
    if not record_stage("desktop_reached", ok_desktop, desktop_data or {oracle = "desktop timeout", oracle_type = "memory-backed", detail = {}}) then
        pcall(sys.capture_output_off)
        write_json(path_join(base_dir, "summary.json"), {
            pass = false,
            stages = stages,
            console_log_path = console_path,
            console_summary = summarize_console_log(console_path),
        })
        close_log()
        sys.exit(1)
    end

    local ok_mouse, mouse_data = mouse_stage()
    if not record_stage("mouse_input", ok_mouse, mouse_data) then
        pcall(sys.capture_output_off)
        write_json(path_join(base_dir, "summary.json"), {
            pass = false,
            stages = stages,
            console_log_path = console_path,
            console_summary = summarize_console_log(console_path),
        })
        close_log()
        sys.exit(1)
    end

    local coords = {
        drive_x = cfg.drive_x or 35,
        drive_y = cfg.drive_y or 44,
        file_x = cfg.file_x or 149,
        file_y = cfg.file_y or 155,
    }

    local ok_drive, drive_data = drive_open_stage(coords)
    if not record_stage("drive_open", ok_drive, drive_data) then
        pcall(sys.capture_output_off)
        write_json(path_join(base_dir, "summary.json"), {
            pass = false,
            stages = stages,
            console_log_path = console_path,
            console_summary = summarize_console_log(console_path),
        })
        close_log()
        sys.exit(1)
    end

    local ok_prg, prg_data = prg_launch_stage(coords)
    record_stage("prg_launch", ok_prg, prg_data)

    local summary = {
        pass = ok_boot and ok_desktop and ok_mouse and ok_drive and ok_prg,
        stages = stages,
        console_log_path = console_path,
    }
    pcall(sys.capture_output_off)
    summary.console_summary = summarize_console_log(console_path)
    write_json(path_join(base_dir, "summary.json"), summary)
    close_log()
    sys.exit(summary.pass and 0 or 1)
end

return M
