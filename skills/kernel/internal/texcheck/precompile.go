// precompile.go：luadraw 两段式预编译（SK-I16）。
//
// 原理：luadraw 的 Lua 侧是纯 Lua（对 TeX 的耦合仅 Show()→Sendtotex() 一处 tex.sprint），
// 用 plain lua5.4 跑绘图代码、Savetofile() 直接落 TikZ 文件；改写 tex 把环境块换成
// \input{*.tkz}，之后任何引擎（pdflatex/xelatex/tectonic）都能编译——无 lualatex 机器的
// 备选路径。spike 实证：docs/luadraw/spike/README.md（官方 3D 示例产出完整 tikzpicture）。
//
// 垫片方案与 lingnc-memory/toolchain/luadraw/bin/luadraw2tikz.lua 对齐（那边的
// complex 压力测试——3502 行/267KB 产物过 tectonic——验证过边界）：
//  ① table.append  单层合并语义（Dboxaxes3d 依赖；误写成逐参数追加会炸）
//  ② table.copy    浅拷贝（LuaTeX 预载扩展）
//  ③ require 后缀剥离（包内 require 'xxx.lua' 写法；调用点归一，天然无双加载）
//  ④ tex 空桩 + Show/Save 重定向 Savetofile（绕开唯一 tex.sprint 耦合点）
package texcheck

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// LuaAvailable 返回可用的 lua 解释器路径，找不到返回 ""。
// 探测链：环境变量 LUADRAW_LUA（部署特化的标准入口，各工作区路径不同不入库）
// → PATH(lua5.4/lua)。持久工具链等特化环境一律经 LUADRAW_LUA 注入。
func LuaAvailable() string {
	if p := os.Getenv("LUADRAW_LUA"); p != "" {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	for _, b := range []string{"lua5.4", "lua"} {
		if p, err := exec.LookPath(b); err == nil {
			return p
		}
	}
	return ""
}

// luadrawEnvRe 匹配 \begin{luadraw}{name=...,...} ... \end{luadraw}（跨行，非贪婪）。
var luadrawEnvRe = regexp.MustCompile(`(?s)\\begin\{luadraw\}(\{[^{}]*\})?(.*?)\\end\{luadraw\}`)

// nameOptRe 从环境选项串里取 name= 值。
var nameOptRe = regexp.MustCompile(`name=([^,}\s]+)`)

// precompiledMarker 改写标记：幂等依据（已预编译的文件不再处理）。
const precompiledMarker = "% luadraw-precompiled"

// precompileRunnerLua 独立运行器模板。占位：__FILES__（luadraw 模块目录）、__OUTDIR__（产物目录）。
// 用法：lua runner.lua manifest.lst；manifest 每行 "序号\t块名"，块代码在同目录 block_<序号>.lua。
const precompileRunnerLua = `-- luadraw 预编译运行器（内核生成；垫片见 precompile.go 头注）
local FILES = "__FILES__"
local OUTDIR = "__OUTDIR__"
local MANIFEST = assert(arg and arg[1], "usage: lua runner.lua manifest.lst")

package.path = FILES .. "/?.lua;" .. FILES .. "/extensions/?.lua;" .. package.path

-- 垫片①table.append：LuaTeX 扩展，单层合并语义
table.append = table.append or function(t, ...)
    for _, v in ipairs({...}) do
        if type(v) == "table" then
            for _, e in ipairs(v) do t[#t+1] = e end
        else
            t[#t+1] = v
        end
    end
    return t
end
-- 垫片②table.copy：LuaTeX 扩展，浅拷贝
table.copy = table.copy or function(t)
    local r = {}
    for k, v in pairs(t) do r[k] = v end
    return r
end
-- 垫片③require 后缀剥离（调用点归一，天然无双名加载）
local _require = require
require = function(name)
    if type(name) == "string" then name = name:gsub("%.lua$", "") end
    return _require(name)
end
-- 垫片④tex 空桩：Show 不走 Sendtotex，纯防御
tex = tex or { sprint = function(...) end, print = print }

-- 环境组装（等价 luadraw.sty；graph3d 恒载 = 包选项 [3d]）
luadraw = { cachedir = OUTDIR }
luadraw.graph = require 'luadraw_graph2d'
function luadraw.graph:Show() self:Savetofile(luadraw.graph_name) end
function luadraw.graph:Save() self:Savetofile(luadraw.graph_name) end
luadraw.graph3d = require 'luadraw_graph3d'
require 'luadraw_frustum_and_co'
require 'luadraw_central_perspective'
-- 常用别名（doc \shortcuts 等价物）
ld = luadraw
Z = luadraw.cpx.Z
M = luadraw.pt3d.M
pt3d = luadraw.pt3d

local blocks_dir = MANIFEST:match("^(.*)/[^/]*$") or "."
local n_fail = 0
for line in io.lines(MANIFEST) do
    local idx, name = line:match("^(%S+)%s+(%S+)$")
    if idx then
        local fh = io.open(blocks_dir .. "/block_" .. idx .. ".lua", "r")
        local code = fh:read("a")
        fh:close()
        luadraw.graph_name = OUTDIR .. "/" .. name .. ".tkz"
        local chunk, cerr = load(code, "=block_" .. idx)
        if chunk then
            local ok, rerr = pcall(chunk)
            if not ok then n_fail = n_fail + 1; io.stderr:write("block_" .. idx .. " 运行失败: " .. tostring(rerr) .. "\n") end
        else
            n_fail = n_fail + 1; io.stderr:write("block_" .. idx .. " 语法错误: " .. tostring(cerr) .. "\n")
        end
    end
end
os.exit(n_fail > 0 and 1 or 0)
`

// PrecompileLuaDraw 把 dir 下所有含 luadraw 环境的 .tex 预编译：
// 每个环境块 → lua 运行 → <name>.tkz → 块原位替换为 \input{<name>.tkz} + 幂等标记。
// filesDir = luadraw 模块目录（skills/docs/luadraw/files）。返回 (改写文件数, 产物数, error)。
func PrecompileLuaDraw(dir, mainFile, filesDir string) (int, int, error) {
	lua := LuaAvailable()
	if lua == "" {
		return 0, 0, fmt.Errorf("无 lua 解释器（需 lua5.4 或 lua；可用 LUADRAW_LUA 指定路径）")
	}
	if _, err := os.Stat(filesDir); err != nil {
		return 0, 0, fmt.Errorf("luadraw 模块目录缺失: %s（安装包应自带 docs/luadraw/files）", filesDir)
	}
	matches, _ := filepath.Glob(filepath.Join(dir, "*.tex"))
	changed := 0
	produced := 0
	workDir := filepath.Join(dir, ".luadraw-pre")
	_ = os.RemoveAll(workDir)
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return 0, 0, err
	}
	defer os.RemoveAll(workDir)

	type rewrite struct {
		path string
		data string
	}
	var rewrites []rewrite
	for _, f := range matches {
		data, err := os.ReadFile(f)
		if err != nil || !luadrawEnvRe.Match(data) || strings.Contains(string(data), precompiledMarker) {
			continue
		}
		var manifest []string
		n := 0
		newData := luadrawEnvRe.ReplaceAllStringFunc(string(data), func(m string) string {
			n++
			name := fmt.Sprintf("luadraw_%d", n)
			if sm := luadrawEnvRe.FindStringSubmatch(m); sm != nil && sm[1] != "" {
				if nm := nameOptRe.FindStringSubmatch(sm[1]); nm != nil {
					name = nm[1]
				}
			}
			code := luadrawEnvRe.FindStringSubmatch(m)[2]
			code = strings.ReplaceAll(code, `\shortcuts`, "") // 别名已由运行器注入
			_ = os.WriteFile(filepath.Join(workDir, fmt.Sprintf("block_%d.lua", n)), []byte(code), 0o644)
			manifest = append(manifest, fmt.Sprintf("%d\t%s", n, name))
			return fmt.Sprintf("%s\n\\input{%s.tkz}\n%% %s", precompiledMarker, name, m)
		})
		if n == 0 {
			continue
		}
		_ = os.WriteFile(filepath.Join(workDir, "manifest.lst"), []byte(strings.Join(manifest, "\n")+"\n"), 0o644)
		runner := strings.ReplaceAll(precompileRunnerLua, "__FILES__", filesDir)
		runner = strings.ReplaceAll(runner, "__OUTDIR__", dir)
		runnerPath := filepath.Join(workDir, "runner.lua")
		_ = os.WriteFile(runnerPath, []byte(runner), 0o644)
		cmd := exec.Command(lua, runnerPath, filepath.Join(workDir, "manifest.lst"))
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			return changed, produced, fmt.Errorf("luadraw 预编译运行失败: %v\n%.600s", err, out)
		}
		rewrites = append(rewrites, rewrite{f, newData})
		changed++
		produced += n
	}
	for _, r := range rewrites {
		if err := os.WriteFile(r.path, []byte(r.data), 0o644); err != nil {
			return changed, produced, err
		}
	}
	return changed, produced, nil
}

// DecideEngine 双路探测（SK-I16）：章内含 luadraw 时——
// 有 lualatex → 强制 lualatex（主路径）；无 lualatex 有 lua → 预编译后走正常链（回退路径）；
// 都没有 → 报错指引。返回 (engine, 是否走了预编译, error)。engine 空 = 正常回退链。
func DecideEngine(dir, mainFile, filesDir string) (string, bool, error) {
	if !NeedsLuaLaTeX(dir, mainFile) {
		return "", false, nil
	}
	if _, err := exec.LookPath("lualatex"); err == nil {
		return "lualatex", false, nil
	}
	if LuaAvailable() != "" {
		changed, _, err := PrecompileLuaDraw(dir, mainFile, filesDir)
		if err != nil {
			return "", false, err
		}
		if changed > 0 && NeedsLuaLaTeX(dir, mainFile) {
			return "", false, fmt.Errorf("预编译后仍存在 luadraw 环境（幂等标记缺失？）")
		}
		return "", true, nil // 走正常链
	}
	return "", false, fmt.Errorf("本章用了 luadraw，但既无 lualatex 也无 lua5.4——二选一安装：texlive-lualatex（直编）或 lua5.4（预编译回退）")
}
