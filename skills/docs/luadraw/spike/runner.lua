-- luadraw standalone spike runner（SK-I16 两段式预编译实证）
-- 要点：①自定义 searcher 按"去后缀基名"查 package.loaded（治包内 require 带不带 .lua
--        后缀的双名加载——类表/元表会分裂）；②Show() 重定向 Savetofile（绕开唯一
--        tex.sprint 耦合点）；③tex 空桩防御。
local FILES = "/home/kimi/dv2/skills/docs/luadraw/files"
table.insert(package.searchers, function(name)
    local base = name:sub(-4) == ".lua" and name:sub(1, -5) or name
    if package.loaded[base] ~= nil then
        return function() return package.loaded[base] end
    end
    if name:sub(-4) == ".lua" then
        local p = FILES .. "/" .. name
        local f = io.open(p, "r")
        if f then f:close()
            return function()
                local m = loadfile(p)()
                package.loaded[base] = m  -- 双名归一：基名也登记，后到的无后缀 require 命中缓存
                return m
            end
        end
    end
end)
package.path = FILES .. "/?.lua;" .. package.path
tex = { sprint = function() end, write = function() end }
-- lualibs 垫片（LuaLaTeX 预载 lualibs 提供的全局工具，standalone 需自备；按需增补）
function table.copy(t) local u = {} for k, v in pairs(t) do u[k] = v end return u end
luadraw = { cachedir = "." }
require 'luadraw_base'  -- 先加载基座：table.copy 等全局工具（LuaTeX 下 .sty 亦如此）
luadraw.graph = require 'luadraw_graph2d'
luadraw.graph_name = "/tmp/spike.tkz"
function luadraw.graph:Show() self:Savetofile(luadraw.graph_name) end
luadraw.graph3d = require 'luadraw_graph3d'
require 'luadraw_frustum_and_co'
require 'luadraw_central_perspective'
local ld = luadraw  -- cpx=calc.lua:13 自赋；pt3d=matrix3d.lua:13 自赋
local Z = ld.cpx.Z
local M = ld.pt3d.M
local pt3d = ld.pt3d  -- 示例体里用到 pt3d.toPoint3d


local g = ld.graph3d:new{}
local P = ld.polyreg(0, Z(2,3), 5)
g:Dpolyline(P, true, "red,line width=1.2pt")
local Q = ld.map(pt3d.toPoint3d, P)
P = ld.pyramid(Q, M(0,0,4))
g:Dpoly(P, {color="blue", opacity=0.6})
g:Show()

