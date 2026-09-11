package main

import (
	"fmt"
	"os"
	"path/filepath"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"mineru-tools/internal/config"
	"mineru-tools/internal/sessionview"
)

// newSessionsCmd wires the session viewer: a static snapshot of every AI
// session transcript under a root, or a local read-only server that refreshes
// while a pipeline is running.
func newSessionsCmd() *cobra.Command {
	// The invocation directory is captured while the command tree is being
	// built — that is, before cobra runs PersistentPreRunE, which chdirs into
	// the config file's directory when a global config applies. "默认扫描当前
	// 目录" has to mean the directory the user actually ran the command from.
	startDir, _ := os.Getwd()

	cmd := &cobra.Command{
		Use:   "sessions",
		Short: "会话预览：把 AI 会话转录渲染成可浏览页面（静态导出 / 本地实时服务）",
		Long: `会话预览——扫描根目录下的 AI 会话转录（*.jsonl）并渲染成可浏览页面。

扫描范围是根目录下任意层级的 *.jsonl，例如：
  work/style_session.jsonl（样式会话）、work/sessions/chapters.jsonl（章节划分）、
  work/sessions/convert_<章>.jsonl（单章转换）、
  source/sessions/vector_<图>.jsonl（档位2 矢量图会话）。
页面默认**白天模式**（工具栏「🌙 深色 / ☀️ 浅色」切换，选择记在浏览器 localStorage；
两种主题的配色都走 CSS 变量）。左侧**按项目分组**（点标题折叠/展开，带会话数）：多项目布局
下一个工作区 <输出根>/<书名>/ 是独立的一组（标题显示书名，输出根弱化成前缀），旧版单项目
输出根标「旧版单项目」，工程内部目录名（work/source/…）与扫描根下的散落转录归「（根目录）」；
组内是会话行（阶段标签、消息数、大小、相对时间、活跃圆点）。侧栏三层（项目 → 流程阶段 →
会话）**默认全部收起**，只认「手动展开过」的记忆；切会话时，装着当前会话的那一组会自动展开
（除非你手动收起过它）。逐图会话按**图片名短写**命名（图注 → 文件名的可读标签 → 前 8 位
内容哈希 + 扩展名，撞车退化到 12 位、再撞就写全名），64 位哈希只留在行的悬浮说明里。

中栏是消息时间线，**全部左对齐**（没有右侧气泡）：
  · 不带图的 user 行 = **系统消息**（「系统 · user 轮」标签）：在这个工具里 user 轮是 harness
    自己发的一轮，不是人打的字；长任务提示默认只露 8 行（渐隐），可「展开全文」并卡内滚动；
  · 带图的 user 行 = 图片投喂 / **工具图片回执**，按归属收进对应工具的展开区（会话开头的
    原图投喂轮则归到第一条任务），并注明归属依据（「归属：call <id>」精确匹配 /「归属：由顺序
    推断」/「归属：本会话任务」）；归属不出来的才单独成一条折叠行；
  · 工具调用 = **一次调用一行**（工具名 · 一行摘要 · 输入→输出 字符数 · ok/error），回执并进
    同一张卡片的「输入 / 输出 / 附件」三段里，展开后与思考块**同一套**限高内滚 + 「展开全文
    （N 行 / M 字符）」；
  · 消息正文支持 **Markdown 预览**（标题 / 列表 / 代码块 / 表格 / 链接，自带小渲染器、无外链、
    不拼 HTML），页签行右端有 Markdown 开关（默认开，可回到纯文本）；
  · 工具输入输出与代码块做**等宽高亮**：JSON（键 / 字符串 / 数字 / true-false-null）与终端
    输出（命令行 $ / #、diff 的 + / -、error/warning/OK 状态词、LaTeX 的 ! 与
    Overfull/Underfull、路径与 URL）；复制按钮复制的始终是原始文本；
  · 图片缩略图点击放大（Esc 关闭），默认折叠的思考过程带字符数（展开后等宽低对比 + 卡内滚动）。

「轨迹」页是**事件表**，与对话页刻意不同：一行一步、类型照转录里的**真实角色**
（用户 / 助手 / 思考 / 工具 / 结果 / 元信息 / 用量），工具调用与工具结果**各占一行不合并**；
带图 user 轮在摘要里写明「图片 ×N」与归属依据（点开能看图），点行跳回对话里合并后的那一块。

每个会话开头还会写一条 t=meta 元信息行记录**本次运行
模型被交代了什么**（系统提示词全文 + 当时发给 API 的工具定义），它**不参与会话回放**、
也不计入消息数，页面把它渲染成时间线最前面的可折叠卡片（老转录没有这行，照常显示）；
无法解析的行会跳过并在顶部提示"跳过 N 行坏数据"。

每次 API 请求还会写一条 t="usage" 用量行（同样不计入消息、不参与回放）：输入/输出 token、
前缀缓存命中量、耗时、首字延迟、结束原因。页面借此在时间线顶部显示**会话指标卡片**
（输入/输出 token、缓存命中率、平均首字延迟、输出速度 tok/s、平均耗时、会话跨度，
外加可展开的"每次请求明细"表），侧栏每个会话显示一行用量摘要并在底部给出所有会话的合计；
旧转录没有用量行时这一块不显示（不会用 0 冒充实测值）。每行还带写入时间戳 ts。
工具产出的图片在 wire 上只能走 user 消息（tool 消息的 content 只能是文本），所以转录里
「带图 user 行」多半是**工具图片回执**：这些轮的文本句柄是
Tool image output from <工具> (call <id>) (for your visual review):，页面据此精确归属；
旧转录的句柄只有 Tool image output 前缀，页面按顺序推断到同一轮的调用上。

两种模式：
  静态导出（默认）  生成一个自包含 HTML（数据/样式/脚本全部内嵌），图片按相对路径
                    引用，file:// 直接打开即可，不需要服务；页面显示
                    "静态快照 · 生成于 …"，不做任何轮询。
  实时服务 --serve  启动本地只读 HTTP 服务（默认只监听 127.0.0.1:8848），页面每 2 秒
                    轮询 /api/index，当前会话有新行就按行号增量追加渲染（保持滚动位置，
                    打开"自动跟随"则滚到底部），适合边跑流程边看会话。服务只读，
                    只提供根目录内的文件并拒绝越界路径（../）。

示例：
  docvision sessions                            # 扫描当前目录，生成 ./sessions.html
  docvision sessions --dir ~/PDF2MD             # 指定扫描根目录
  docvision sessions --list                     # 只在终端列出扫到的会话
  docvision sessions --serve                    # 实时预览 http://127.0.0.1:8848/
  docvision sessions --serve --addr 127.0.0.1:9000
  docvision sessions --out /tmp/sessions.html   # 自定义静态导出路径`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			dirFlag, _ := cmd.Flags().GetString("dir")
			root := dirFlag
			if root == "" {
				root = startDir
			} else if !filepath.IsAbs(root) {
				// Relative --dir stays relative to where the user started, for
				// the same reason the default does.
				root = filepath.Join(startDir, root)
			}
			root, err := filepath.Abs(root)
			if err != nil {
				return err
			}

			serve, _ := cmd.Flags().GetBool("serve")
			list, _ := cmd.Flags().GetBool("list")
			if serve && list {
				return fmt.Errorf("--list 与 --serve 不能同时使用：--list 只在终端列出会话")
			}

			sessions, err := sessionview.Scan(root)
			if err != nil {
				return err
			}
			// 价格来自配置里的 models.*.price（每个模型一份费率），没有配置
			// 就整篇不显示金额：¥0 会被读成"这次没花钱"。
			prices := map[string]config.PriceConfig{}
			if cfg, cerr := loadConfigWithFlag(cmd); cerr != nil {
				// 读不到配置就只是没有价格，不该挡住查看会话。
				fmt.Fprintln(os.Stderr, "提示：读取配置失败，本次不显示金额：", cerr)
			} else {
				prices = cfg.ModelPrices()
			}
			sessionview.ApplyPrices(sessions, prices)

			if list {
				printSessions(sessions, root)
				return nil
			}
			if costOnly, _ := cmd.Flags().GetBool("cost"); costOnly {
				printCostReport(sessions, len(prices) > 0)
				return nil
			}
			if serve {
				addr, _ := cmd.Flags().GetString("addr")
				// 不自动打开浏览器：命令行里打印确切 URL，由用户决定怎么打开。
				return sessionview.Serve(root, addr, false)
			}

			out, _ := cmd.Flags().GetString("out")
			if out == "" {
				out = filepath.Join(root, "sessions.html")
			} else if !filepath.IsAbs(out) {
				// 相对 --out 必须按"命令启动时"的目录解析：进程在读取
				// 配置后可能已经 chdir 到 ~/.docvision，此时相对路径会
				// 落到那里（并因只读而报 mkdir 失败）。
				out = filepath.Join(startDir, out)
			}
			out, err = filepath.Abs(out)
			if err != nil {
				return err
			}
			if err := sessionview.WriteStaticHTML(root, out, sessions); err != nil {
				return err
			}
			var msgs int
			for _, s := range sessions {
				msgs += s.Messages
			}
			fmt.Printf("会话预览已生成: %s\n", out)
			fmt.Printf("共 %d 个会话 / %d 条消息。用浏览器打开它：\n  file://%s\n", len(sessions), msgs, out)
			fmt.Println("需要边跑边自动刷新时改用: docvision sessions --serve")
			return nil
		},
	}
	cmd.Flags().String("dir", "", "扫描根目录（默认：运行命令时所在的目录）")
	cmd.Flags().Bool("serve", false, "启动本地只读服务并实时刷新（打印 URL，不自动打开浏览器）")
	cmd.Flags().String("addr", sessionview.DefaultAddr, "服务监听地址（默认 127.0.0.1:8848，只监听本机）")
	cmd.Flags().Bool("list", false, "只在终端列出扫到的会话（项目/阶段/消息数/提示词字符数/大小/修改时间/路径）")
	cmd.Flags().String("out", "", "静态导出路径（默认 <根目录>/sessions.html）")
	cmd.Flags().Bool("cost", false, "只打印按阶段的用量与费用报告（token/缓存命中率/价格/平均每次请求），价格来自配置 models.*.price")
	return cmd
}

