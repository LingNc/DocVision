# 06 · 用户档案与配置位置（规定）

> 技能的私有化配置（token、私有端点、后续可能的私有 key 类）**一律存规定位置**，
> 不散落在 shell 历史、临时文件或对话里。优先级：**环境变量 > 档案文件 > 内置默认**。

## 档案位置（唯一规定位）

**`<技能目录>/data/profile.env`**（随技能走；安装版在 `docvision-skills/data/`）。

```bash
cp data/profile.example data/profile.env && chmod 600 data/profile.env
# 编辑填入：MINERU_TOKEN=你的token  （# 开头为注释；MINERU_API_BASE_URL 一般不用填）
```

- **权限 600**（仅本人可读）；`data/profile.env` 已 gitignore，**永不入库**；
- 读取顺序：命令显式参数 > `MINERU_TOKEN` 等环境变量 > profile.env > 内置默认；
- 出包与 git 只带 `data/profile.example`（空值模板），不带真实档案。

## SSH 密钥（同属 profile 内容）

上报/推送用的 ed25519 密钥对属用户私有物，规定位：（技能目录下，gitignore 随 data/ 规则）；需要向用户出示的**永远只有公钥**（）。relay 服务器的  属服务器侧秘密，只在部署机环境变量，不进任何文件。

## 项目级配置（书与书不同）

`<项目根>/skills_config.json`（`config-get/config-set` 管理）：判据阈值
（figure.min_overlap 等）、拆分页数、试点章、字体回退、MinerU 模型参数。
**属于"书"的配置进 skills_config.json；属于"用户"的密钥类进 data/profile.env**——两不混。

## 新增配置项的落点纪律

- 密钥/token/私有端点 → profile.env（永不上库、文档不写示例值）；
- 判据数值/行为开关 → skills_config.json（用户专属，agent 无权）；
- 流程性默认值 → 代码内置 Default（开发者改）。
