local S = require("diag_symbols")

local M = { S = S }

M.VIDEO_CTRL = 0xF0000
M.VIDEO_MODE = 0xF0004
M.VIDEO_STATUS = 0xF0008
M.VIDEO_PAL_INDEX = 0xF0078
M.VIDEO_PAL_DATA = 0xF007C
M.VIDEO_COLOR_MODE = 0xF0080
M.VIDEO_FB_BASE = 0xF0084
M.MOD_PLAY_PTR = 0xF0BC0
M.MOD_PLAY_LEN = 0xF0BC4
M.MOD_PLAY_CTRL = 0xF0BC8
M.MOD_PLAY_STATUS = 0xF0BCC
M.MOD_POSITION = 0xF0BD4
M.SID_PLAY_PTR = 0xF0E20
M.SID_PLAY_LEN = 0xF0E24
M.SID_PLAY_CTRL = 0xF0E28
M.SID_PLAY_STATUS = 0xF0E2C

M.SCREEN_W = 320
M.SCREEN_H = 240
M.PRESENT_BASE = 0x126000

M.DEV_SKIP_FLATS = 0x00000001
M.DEV_SKIP_SIMPLE_WALLS = 0x00000002
M.DEV_SKIP_SHADED_WALLS = 0x00000004
M.DEV_SKIP_BITMAPS = 0x00000008
M.DEV_SKIP_FASTBUFFER_CLEAR = 0x00000100
M.DEV_SKIP_AI_ATTACK = 0x00000200
M.DEV_SKIP_LIGHTING = 0x00000800
M.DEV_SKIP_PVS_AMEND = 0x00004000
M.DEV_SKIP_EDGE_PVS = 0x00008000
M.DEV_SKIP_OVERLAY = 0x80000000

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

function M.rd32le(addr)
    local s = M.rd(addr, 4)
    local a, b, c, d = string.byte(s, 1, 4)
    return (a or 0) + ((b or 0) * 0x100) + ((c or 0) * 0x10000) + ((d or 0) * 0x1000000)
end

function M.wr8(addr, v)
    dbg.write_mem(addr, string.char(v % 256))
end

function M.wr16(addr, v)
    dbg.write_mem(addr, string.char(math.floor(v / 0x100) % 256, v % 256))
end

function M.wr32(addr, v)
    dbg.write_mem(addr, string.char(
        math.floor(v / 0x1000000) % 256,
        math.floor(v / 0x10000) % 256,
        math.floor(v / 0x100) % 256,
        v % 256))
end

function M.s16(v)
    if v >= 0x8000 then return v - 0x10000 end
    return v
end

function M.cstr(addr, max)
    if not addr or addr == 0 then return "" end
    local s = M.rd(addr, max)
    local z = string.find(s, "\0", 1, true)
    if z then s = string.sub(s, 1, z - 1) end
    return s
end

function M.attach()
    sys.print("diag", "attach", "mode", cpu.mode(), "running", tostring(cpu.is_running()))
    dbg.open()
    dbg.clear_all_bp()
    if dbg.clear_all_wp then dbg.clear_all_wp() end
    dbg.close()
end

function M.tap(key)
    if key == nil then return end
    term.scancode(key)
    sys.wait_ms(40)
    term.scancode(key + 0x80)
    sys.wait_ms(80)
end

function M.drive_default_menu()
    if keys == nil then return end
    sys.wait_ms(250)
    M.tap(keys.ENTER or keys.RETURN or keys.SPACE)
    M.tap(keys.ENTER or keys.RETURN or keys.SPACE)
    M.tap(keys.SPACE or keys.ENTER or keys.RETURN)
end

