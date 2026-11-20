#!/usr/bin/env python3
# luadraw-expand.py —— luadraw 两阶段预编译的独立脚本版（用户 11-20 简化要求）。
#
# 用法：python3 luadraw-expand.py <input.tex> [files_dir] [lua_bin]
#   files_dir: luadraw 宏包目录（含 *.lua 模块），默认 <脚本上级目录>/../files
#   lua_bin:   lua5.4/lua 可执行文件，默认 PATH 探测（可用 LUADRAW_LUA 指定）
#
# 行为：抽取 \begin{luadraw}{name=...,...}...\end{luadraw} 块 → 用 plain lua 跑四垫片
# 运行器（等价 spike 方案，SK-I16 实证）→ 每块产出 <name>.tkz → 原块替换为
# \input{<name>.tkz}。之后用 pdflatex/xelatex/tectonic 正常编译即可——无需 lualatex。
# 幂等：已替换为 \input{*.tkz} 的文件再跑无变化。
import os, re, shutil, subprocess, sys, tempfile

LUADRAW_ENV_RE = re.compile(
    r"\\begin\{luadraw\}(\{[^{}]*\})?(.*?)\\end\{luadraw\}", re.S)

RUNNER = r'''-- luadraw 预编译运行器（luadraw-expand.py 生成；四垫片方案与内核 precompile.go 同源）
local FILES, OUTDIR, MANIFEST = ...
package.path = FILES .. "/?.lua;" .. FILES .. "/extensions/?.lua;" .. package.path
table.append = table.append or function(t, ...)
    for _, v in ipairs({...}) do
        if type(v) == "table" then for _, e in ipairs(v) do t[#t+1] = e end
        else t[#t+1] = v end
    end
    return t
end
table.copy = table.copy or function(t) local r = {} for k, v in pairs(t) do r[k] = v end return r end
local _require = require
require = function(name)
    if type(name) == "string" then name = name:gsub("%.lua$", "") end
    return _require(name)
end
tex = tex or { sprint = function(...) end, print = print }
luadraw = { cachedir = OUTDIR }
luadraw.graph = require 'luadraw_graph2d'
function luadraw.graph:Show() self:Savetofile(luadraw.graph_name) end
function luadraw.graph:Save() self:Savetofile(luadraw.graph_name) end
luadraw.graph3d = require 'luadraw_graph3d'
require 'luadraw_frustum_and_co'
require 'luadraw_central_perspective'
ld = luadraw
Z = luadraw.cpx.Z
M = luadraw.pt3d.M
pt3d = luadraw.pt3d
local n_fail = 0
for line in io.lines(MANIFEST) do
    local idx, name = line:match("^(%S+)%s+(%S+)$")
    if idx then
        local fh = io.open(("%s/block_%s.lua"):format(MANIFEST:match("^(.*)/[^/]*$") or ".", idx), "r")
        local code = fh:read("a"); fh:close()
        luadraw.graph_name = OUTDIR .. "/" .. name .. ".tkz"
        local chunk, cerr = load(code, "=block_" .. idx)
        if chunk then
            local ok, rerr = pcall(chunk)
            if not ok then n_fail = n_fail + 1; io.stderr:write("block_"..idx.." 运行失败: "..tostring(rerr).."\n") end
        else
            n_fail = n_fail + 1; io.stderr:write("block_"..idx.." 语法错误: "..tostring(cerr).."\n")
        end
    end
end
os.exit(n_fail > 0 and 1 or 0)
'''

def main():
    if len(sys.argv) < 2:
        sys.exit("usage: luadraw-expand.py <input.tex> [files_dir] [lua_bin]")
    tex_path = sys.argv[1]
    files_dir = sys.argv[2] if len(sys.argv) > 2 else os.path.join(
        os.path.dirname(os.path.abspath(__file__)), "..", "files")
    lua_bin = sys.argv[3] if len(sys.argv) > 3 else (os.environ.get("LUADRAW_LUA") or
               shutil.which("lua5.4") or shutil.which("lua") or "")
    if not lua_bin:
        sys.exit("无 lua 解释器（装 lua5.4 或用 LUADRAW_LUA 指定路径）")
    if not os.path.isdir(files_dir):
        sys.exit("luadraw 宏包目录缺失: %s" % files_dir)
    src = open(tex_path, encoding="utf-8").read()
    if "luadraw-precompiled" in src:
        print("已预编译（luadraw-precompiled 标记在），无变化")
        return
    blocks = LUADRAW_ENV_RE.findall(src)
    if not blocks:
        print("无 luadraw 环境块，无变化")
        return
    outdir = os.path.dirname(os.path.abspath(tex_path))
    work = tempfile.mkdtemp(prefix="luadraw-pre-")
    manifest = []
    n = 0
    def repl(mo):
        nonlocal n
        n += 1
        name = "luadraw_%d" % n
        nm = re.search(r"name=([^,}\s]+)", mo.group(1) or "")
        if nm:
            name = nm.group(1)
        open(os.path.join(work, "block_%d.lua" % n), "w", encoding="utf-8").write(mo.group(2))
        manifest.append("%d\t%s" % (n, name))
        return "%% luadraw-precompiled\n\\input{%s.tkz}" % name
    new_src = LUADRAW_ENV_RE.sub(repl, src)
    open(os.path.join(work, "manifest.lst"), "w").write("\n".join(manifest) + "\n")
    runner_path = os.path.join(work, "runner.lua")
    open(runner_path, "w", encoding="utf-8").write(RUNNER)
    r = subprocess.run([lua_bin, runner_path, files_dir, outdir,
                        os.path.join(work, "manifest.lst")],
                       capture_output=True, text=True)
    if r.returncode != 0:
        sys.exit("luadraw 预编译失败:\n" + r.stdout[-800:] + r.stderr[-800:])
    open(tex_path, "w", encoding="utf-8").write(new_src)
    shutil.rmtree(work, ignore_errors=True)
    print("OK：%d 个 luadraw 块 → %s/*.tkz，%s 已改写为 \\input（之后 xelatex/pdflatex/tectonic 正常编译）"
          % (n, outdir, os.path.basename(tex_path)))

if __name__ == "__main__":
    main()