// usageCell summarises the session's token accounting for --list: requests,
// tokens in/out and the prefix-cache hit rate. "-" means the transcript has no
// t="usage" lines (it predates usage recording), which must not look like a
// measurement of zero.
func usageCell(s sessionview.SessionInfo) string {
	st := s.Stats
	if st == nil || st.Requests == 0 {
		return "-"
	}
	cell := fmt.Sprintf("%d req · %s/%s", st.Requests,
		sessionview.HumanCount(st.PromptTokens), sessionview.HumanCount(st.Completion))
	if st.PromptTokens > 0 {
		cell += fmt.Sprintf(" · 缓存 %.0f%%", st.CacheHitPct)
	}
	if st.OutputTPS > 0 {
		cell += fmt.Sprintf(" · %.1f tok/s", st.OutputTPS)
	}
	return cell
}

// printSessions renders the terminal table used by --list.
func printSessions(sessions []sessionview.SessionInfo, root string) {
	if len(sessions) == 0 {
		fmt.Printf("没有找到会话转录（*.jsonl）：%s\n", root)
		return
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "项目\t阶段\t消息数\t提示词\t用量\t成本\t大小\t修改时间\t路径")
	var msgs int
	for _, s := range sessions {
		msgs += s.Messages
		live := ""
		if s.Live {
			live = " ●"
		}
		prompt := "-"
		if s.Meta != nil && s.Meta.PromptChars > 0 {
			prompt = fmt.Sprintf("%d 字符", s.Meta.PromptChars)
		}
		project := s.Project
		if project == "" {
			project = "（根目录）"
		}
		fmt.Fprintf(w, "%s\t%s%s\t%d\t%s\t%s\t%s\t%s\t%s\t%s\n",
			project,
			s.Title, live, s.Messages, prompt, usageCell(s), costCell(s), sessionview.HumanSize(s.Bytes),
			s.ModTime.Format("2006-01-02 15:04:05"), s.ID)
	}
	_ = w.Flush()
	fmt.Printf("\n共 %d 个会话 / %d 条消息（● = 最近 60 秒内有写入）\n根目录: %s\n", len(sessions), msgs, root)
}

