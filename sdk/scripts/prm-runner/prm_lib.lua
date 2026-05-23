-- Helpers shared between prm_runner.ies and generated iemon wrappers.
-- Sandbox-aware: no io.*, no os.*, no dofile. Everything goes through
-- the registered sys/dbg/term Lua surface.

local M = {}

-- flatten converts a {text,color} row table from dbg.command_output into
-- a plain-text string. Safe to call with non-table input (returns empty).
function M.flatten(rows)
  if type(rows) ~= "table" then return tostring(rows or "") end
  local out = {}
  for _, r in ipairs(rows) do out[#out+1] = r.text or "" end
  return table.concat(out, "\n")
end

-- JSON encoder. Gopher-lua's tight register budget made earlier recursive
-- encoders overflow on 24-case reports. This one is schema-aware
-- (cases/steps/expected/actual) and fully iterative — it recurses one
-- level only, into the flat string arrays.

local function escape_string(s)
  s = s:gsub("\\", "\\\\")
  s = s:gsub("\"", "\\\"")
  s = s:gsub("\n", "\\n")
  s = s:gsub("\r", "\\r")
  s = s:gsub("\t", "\\t")
  s = s:gsub("[%z\1-\31]", function(c)
    return string.format("\\u%04X", string.byte(c))
  end)
  return s
end

local function quote(s)
  if s == nil then return "null" end
  return "\"" .. escape_string(tostring(s)) .. "\""
end

local function string_array(arr)
  if arr == nil then return "[]" end
  local out = {"["}
  for i, s in ipairs(arr) do
    if i > 1 then out[#out+1] = "," end
    out[#out+1] = quote(s)
  end
  out[#out+1] = "]"
  return table.concat(out)
end

local function encode_step(s)
  local parts = {"{"}
  local function kv(k, v)
    if #parts > 1 then parts[#parts+1] = "," end
    parts[#parts+1] = "\"" .. k .. "\":" .. v
  end
  if s.input ~= nil       then kv("input",       quote(s.input))       end
  if s.cmd ~= nil         then kv("cmd",         quote(s.cmd))         end
  if s.kind ~= nil        then kv("kind",        quote(s.kind))        end
  if s.status ~= nil      then kv("status",      quote(s.status))      end
  if s.skip_reason ~= nil then kv("skip_reason", quote(s.skip_reason)) end
  kv("expected", string_array(s.expected))
  kv("actual",   string_array(s.actual))
  parts[#parts+1] = "}"
  return table.concat(parts)
end

local function encode_steps(steps)
  if steps == nil then return "[]" end
  local out = {"["}
  for i, s in ipairs(steps) do
    if i > 1 then out[#out+1] = "," end
    out[#out+1] = encode_step(s)
  end
  out[#out+1] = "]"
  return table.concat(out)
end

local function encode_api_lint(l)
  if l == nil then return "null" end
  local parts = {"{\"status\":", quote(l.status)}
  if l.unknown ~= nil then
    parts[#parts+1] = ",\"unknown\":"
    parts[#parts+1] = string_array(l.unknown)
  end
  parts[#parts+1] = "}"
  return table.concat(parts)
end

local function encode_case(c)
  local parts = {"{"}
  local function kv(k, v)
    if #parts > 1 then parts[#parts+1] = "," end
    parts[#parts+1] = "\"" .. k .. "\":" .. v
  end
  kv("id",     quote(c.id))
  kv("source", quote(c.source))
  kv("fence_start_line", tostring(c.fence_start_line or 0))
  kv("kind",   quote(c.kind))
  kv("status", quote(c.status))
  if c.cpu ~= nil          then kv("cpu",          quote(c.cpu)) end
  if c.skip_reason ~= nil  then kv("skip_reason",  quote(c.skip_reason)) end
  if c.monitor_dump ~= nil then kv("monitor_dump", quote(c.monitor_dump)) end
  if c.error ~= nil        then kv("error",        quote(c.error)) end
  if c.api_lint ~= nil     then kv("api_lint",     encode_api_lint(c.api_lint)) end
  kv("steps", encode_steps(c.steps))
  parts[#parts+1] = "}"
  return table.concat(parts)
end

-- encode_report serializes a {cases={...}} report. Iterative; no Lua
-- stack growth beyond the schema's natural shape.
function M.encode_report(report)
  local parts = {"{\"cases\":["}
  if report.cases then
    for i, c in ipairs(report.cases) do
      if i > 1 then parts[#parts+1] = "," end
      parts[#parts+1] = encode_case(c)
    end
  end
  parts[#parts+1] = "]}"
  return table.concat(parts)
end

-- normalize strips trailing whitespace per line and any trailing blank
-- lines, matching the extractor's hash-normalize but also forgiving
-- trailing CRs (BASIC emits \r\n on the wire).
function M.normalize(s)
  if s == nil then return "" end
  s = s:gsub("\r", "")
  local lines = {}
  for line in (s .. "\n"):gmatch("([^\n]*)\n") do
    lines[#lines+1] = (line:gsub("[ \t]+$", ""))
  end
  while #lines > 0 and lines[#lines] == "" do
    lines[#lines] = nil
  end
  return table.concat(lines, "\n")
end

-- splitlines mirrors normalize but returns a table of lines (for diffing).
function M.splitlines(s)
  s = M.normalize(s)
  if s == "" then return {} end
  local out = {}
  for line in (s .. "\n"):gmatch("([^\n]*)\n") do
    out[#out+1] = line
  end
  return out
end

return M
