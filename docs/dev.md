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

推送 `v*.*.*` 标签时，GitHub Actions 自动：

1. 运行测试（`go test -v -race ./...`）
2. 交叉编译 5 个平台二进制
3. 创建 GitHub Release 并上传产物（Release 说明取 `CHANGELOG.md` 中该标签的小节）

> 带 `-` 的标签（如 `v1.5.0-beta.3`）属于预发布，工作流会跳过发布任务，不会创建 Release；需要发布时用不带 `-` 的版本号。

```bash
git tag v1.5.0
git push origin v1.5.0
```

## Python 脚本独立使用

历史 Python 脚本已归档至 `legacy/python/`，独立使用方式请参阅该目录下的 `README.md`。
