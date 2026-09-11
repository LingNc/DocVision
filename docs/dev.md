# 目录结构

> 返回 [README](../README.md)（文档索引见 README「文档」一节）。


```
files/                  源 PDF/Office/图片文件
files/done/             分割完成后归档的源文件（成功 split 后从 files/ 移入）
split_files/            分割后的 PDF/DOCX
mineru_output/          MinerU API 返回的解析结果
output/                 合并后的 Markdown 和引用的图片
output/images/{主题}/   按主题组织的图片
finally/                AI 处理后的最终 Markdown
finally/progress_items/ AI 处理进度记录（断点续传）
logs/                   img2text 处理日志（img2text_*.log + img2text_error_*.log）
finally_latex/{项目名}/    档位2 LaTeX 输出（paths.latex_output）：md + figures/ + images/ + progress_items/ + sessions/
latex_project/{项目名}/    档位1 全书工作区：source/ style/ chapters/ work/ build/ out/ 及 progress.json
fonts/                  AI 字体目录（paths.fonts）：缺字体时按样式会话报告的清单手动放入
```

> `files/done/` 在 SplitAll 模式下自动维护：每次 split 成功的源文件会被 `os.Rename` 到这里；DOCX 直通文件（页数低于阈值）保留在 `files/`，等下次评估。
> `--force` 会在 split 前把 `done/` 中同名的源文件移回 `files/` 再处理。
> 旧的 `*.pdf.done` / `*.docx.done` 标记会在 SplitAll 开始时被迁移到 `done/` 并去掉后缀。
> `os.Rename` 跨文件系统会失败（EXDEV），此时仅打印 warning，不会中断 split；如需跨盘归档请在 `paths.done_dir` 选择同盘路径。
> `logs/` 是 T7 新增目录，专门放 `img2text` 处理期间生成的主日志和错误日志。
> `finally/` 仍保存最终 Markdown 和 `progress_items/` 断点续传记录；
> 分析 / 拆分工具默认从 `logs/` 读取，并回退到 `finally/` 以兼容旧日志。

## CI/CD

推送 `v*.*.*` 标签（或手动 `workflow_dispatch` 指定标签）时，GitHub Actions 自动：

1. 运行测试（`go test -v -race ./...`）
2. 交叉编译 5 个平台二进制
3. 创建 GitHub Release 并上传产物（Release 说明取 `CHANGELOG.md` 中该标签的小节，找不到就退回整份 CHANGELOG 并给 warning）

**带 `-` 的标签发"预发布"（Pre-release），不带 `-` 的标签发正式版**：两者都会真正构建并附上 5 个平台产物，区别只在 Release 的标记——预发布永远不会被标成 "Latest"，正式版则显式标 Latest（GitHub 默认把**最后发布**的那个标成 Latest，曾因此让 `v1.0.1` 抢了 `v1.1.0` 的位置）。

```bash
git tag v1.5.0
git push origin v1.5.0

# 老标签补发（比如打标签时工作流还不会发预发布）：用当前分支的工作流逻辑补跑
gh workflow run release.yml -f tag=v1.5.0-beta.4
```

> 只推 `master`（不打标签）**不会**触发任何构建——工作流的触发条件是标签推送。所以"1.1 之后没有任何 Release"通常是标签没推上去，而不是构建失败（可用 `git ls-remote --tags origin` 看远端到底有哪些标签）。

## Python 脚本独立使用

历史 Python 脚本已归档至 `legacy/python/`，独立使用方式请参阅该目录下的 `README.md`。