function M.dump_regs(label)
    local names = {"PC", "SR", "D0", "D1", "D2", "D3", "D4", "D5", "D6", "D7", "A0", "A1", "A2", "A3", "A4", "A5", "A6", "A7", "SP"}
    local out = {}
    for _, name in ipairs(names) do
        local v = dbg.get_reg(name)
        if v ~= nil then out[#out + 1] = name .. "=" .. M.hex(v) end
    end
    sys.print("regs", label, table.concat(out, " "))
end

function M.dump_disasm(label, count)
    local pc = dbg.get_pc()
    local rows = dbg.disasm(pc, count or 10)
    for _, row in ipairs(rows) do
        sys.print("dis", label, M.hex(row.addr), row.hex or "", row.mnemonic or "")
    end
end

function M.dump_paths(label)
    sys.print("path", label, "last", M.cstr(S.io_ie_path_vb, 160))
    if S.io_ie_alt_path_vb then sys.print("path", label, "alt", M.cstr(S.io_ie_alt_path_vb, 164)) end
    if S.io_ie_unpacked_path_vb then sys.print("path", label, "unpacked", M.cstr(S.io_ie_unpacked_path_vb, 180)) end
    if S.io_ie_failed_name_vb then sys.print("path", label, "failed", M.cstr(S.io_ie_failed_name_vb, 160)) end
    if S.io_ie_unpack_path_vb then
        sys.print("unpack", label,
            "path", M.cstr(S.io_ie_unpack_path_vb, 160),
            "block", M.hex(M.rd32(S.io_ie_unpack_block_l)),
            "head", M.hex(M.rd32(S.io_ie_unpack_head_l)),
            "len", M.hex(M.rd32(S.io_ie_unpack_len_l)),
            "stored", M.hex(M.rd32(S.io_ie_unpack_stored_l)))
    end
end

function M.hash_mem(addr, len)
    if not addr or addr == 0 or len <= 0 then return 0 end
    local data = M.rd(addr, len)
    local h = 5381
    for i = 1, #data do
        h = (h * 33 + (string.byte(data, i) or 0)) % 0x100000000
    end
    return h
end

function M.dump_core(label)
    sys.print("core", label,
        "pc", M.hex(dbg.get_pc()),
        "stage", M.hex(M.rd32(S.ie_game_stage)),
        "vbl", S._Vid_VBLCount_l and M.hex(M.rd32(S._Vid_VBLCount_l)) or "na",
        "lastVbl", S.Vid_VBLCountLast_l and M.hex(M.rd32(S.Vid_VBLCountLast_l)) or "na",
        "running", M.hex(M.rd8(S.Game_Running_b)),
        "readctl", M.hex(M.rd8(S.READCONTROLS)),
        "animFrames", S.Anim_FramesToDraw_w and M.hex(M.rd16(S.Anim_FramesToDraw_w)) or "na",
        "flags", M.hex(M.rd32(S._Dev_DebugFlags_l)))
    if S.draw_NumPoints_w then
        sys.print("object", label, "numPoints", M.s16(M.rd16(S.draw_NumPoints_w)))
    end
    sys.print("player", label,
        "x", M.hex(M.rd32(S.Plr1_XOff_l)),
        "y", M.hex(M.rd32(S.Plr1_YOff_l)),
        "z", M.hex(M.rd32(S.Plr1_ZOff_l)),
        "zone", M.s16(M.rd16(S.Plr1_Zone_w)),
        "zonePtr", M.hex(M.rd32(S.Plr1_ZonePtr_l)),
        "health", M.hex(M.rd16(S.Plr1_Health_w)))
    sys.print("video", label,
        "ctrl", M.hex(video.read_reg(M.VIDEO_CTRL)),
        "mode", M.hex(video.read_reg(M.VIDEO_MODE)),
        "status", M.hex(video.read_reg(M.VIDEO_STATUS)),
        "color", M.hex(video.read_reg(M.VIDEO_COLOR_MODE)),
        "fb", M.hex(video.read_reg(M.VIDEO_FB_BASE)),
        "palidx", M.hex(video.read_reg(M.VIDEO_PAL_INDEX)),
        "paldata", M.hex(video.read_reg(M.VIDEO_PAL_DATA)))
end

function M.dump_render_ptrs(label)
    sys.print("render", label,
        "fast", M.hex(M.rd32(S._Vid_FastBufferPtr_l)),
        "draw", M.hex(M.rd32(S._Vid_DrawScreenPtr_l)),
        "display", M.hex(M.rd32(S._Vid_DisplayScreenPtr_l)),
        "maps", M.hex(M.rd32(S.Draw_TextureMapsPtr_l)),
        "texPal", M.hex(M.rd32(S.Draw_TexturePalettePtr_l)),
        "drawPalPtr", M.hex(M.rd32(S.Draw_PalettePtr_l)),
        "drawPalStatic", M.hex(S._draw_Palette_vw))
end

function M.dump_clip_state(label)
    local left = S._Draw_LeftClip_w and M.s16(M.rd16(S._Draw_LeftClip_w)) or 0
    local right = S._Draw_RightClip_w and M.s16(M.rd16(S._Draw_RightClip_w)) or 0
    local zone_left = S._Draw_ZoneClipL_w and M.s16(M.rd16(S._Draw_ZoneClipL_w)) or 0
    local zone_right = S._Draw_ZoneClipR_w and M.s16(M.rd16(S._Draw_ZoneClipR_w)) or 0
    local zone = S._Draw_CurrentZone_w and M.s16(M.rd16(S._Draw_CurrentZone_w)) or -1
    sys.print("clip", label,
        "zone", zone,
        "zoneL", zone_left,
        "zoneR", zone_right,
        "left", left,
        "right", right,
        "valid", tostring(left >= zone_left and right <= zone_right and left < right))
    return left, right, zone_left, zone_right, zone
end

function M.clip_state_invalid()
    if not (S._Draw_LeftClip_w and S._Draw_RightClip_w and S._Draw_ZoneClipL_w and S._Draw_ZoneClipR_w) then
        return false, "missing_symbols"
    end
    local left = M.s16(M.rd16(S._Draw_LeftClip_w))
    local right = M.s16(M.rd16(S._Draw_RightClip_w))
    local zone_left = M.s16(M.rd16(S._Draw_ZoneClipL_w))
    local zone_right = M.s16(M.rd16(S._Draw_ZoneClipR_w))
    if left < zone_left then return true, "left_before_zone" end
    if right > zone_right then return true, "right_after_zone" end
    if left >= right then return true, "empty_or_inverted" end
    return false, "ok"
end

function M.dump_lighting(label)
    local flags = M.rd32(S._Dev_DebugFlags_l)
    sys.print("lighting", label,
        "debugFlags", M.hex(flags),
        "skipLighting", tostring((math.floor(flags / M.DEV_SKIP_LIGHTING) % 2) ~= 0),
        "forceSimple", S._Draw_ForceSimpleWalls_b and M.hex(M.rd8(S._Draw_ForceSimpleWalls_b)) or "na",
        "animLighting", S._Anim_LightingEnabled_b and M.hex(M.rd8(S._Anim_LightingEnabled_b)) or "na",
        "gouraudSelected", S.draw_GouraudFlatsSelected_b and M.hex(M.rd8(S.draw_GouraudFlatsSelected_b)) or "na",
        "gouraudActive", S.draw_UseGouraudFlats_b and M.hex(M.rd8(S.draw_UseGouraudFlats_b)) or "na")
    sys.print("display_state", label,
        "contrast", S._Vid_ContrastAdjust_w and M.hex(M.rd16(S._Vid_ContrastAdjust_w)) or "na",
        "brightness", S._Vid_BrightnessOffset_w and M.hex(M.rd16(S._Vid_BrightnessOffset_w)) or "na",
        "gamma", S._Vid_GammaLevel_b and M.hex(M.rd8(S._Vid_GammaLevel_b)) or "na")
    if S.boxbrights_vw then
        local values = {}
        local same = true
        local first = M.rd16(S.boxbrights_vw)
        for i = 0, 31 do
            local v = M.rd16(S.boxbrights_vw + i * 2)
            values[#values + 1] = string.format("%04X", v)
            if v ~= first then same = false end
        end
        sys.print("brightness", label, "boxbrights.first32", table.concat(values, " "), "allSame", tostring(same))
    end
    if S.draw_BrightnessScaleTable_vw then
        sys.print("brightness", label, "scaleTable", M.hex(S.draw_BrightnessScaleTable_vw),
            "hash64", M.hex(M.hash_mem(S.draw_BrightnessScaleTable_vw, 64)))
    end
end

function M.dump_music(label)
    sys.print("music", label,
        "lvlPtr", S.Lvl_MusicPtr_l and M.hex(M.rd32(S.Lvl_MusicPtr_l)) or "na",
        "mtData", S.mt_data and M.hex(M.rd32(S.mt_data)) or "na",
        "mtSize", S.mt_size and M.hex(M.rd32(S.mt_size)) or "na",
        "reachedEnd", S.reachedend and M.hex(M.rd8(S.reachedend)) or "na",
        "modPtr", M.hex(M.rd32le(M.MOD_PLAY_PTR)),
        "modLen", M.hex(M.rd32le(M.MOD_PLAY_LEN)),
        "modCtrl", M.hex(M.rd32le(M.MOD_PLAY_CTRL)),
        "modStatus", M.hex(M.rd32le(M.MOD_PLAY_STATUS)),
        "modPos", M.hex(M.rd32le(M.MOD_POSITION)),
        "sidPtr", M.hex(M.rd32le(M.SID_PLAY_PTR)),
        "sidLen", M.hex(M.rd32le(M.SID_PLAY_LEN)),
        "sidCtrl", M.hex(M.rd32le(M.SID_PLAY_CTRL)),
        "sidStatus", M.hex(M.rd32le(M.SID_PLAY_STATUS)))
end

function M.scan_fb(label, base, x0, y0, w, h)
    if base == nil or base == 0 then
        sys.print("fb", label, M.hex(base), "null")
        return
    end
    local counts = {}
    local nonzero = 0
    local minx, miny, maxx, maxy = 9999, 9999, -1, -1
    for y = y0, y0 + h - 1 do
        local bytes = M.rd(base + y * M.SCREEN_W + x0, w)
        for x = 0, w - 1 do
            local v = string.byte(bytes, x + 1) or 0
            counts[v] = (counts[v] or 0) + 1
            if v ~= 0 then
                nonzero = nonzero + 1
                if x0 + x < minx then minx = x0 + x end
                if y < miny then miny = y end
                if x0 + x > maxx then maxx = x0 + x end
                if y > maxy then maxy = y end
            end
        end
    end
    local keys_sorted = {}
    for k, _ in pairs(counts) do keys_sorted[#keys_sorted + 1] = k end
    table.sort(keys_sorted, function(a, b) return counts[a] > counts[b] end)
    local top = {}
    for i = 1, math.min(16, #keys_sorted) do
        top[#top + 1] = string.format("%02X:%d", keys_sorted[i], counts[keys_sorted[i]])
    end
    sys.print("fb", label, M.hex(base), "nz", nonzero, "bbox", minx, miny, maxx, maxy, "top", table.concat(top, " "))
end

function M.dump_buffers(label)
    M.scan_fb(label .. ".fast", M.rd32(S._Vid_FastBufferPtr_l), 0, 0, M.SCREEN_W, M.SCREEN_H)
    M.scan_fb(label .. ".draw", M.rd32(S._Vid_DrawScreenPtr_l), 0, 0, M.SCREEN_W, M.SCREEN_H)
    M.scan_fb(label .. ".display", M.rd32(S._Vid_DisplayScreenPtr_l), 0, 0, M.SCREEN_W, M.SCREEN_H)
    M.scan_fb(label .. ".fbreg", video.read_reg(M.VIDEO_FB_BASE), 0, 0, M.SCREEN_W, M.SCREEN_H)
    M.scan_fb(label .. ".present", M.PRESENT_BASE, 0, 0, M.SCREEN_W, M.SCREEN_H)
end

function M.dump_checkpoint(label)
    dbg.open()
    M.dump_core(label)
    M.dump_clip_state(label)
    M.dump_lighting(label)
    M.dump_music(label)
    M.dump_render_ptrs(label)
    M.dump_paths(label)
    M.dump_regs(label)
    M.dump_disasm(label, 8)
end

function M.set_flags(mask)
    dbg.open()
    local old = M.rd32(S._Dev_DebugFlags_l)
    M.wr32(S._Dev_DebugFlags_l, mask)
    sys.print("flags", M.hex(old), "=>", M.hex(M.rd32(S._Dev_DebugFlags_l)))
end

function M.run_until(label, addr)
    sys.print("run_until", label, M.hex(addr))
    dbg.open()
    dbg.clear_all_bp()
    dbg.set_bp(addr)
    if dbg.get_pc() ~= addr then
        dbg.step(1)
    end
    dbg.continue()
    sys.wait_ms(2500)
    dbg.open()
    M.dump_checkpoint(label)
end

function M.read_file(path)
    local f = io.open(path, "rb")
    if not f then return nil end
    local data = f:read("*a")
    f:close()
    return data
end

function M.first_file(paths)
    for _, path in ipairs(paths) do
        local data = M.read_file(path)
        if data then return path, data end
    end
    return nil, nil
end

function M.byte_mismatch(mem_addr, expected, len)
    local got = M.rd(mem_addr, len)
    local mismatches = 0
    local first = -1
    for i = 1, len do
        if string.byte(got, i) ~= string.byte(expected, i) then
            mismatches = mismatches + 1
            if first < 0 then first = i - 1 end
        end
    end
    return mismatches, first, got
end

return M
