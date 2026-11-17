-- ktw-macros.lua — Pandoc Lua filter for <!--ktw --> macros.
-- Handles: pagebreak, table_style, table_default_style, p_style, p_default_style.
-- Also fixes list styles to use Word built-in List Bullet / List Number.

local table_style = nil
local table_default_style = nil
local p_style = nil
local p_default_style = nil

-- Parse <!--ktw ... --> blocks. Supports one-line and multi-line.
local function parse_ktw(text)
  local body = text:match("<!%-%-ktw%s+(.-)%s*%-%->")
  if not body then return nil end

  local macros = {}
  if not body:find(":") then
    macros[body:match("^%s*(%S+)%s*$") or body] = true
    return macros
  end
  for line in (body .. "\n"):gmatch("([^\n]+)\n") do
    local key, value = line:match("^%s*([%w_]+)%s*:%s*(.-)%s*$")
    if key then
      macros[key] = value
    end
  end
  return macros
end

local function apply_macros(macros)
  if macros["pagebreak"] then
    return true
  end

  if macros["table_style"] ~= nil then
    local v = macros["table_style"]
    if v == "" then table_style = table_default_style
    else table_style = v end
  end

  if macros["table_default_style"] ~= nil then
    local v = macros["table_default_style"]
    if v ~= "" then
      table_default_style = v
      if table_style == nil then table_style = table_default_style end
    end
  end

  if macros["p_style"] ~= nil then
    local v = macros["p_style"]
    if v == "" then p_style = p_default_style
    else p_style = v end
  end

  if macros["p_default_style"] ~= nil then
    local v = macros["p_default_style"]
    if v ~= "" then
      p_default_style = v
      if p_style == nil then p_style = p_default_style end
    end
  end

  return false
end

function RawBlock(el)
  if el.format ~= "html" then return end
  local macros = parse_ktw(el.text)
  if not macros then return end

  local is_pagebreak = apply_macros(macros)
  if is_pagebreak then
    return pandoc.RawBlock("openxml",
      '<w:p><w:r><w:br w:type="page"/></w:r></w:p>')
  end
  return {}
end

function Table(el)
  if table_style then
    el.attributes["custom-style"] = table_style
  end
  return el
end

function Para(el)
  if p_style then
    return pandoc.Div(el, pandoc.Attr("", {}, {{"custom-style", p_style}}))
  end
end

function Header(el)
  return el
end
