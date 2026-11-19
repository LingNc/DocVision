#!/usr/bin/env python3
# 从内核源码抽取 CLI 命令签名与 MCP 工具表，生成/刷新 docs/10-command-reference.md 的数据区。
# 用法：python3 extract-tools.py <仓库根>   （在仓库根跑；改代码后重跑即得新签名表）
import json, re, subprocess, sys

root = sys.argv[1] if len(sys.argv) > 1 else "."
K = f"{root}/skills/kernel"

# ---- CLI：顶层 case 的用法串 ----
m = open(f"{K}/cmd/dvagent/main.go").read()
case_pat = re.compile(r'\n\tcase "([a-z0-9_-]+)"(?:, "([a-z0-9_-]+)")?:')
ms = list(case_pat.finditer(m))
rows = []
for idx, c in enumerate(ms):
    name = c.group(1) + ("," + c.group(2) if c.group(2) else "")
    end = ms[idx+1].start() if idx+1 < len(ms) else len(m)
    um = re.search(r'die\(fmt\.Errorf\("([^"]*需要[^"]*)"', m[c.start():end])
    rows.append({"name": name, "usage": um.group(1) if um else ""})

# ---- MCP：注册表花括号平衡 + 工具块解析（名字类含连字符）----
mc = open(f"{K}/cmd/dvagent/mcp.go").read()
decl = mc.index("var mcpTools = []mcpTool{")
br = mc.index("{", decl)
depth, j = 0, br
while True:
    if mc[j] == '{': depth += 1
    elif mc[j] == '}':
        depth -= 1
        if depth == 0: break
    j += 1
arr = mc[br+1:j]
blocks, d, beg = [], 0, 0
for k in range(len(arr)):
    if arr[k] == '{':
        if d == 0: beg = k
        d += 1
    elif arr[k] == '}':
        d -= 1
        if d == 0: blocks.append(arr[beg:k+1])
pat = re.compile(r'\{"([a-z0-9_-]+)",\s*"((?:[^"\\]|\\.)*)"')
mcp = []
for b in blocks:
    nm = pat.search(b)
    if not nm: continue
    req = re.search(r'\[\]string\{([^}]*)\}', b)
    params = re.findall(r'"(\w+)":\s*strProp', b)
    mcp.append({"name": nm.group(1), "desc": nm.group(2).replace("\\n", " "),
                "params": params,
                "required": [x.strip().strip('"') for x in req.group(1).split(",")] if req else []})

print(json.dumps({"cli_cases": len(rows), "mcp_tools": len(mcp)}, ensure_ascii=False))
for r in rows: print("CLI", r["name"], "|", r["usage"][:60])
for t in mcp: print("MCP", t["name"], t["params"], t["required"])