// costCell renders the session's money for --list: "-" when the model has no
// configured price (an unknown rate must not look like a free session) and
// "≥" when some of its requests could not be priced.
func costCell(s sessionview.SessionInfo) string {
	if s.Cost == nil {
		return "-"
	}
	return fmt.Sprintf("%s%.2f", s.Cost.Currency, s.Cost.Total)
}

// printCostReport renders the per-stage cost table: which stage spent what,
// with the token split, the prefix-cache hit rate and the average per request.
// Total row included; per-image / per-page averages need the book's scale and
// are printed by the latex run itself (see internal/latex CostSummary).
func printCostReport(sessions []sessionview.SessionInfo, pricesConfigured bool) {
	rows := sessionview.StageCosts(sessions)
	if len(rows) == 0 {
		fmt.Println("没有可统计的会话")
		return
	}
	total := sessionview.TotalCost(sessions)
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "阶段\t会话\t请求\t输入 tokens\t缓存命中\t缓存率\t输出 tokens\t费用\t每次请求")
	var reqs, in, cached, out int
	var money float64
	currency := ""
	unpriced := sessionview.UnpricedRequests(sessions)
	for _, r := range rows {
		cache := "-"
		if r.PromptTokens > 0 {
			cache = fmt.Sprintf("%s (%.0f%%)", sessionview.HumanCount(r.CachedTokens), r.CacheHitPct)
		}
		cost := "未配价"
		if r.Currency != "" || r.Cost > 0 {
			cost = fmt.Sprintf("%s%.2f", r.Currency, r.Cost)
		}
		if r.Unpriced > 0 {
			cost = "≥" + cost
		}
		avg := "-"
		if r.Requests > 0 && (r.Currency != "" || r.Cost > 0) {
			avg = fmt.Sprintf("%s%.4f", r.Currency, r.AvgCostPerRequest())
		}
		fmt.Fprintf(w, "%s\t%d\t%d\t%s\t%s\t%s\t%s\t%s\t%s\n",
			r.Stage, r.Sessions, r.Requests, sessionview.HumanCount(r.PromptTokens),
			cache, cachePct(r.PromptTokens, r.CachedTokens), sessionview.HumanCount(r.Completion), cost, avg)
		reqs += r.Requests
		in += r.PromptTokens
		cached += r.CachedTokens
		out += r.Completion
		money += r.Cost
		if r.Currency != "" {
			currency = r.Currency
		}
	}
	// 没有任何一口价命中时，金额列写 "-" 而不是 "0.00"：0.00 会被读成
	// "这次几乎没花钱"，而真实含义是"没配价格、算不出来"。
	totalMoney := "-"
	if currency != "" {
		totalMoney = fmt.Sprintf("%s%.2f", currency, money)
	}
	fmt.Fprintf(w, "合计\t%d\t%d\t%s\t%s\t%s\t%s\t%s\t-\n",
		len(sessions), reqs, sessionview.HumanCount(in), sessionview.HumanCount(cached),
		cachePct(in, cached), sessionview.HumanCount(out), totalMoney)
	_ = w.Flush()
	if unpriced > 0 {
		fmt.Printf("注意：有 %d 次请求的模型没配价格，金额是**下界**（models.<条目>.price 里补 input/cached/output）\n", unpriced)
	}
	if total == nil {
		if pricesConfigured {
			fmt.Println("注意：会话用的模型名在价格表里找不到（转录里的 model 字段必须与 models.<条目>.model 一致），本次不显示金额。")
		} else {
			fmt.Println("注意：配置里没有**可用**的 models.*.price（没写，或单价全是 0），因此不显示金额（配置文件:" + configPathUsed() + "）。")
		}
	}
}

// configPathUsed reports which config file the command resolved (the --config
// flag or the discovered ./config.yaml / ~/.docvision/config.yaml). It lands in
// the "no prices configured" hint because that hint is usually produced by a
// config the user did not mean to load.
func configPathUsed() string {
	if resolvedConfigPath != "" {
		return resolvedConfigPath
	}
	return "未找到"
}

func cachePct(prompt, cached int) string {
	if prompt <= 0 {
		return "-"
	}
	return fmt.Sprintf("%.0f%%", float64(cached)*100/float64(prompt))
}
