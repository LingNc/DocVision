# Figure Compare 前瞻原型

本目录维护可复用的图形比较算法与独立会话审阅 viewer。它是研发原型，不等同于 DocVision 正式功能；不含真实项目图片、转录或本地路径。

## 目录

- `algorithms/`：legacy 比较器、新版视觉证据评估器、容错指标、校准工具、judge 合约及会话停止策略。
- `tests/`：合成图像和结构化判定测试，作为源码纳入 Git。
- `runner/`：Go 单会话 harness，提供 `view_pdf` 候选预览与 `compare_figure` 比较；比较回执含最差区域坐标，可按区域放大复查。
- `compare_figure_skill/`：可独立复制/导出的 compare_figure Skill，包含完整算法、动态阈值和离线 viewer。
- `viewer/`：会话 JSONL 浏览器，可按工具调用回退转录并在确认后备份原文件。
- `docs/`：设计边界与使用说明。

## 快速使用

Python 依赖：NumPy、Pillow。执行测试：

```bash
python3 -m pip install numpy pillow
PYTHONPATH=algorithms python3 -m unittest discover -s tests -v
```

运行新版图像评估：

```bash
python3 algorithms/compare_vnext.py evaluate --help
```

使用 viewer：浏览器打开 `viewer/viewer.html`，点击“选择工作目录”选择包含多个运行子目录的根目录（例如 figure_compare_runs）。viewer 会在浏览器中递归读取并列出所有 `session.jsonl`，选择某个会话后加载其对应的 `media/` 与 `compare/round_*` 缩略图；点击“刷新”可重新读取新增或更新的会话。不会上传整个目录。

运行会话级 Go harness（需在仓库模块上下文中构建，参数详见 `runner/main.go`）：

```bash
cd figure_compare/runner
go run . -config ../../path/to/config.yaml -original ../../path/to/reference.png \\
  -compare-script ../algorithms/compare.py -out ../../path/to/run \\
  -transcript ../../path/to/run/session.jsonl
```

调用本地比较 Skill：

```bash
cd figure_compare/compare_figure_skill
python3 compare.py --original <reference.png> --pdf <candidate.pdf> --out <result-dir> \\
  --threshold-policy adaptive-v1 --difficulty 0.65 --confidence 0.8
```

默认不启用动态阈值，仍使用 50% 固定门槛；当前版本还修复了稀疏线稿粗网格锁错和非对称白边的平移坐标问题，旧分数定义保持不变。Skill 的独立使用、依赖和输出契约见 [Skill 说明](compare_figure_skill/SKILL.md)。可只打包该目录供其他 Agent 使用：

```bash
cd figure_compare/compare_figure_skill
tar -czf /tmp/compare_figure_skill.tar.gz compare.py SKILL.md viewer.html
```

在浏览器打开 `compare_figure_skill/viewer.html`，手动选择 `metrics.json` 与同一输出目录中的 PNG 文件，即可离线查看。

程序会在指定输出目录产生 `session.jsonl`、运行日志和工作产物；打开 `viewer/viewer.html` 后选择包含多个运行子目录的根工作目录，再从会话列表中选择目标会话查看。配置、原图、输出路径均由命令行传入，不在源码中写死。若在 viewer 中从某次工具调用处截断会话，先停止原 runner，再使用相同的 `-out` 与 `-transcript` 参数追加 `-resume`；续跑进程会恢复保留的 JSONL 历史，让模型从截断点直接继续（不发送新的用户消息），但需要重新编译并比较后才能提交。

## 隐私与数据边界

提交前不得把任何本地书籍/项目身份、作者名、原图内容描述、个人目录绝对路径、私有会话转录、源图片、候选图或运行输出放入本目录。只使用合成测试样例。`.gitignore` 会排除 JSONL、图片、PDF、指标输出、缓存和 Python 字节码；需要对比的真实数据应留在工作区外，不提交。

## 保护约定

`figure_compare/` 是有意纳入版本控制的前瞻项目，不是临时产物。清理、迁移或删除本目录前必须先逐项向用户确认。见本目录的 [AGENTS.md](AGENTS.md) 和 [设计说明](docs/design.md)。
