package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"mineru-tools/internal/config"

	"github.com/spf13/cobra"
)

// pickEditor returns the editor argv prefix, honoring explicit env
// overrides first, then vim > nano > vi as requested fallbacks.
func pickEditor() ([]string, error) {
	var candidates [][]string
	for _, env := range []string{"DOCVISION_EDITOR", "EDITOR", "VISUAL"} {
		if v := os.Getenv(env); strings.TrimSpace(v) != "" {
			candidates = append(candidates, strings.Fields(v))
		}
	}
	for _, name := range []string{"vim", "nano", "vi"} {
		candidates = append(candidates, []string{name})
	}
	for _, argv := range candidates {
		if _, err := exec.LookPath(argv[0]); err == nil {
			return argv, nil
		}
	}
	return nil, fmt.Errorf("未找到可用编辑器（vim/nano/vi）；可设置 $EDITOR 后重试，或直接手动编辑配置文件")
}

func newSetupCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "setup",
		Short: "编辑配置文件（自动选择 vim/nano）并在保存时校验",
		Long:  "打开编辑器修改生效的配置文件（优先当前目录 config.yaml，否则 ~/.docvision/config.yaml）。",
		RunE: func(cmd *cobra.Command, _ []string) error {
			path := resolvedConfigPath
			if path == "" {
				return fmt.Errorf("未能确定配置文件路径，请用 --config 指定")
			}
			argv, err := pickEditor()
			if err != nil {
				return err
			}
			fmt.Printf("编辑配置: %s（编辑器: %s）\n", path, argv[0])
			for {
				c := exec.Command(argv[0], append(argv[1:], path)...)
				c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
				if err := c.Run(); err != nil {
					fmt.Fprintf(os.Stderr, "编辑器异常退出: %v\n", err)
				}
				problems := config.ValidateFile(path)
				if len(problems) == 0 {
					fmt.Println("✓ 配置校验通过，已保存")
					return nil
				}
				fmt.Printf("\n配置存在 %d 个问题，未通过校验（文件保持你的修改）:\n", len(problems))
				for _, p := range problems {
					fmt.Printf("  ✗ %s\n", p)
				}
				fmt.Print("按回车重新编辑，输入 q 退出: ")
				line, rerr := bufio.NewReader(os.Stdin).ReadString('\n')
				ans := strings.TrimSpace(line)
				if rerr != nil || ans == "q" || ans == "quit" {
					return fmt.Errorf("已退出，配置文件内容未改动（仍未通过校验）")
				}
			}
		},
	}
}
