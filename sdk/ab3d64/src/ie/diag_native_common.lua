local M = {}

package.path = "_build/?.lua;ab3d2_source/_build/?.lua;ie/?.lua;ab3d2_source/ie/?.lua;" .. package.path

M.MODE_1920x1080 = 0x06
M.CLUT8 = 0x01
M.NATIVE_W = 1920
M.NATIVE_H = 1080
M.SCALED_W = 320
M.SCALED_H = 240
M.NATIVE_BYTES = M.NATIVE_W * M.NATIVE_H
M.SCALED_BYTES = M.SCALED_W * M.SCALED_H
M.NATIVE_CHUNKY_BASE = 0x02000000
M.NATIVE_CHUNKY_BACK_BASE = 0x02200000
M.NATIVE_MENU_SCALE_BASE = 0x02400000
M.NATIVE_MENU_SCALE_BACK_BASE = 0x02600000
M.OVERDRIVE_SCALE_BASE = 0x02000000
M.OVERDRIVE_SCALE_BACK_BASE = 0x02200000
M.FILE_HEAP_PTR = 0x003FFF00
M.FILE_HEAP_BASE = 0x00700000
M.FILE_HEAP_LIMIT = 0x00FE0000

M.VIDEO_CTRL = 0xF0000
M.VIDEO_MODE = 0xF0004
M.VIDEO_COLOR_MODE = 0xF0080
M.VIDEO_FB_BASE = 0xF0084
M.DIAG_ROOT = "ie_diag"
M.NATIVE_DIAG_DIR = M.DIAG_ROOT .. "/native"
M.OVERDRIVE_DIAG_DIR = M.DIAG_ROOT .. "/overdrive"

local function try_require(name)
    package.loaded[name] = nil
    local ok, value = pcall(require, name)
    if ok and type(value) == "table" then return value, name end
    if not ok then sys.print("native_diag", "symbol_require_failed", name, tostring(value)) end
    return nil, nil
end

function M.load_symbols(kind)
    local candidates
    if kind == "overdrive" then
        candidates = {
            "diag_symbols_ie68_overdrive",
            "diag_symbols",
        }
    else
        candidates = {
            "diag_symbols_ie68_overdrive_native",
            "diag_symbols",
        }
    end
    for _, name in ipairs(candidates) do
        sys.print("native_diag", "symbol_require", name)
        local symbols, loaded = try_require(name)
        if symbols then
            sys.print("native_diag", "symbol_loaded", loaded)
            M.symbol_path = loaded
            return symbols
        end
    end
    error("missing diagnostic symbols for " .. tostring(kind), 0)
end

