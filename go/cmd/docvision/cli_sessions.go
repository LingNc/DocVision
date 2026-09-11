package main

import (
	"fmt"
	"os"
	"path/filepath"
	"text/tabwriter"

	"github.com/spf13/cobra"

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
组内是会话行（阶段标签、消息数、大小、相对时间、活跃圆点）；右侧是消息时间线：
用户/任务卡片与图片缩略图（点击放大，Esc 关闭）、默认折叠的思考过程（带字符数，展开后
等宽低对比 + 卡内滚动）、工具调用（工具名 + 格式化参数）、与调用配对编号着色的工具结果
（默认只显示前 8 行，可展开/复制）。每个会话开头还会写一条 t=meta 元信息行记录**本次运行
模型被交代了什么**（系统提示词全文 + 当时发给 API 的工具定义），它**不参与会话回放**、
也不计入消息数，页面把它渲染成时间线最前面的可折叠卡片（老转录没有这行，照常显示）；
无法解析的行会跳过并在顶部提示"跳过 N 行坏数据"。

每次 API 请求还会写一条 t="usage" 用量行（同样不计入消息、不参与回放）：输入/输出 token、
前缀缓存命中量、耗时、首字延迟、结束原因。页面借此在时间线顶部显示**会话指标卡片**
（输入/输出 token、缓存命中率、平均首字延迟、输出速度 tok/s、平均耗时、会话跨度，
外加可展开的"每次请求明细"表），侧栏每个会话显示一行用量摘要并在底部给出所有会话的合计；
旧转录没有用量行时这一块不显示（不会用 0 冒充实测值）。每行还带写入时间戳 ts。

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

			if list {
				printSessions(sessions, root)
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
	fmt.Fprintln(w, "项目\t阶段\t消息数\t提示词\t用量\t大小\t修改时间\t路径")
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
		fmt.Fprintf(w, "%s\t%s%s\t%d\t%s\t%s\t%s\t%s\t%s\n",
			project,
			s.Title, live, s.Messages, prompt, usageCell(s), sessionview.HumanSize(s.Bytes),
			s.ModTime.Format("2006-01-02 15:04:05"), s.ID)
	}
	_ = w.Flush()
	fmt.Printf("\n共 %d 个会话 / %d 条消息（● = 最近 60 秒内有写入）\n根目录: %s\n", len(sessions), msgs, root)
}
