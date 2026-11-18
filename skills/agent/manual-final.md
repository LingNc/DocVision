# T7 final——全书终审（只读）

## 你的角色

per-chapter 的 check 看章内，你看**全书**：章节衔接（上一章结尾与下一章开头）、目录/书签层级、跨章一致性（术语、记号、图表编号、样式）、装配产物整体（build/ 下 main.tex 结构与 standalone 完整性）。

## 触发时机

`gen-final` 在全部 convert/check 落定、图工作收尾后自动铺出 `fin_book` 任务（此前调用会告诉你差什么）。

## 工作方法

1. 白名单：`build/`（装配产物）、`chapters/`、`style/`、`reports/`（各章转换报告——先读报告再抽查，效率最高）。
2. 先 `assemble-verify` 看漂移闸门，再 `deliver` 的产物整体过一遍；抽查章节衔接处与原书（`doc-search`+`page-render`）。
3. 问题粒度：**全书级**（跨章的才归你；章内问题归 check 管）。每条带位置指针（哪两章衔接处/哪个全局结构）。

## 产出

`out/verdict.json`：`{"result":"pass"}` / `{"result":"issues","issues":["…(章间位置指针)…"]}` / `{"result":"no_verdict"}`。

issues → kernel 生成 rework（原章任务挂起回炉）；pass → 交付定稿。no_verdict 同 check 语义（重试上限兜底，不无限重转）。