function M.load_target(default_target)
    local target = rawget(_G, "IE_TARGET") or rawget(_G, "TARGET") or default_target
    if target == nil or target == "" then
        error("missing diagnostic target", 0)
    end
    local candidates = { target }
    if target == default_target then
        candidates[#candidates + 1] = "../alienbreed3d2/ab3d2_source/" .. target
        candidates[#candidates + 1] = "ab3d2_source/" .. target
    end
    for _, candidate in ipairs(candidates) do
        sys.print("native_diag", "load", candidate)
        local ok, err = pcall(cpu.load, candidate)
        if ok then
            cpu.start()
            return candidate
        end
        sys.print("native_diag", "load_failed", candidate, tostring(err))
    end
    error("unable to load diagnostic target " .. tostring(target), 0)
end

function M.ensure_target(default_target)
    if cpu.mode and cpu.mode() == "m68k" then
        return rawget(_G, "IE_TARGET") or rawget(_G, "TARGET") or default_target
    end
    return M.load_target(default_target)
end

function M.hex(n)
    if n == nil then return "nil" end
    return string.format("$%08X", n)
end

function M.rd(addr, len)
    return dbg.read_mem(addr, len)
end

function M.rd8(addr)
    local s = M.rd(addr, 1)
    return string.byte(s, 1) or 0
end

function M.rd16(addr)
    local s = M.rd(addr, 2)
    local a, b = string.byte(s, 1, 2)
    return ((a or 0) * 0x100) + (b or 0)
end

function M.rd32(addr)
    local s = M.rd(addr, 4)
    local a, b, c, d = string.byte(s, 1, 4)
    return ((a or 0) * 0x1000000) + ((b or 0) * 0x10000) + ((c or 0) * 0x100) + (d or 0)
end

function M.s16(v)
    if v >= 0x8000 then return v - 0x10000 end
    return v
end

function M.hash_mem(addr, len)
    if addr == nil or addr == 0 or len <= 0 then return 0 end
    local data = M.rd(addr, len)
    local h = 5381
    for i = 1, #data do
        h = (h * 33 + (string.byte(data, i) or 0)) % 0x100000000
    end
    return h
end

function M.scan_fb(label, base, width, height)
    local nonzero = 0
    local minx, miny, maxx, maxy = width, height, -1, -1
    for y = 0, height - 1 do
        local row = M.rd(base + y * width, width)
        for x = 0, width - 1 do
            local v = string.byte(row, x + 1) or 0
            if v ~= 0 then
                nonzero = nonzero + 1
                if x < minx then minx = x end
                if y < miny then miny = y end
                if x > maxx then maxx = x end
                if y > maxy then maxy = y end
            end
        end
    end
    sys.print("fb", label, M.hex(base), "nz", nonzero, "bbox", minx, miny, maxx, maxy)
    return { nonzero = nonzero, minx = minx, miny = miny, maxx = maxx, maxy = maxy }
end

function M.expect(cond, ...)
    if cond then return end
    sys.print("native_diag", "FAIL", ...)
    sys.quit()
    error("native diagnostic failed", 0)
end

function M.range_overlaps(a0, a1, b0, b1)
    return a0 < b1 and b0 < a1
end

function M.expect_no_overlap(name, a0, len, b0, b1)
    local a1 = a0 + len
    M.expect(not M.range_overlaps(a0, a1, b0, b1),
        name, "overlap", M.hex(a0), M.hex(a1), M.hex(b0), M.hex(b1))
end

function M.ensure_dir(path)
    if os and os.execute then os.execute("mkdir -p " .. path) end
end

function M.write_metadata(dir, file, lines)
    if not io or not io.open then
        sys.print("native_diag", "metadata", "stdout", dir .. "/" .. file)
        for _, line in ipairs(lines) do sys.print("native_diag", line) end
        return
    end
    M.ensure_dir(dir)
    local path = dir .. "/" .. file
    local ok, fh = pcall(io.open, path, "w")
    if not ok or not fh then
        sys.print("native_diag", "metadata", "stdout", path)
        for _, line in ipairs(lines) do sys.print("native_diag", line) end
        return
    end
    for _, line in ipairs(lines) do fh:write(line, "\n") end
    fh:close()
    sys.print("native_diag", "metadata", path)
end

local function sorted_keys(t)
    local keys = {}
    if type(t) ~= "table" then return keys end
    for k, _ in pairs(t) do keys[#keys + 1] = tostring(k) end
    table.sort(keys)
    return keys
end

function M.capture_screenshot(dir, file)
    M.ensure_dir(dir)
    local path = dir .. "/" .. file
    if not rec or not rec.screenshot then
        sys.print("native_diag", "screenshot_unavailable", path)
        return false
    end
    local ok, err = pcall(rec.screenshot, path)
    if not ok then
        sys.print("native_diag", "screenshot_failed", path, tostring(err))
        return false
    end
    sys.print("native_diag", "screenshot", path)
    return true
end

function M.save_debug_evidence(dir, prefix, symbols)
    M.ensure_dir(dir)
    dbg.open()

    local regs = dbg.get_regs and dbg.get_regs() or {}
    local lines = {
        "prefix=" .. tostring(prefix),
        "cpu_mode=" .. tostring(cpu.mode()),
        "cpu_running=" .. tostring(cpu.is_running()),
        "pc=" .. M.hex((dbg.get_pc and dbg.get_pc()) or 0),
        "video_mode=" .. M.hex(video.read_reg(M.VIDEO_MODE)),
        "video_color=" .. M.hex(video.read_reg(M.VIDEO_COLOR_MODE)),
        "video_fb=" .. M.hex(video.read_reg(M.VIDEO_FB_BASE)),
        "frame_hash=" .. M.hex(video.frame_hash and video.frame_hash() or 0),
    }

    if video.get_dimensions then
        local w, h = video.get_dimensions()
        lines[#lines + 1] = "video_dimensions=" .. tostring(w) .. "x" .. tostring(h)
    end

    for _, name in ipairs(sorted_keys(regs)) do
        lines[#lines + 1] = "reg." .. name .. "=" .. M.hex(regs[name])
    end

    if symbols then
        local names = {
            "_Vid_FastBufferPtr_l",
            "_Vid_RightX_w",
            "Vid_CentreX_w",
            "Vid_CentreY_w",
            "Vid_BottomY_w",
            "TOTHEMIDDLE",
            "_Draw_LeftClip_w",
            "_Draw_RightClip_w",
            "_Draw_ZoneClipL_w",
            "_Draw_ZoneClipR_w",
            "View2FloorDist",
            "disttobot",
            "top",
            "bottom",
            "minz",
        }
        for _, name in ipairs(names) do
            local addr = symbols[name]
            if addr then
                local value
                if name:match("_l$") or name == "minz" then
                    value = M.rd32(addr)
                else
                    value = M.s16(M.rd16(addr))
                end
                lines[#lines + 1] = "sym." .. name .. "@" .. M.hex(addr) .. "=" .. tostring(value)
            end
        end
    end

    M.write_metadata(dir, prefix .. "_registers_video_symbols.txt", lines)

    if dbg.save_state then
        local ok, err = pcall(dbg.save_state, dir .. "/" .. prefix .. "_state.bin")
        if ok then
            sys.print("native_diag", "state", dir .. "/" .. prefix .. "_state.bin")
        else
            sys.print("native_diag", "state_failed", tostring(err))
        end
    end

    local fb = video.read_reg(M.VIDEO_FB_BASE)
    if dbg.save_mem_file and fb and fb ~= 0 then
        local ok, err = pcall(dbg.save_mem_file, fb, math.min(M.NATIVE_BYTES, 262144), dir .. "/" .. prefix .. "_fb_head.bin")
        if ok then
            sys.print("native_diag", "mem", dir .. "/" .. prefix .. "_fb_head.bin")
        else
            sys.print("native_diag", "mem_failed", tostring(err))
        end
    end

    dbg.close()
end

function M.attach()
    sys.print("native_diag", "attach", "mode", cpu.mode(), "running", tostring(cpu.is_running()))
    if cpu.is_running and cpu.is_running() then
        sys.print("native_diag", "already_running")
        return
    end
    dbg.open()
    sys.print("native_diag", "dbg.open")
    dbg.clear_all_bp()
    if dbg.clear_all_wp then dbg.clear_all_wp() end
    sys.print("native_diag", "dbg.clear")
    dbg.close()
    sys.print("native_diag", "dbg.close")
    if cpu.start and not cpu.is_running() then
        cpu.start()
        sys.print("native_diag", "cpu.start")
    end
end

function M.tap(key)
    if not keys or key == nil then return end
    term.scancode(key)
    sys.wait_ms(40)
    term.scancode(key + 0x80)
    sys.wait_ms(80)
end

function M.drive_default_menu()
    if not keys then return end
    sys.wait_ms(250)
    M.tap(keys.ENTER or keys.RETURN or keys.SPACE)
    M.tap(keys.ENTER or keys.RETURN or keys.SPACE)
    M.tap(keys.SPACE or keys.ENTER or keys.RETURN)
end

return M
