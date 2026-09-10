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
页面左侧是会话列表（阶段标签、消息数、相对时间、活跃圆点），右侧是消息时间线：
用户/任务卡片与图片缩略图（点击放大，Esc 关闭）、可折叠的思考过程（带字符数）、
工具调用（工具名 + 格式化参数）、与调用配对编号着色的工具结果（默认只显示前 8 行，
可展开/复制）。转录里**没有系统提示词**，也**没有工具的 JSON Schema**（写入时就未落盘），
页面不会假装显示它们；无法解析的行会跳过并在顶部提示"跳过 N 行坏数据"。

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
	cmd.Flags().Bool("list", false, "只在终端列出扫到的会话（阶段/消息数/大小/修改时间/路径）")
	cmd.Flags().String("out", "", "静态导出路径（默认 <根目录>/sessions.html）")
	return cmd
}

// printSessions renders the terminal table used by --list.
func printSessions(sessions []sessionview.SessionInfo, root string) {
	if len(sessions) == 0 {
		fmt.Printf("没有找到会话转录（*.jsonl）：%s\n", root)
		return
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "阶段\t消息数\t大小\t修改时间\t路径")
	var msgs int
	for _, s := range sessions {
		msgs += s.Messages
		live := ""
		if s.Live {
			live = " ●"
		}
		fmt.Fprintf(w, "%s%s\t%d\t%s\t%s\t%s\n",
			s.Title, live, s.Messages, sessionview.HumanSize(s.Bytes),
			s.ModTime.Format("2006-01-02 15:04:05"), s.ID)
	}
	_ = w.Flush()
	fmt.Printf("\n共 %d 个会话 / %d 条消息（● = 最近 60 秒内有写入）\n根目录: %s\n", len(sessions), msgs, root)
}
