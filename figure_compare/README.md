# Figure Compare 前瞻原型

本目录维护可复用的图形比较算法与独立会话审阅 viewer。它是研发原型，不等同于 DocVision 正式功能；不含真实项目图片、转录或本地路径。

## 目录

- `algorithms/`：legacy 比较器、新版视觉证据评估器、容错指标、校准工具、judge 合约及会话停止策略。
- `tests/`：合成图像和结构化判定测试，作为源码纳入 Git。
- `runner/`：Go 单会话 harness，提供 `view_pdf` 候选预览与 `compare_figure` 比较；比较回执含最差区域坐标，可按区域放大复查。
- `viewer/`：单文件 HTML JSONL 会话浏览器模板。
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

程序会在指定输出目录产生 `session.jsonl`、运行日志和工作产物；打开 `viewer/viewer.html` 后选择包含多个运行子目录的根工作目录，再从会话列表中选择目标会话查看。配置、原图、输出路径均由命令行传入，不在源码中写死。

## 隐私与数据边界

提交前不得把任何本地书籍/项目身份、作者名、原图内容描述、个人目录绝对路径、私有会话转录、源图片、候选图或运行输出放入本目录。只使用合成测试样例。`.gitignore` 会排除 JSONL、图片、PDF、指标输出、缓存和 Python 字节码；需要对比的真实数据应留在工作区外，不提交。

## 保护约定

`figure_compare/` 是有意纳入版本控制的前瞻项目，不是临时产物。清理、迁移或删除本目录前必须先逐项向用户确认。见本目录的 [AGENTS.md](AGENTS.md) 和 [设计说明](docs/design.md)。
