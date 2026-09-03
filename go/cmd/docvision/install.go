package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"mineru-tools/internal/config"

	"github.com/spf13/cobra"
)

// installTargets lists candidate install paths in priority order.
func installTargets() []string {
	targets := []string{"/usr/local/bin/docvision"}
	if home, err := os.UserHomeDir(); err == nil {
		targets = append(targets, filepath.Join(home, ".local", "bin", "docvision"))
	}
	return targets
}

func newInstallCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "install",
		Short: "安装 docvision 到系统路径并初始化 ~/.docvision",
		Long:  "把当前可执行文件复制到 /usr/local/bin（无权限时回退 ~/.local/bin），并确保 ~/.docvision/config.yaml 存在。安装后可在任意目录使用 docvision。",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if runtime.GOOS == "windows" {
				return fmt.Errorf("Windows 无需 install：把 docvision.exe 放入任意 PATH 目录即可")
			}
			exe, err := os.Executable()
			if err != nil {
				return fmt.Errorf("定位自身可执行文件: %w", err)
			}
			if real, err := filepath.EvalSymlinks(exe); err == nil {
				exe = real
			}
			data, err := os.ReadFile(exe)
			if err != nil {
				return fmt.Errorf("读取自身: %w", err)
			}
			var installed string
			var lastErr error
			for _, target := range installTargets() {
				if err := writeInstalledBinary(target, exe, data); err != nil {
					lastErr = err
					continue
				}
				installed = target
				break
			}
			if installed == "" {
				return fmt.Errorf("安装失败: %v；请用 sudo 重试: sudo %s install", lastErr, exe)
			}
			fmt.Printf("✓ 已安装到 %s\n", installed)
			path, created, err := config.EnsureDefaultConfig()
			if err != nil {
				return err
			}
			if created {
				fmt.Printf("✓ 已创建默认配置: %s\n", path)
			} else {
				fmt.Printf("✓ 配置已存在: %s\n", path)
			}
			fmt.Println("\n现在可在任意目录使用 docvision；首次使用请运行: docvision setup")
			return nil
		},
	}
}

func writeInstalledBinary(target, selfPath string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	// Installing over the running binary itself is a no-op.
	self, errSelf := os.Stat(selfPath)
	dst, errDst := os.Stat(target)
	if errSelf == nil && errDst == nil && os.SameFile(self, dst) {
		return nil
	}
	return os.WriteFile(target, data, 0o755)
}

func newUninstallCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "uninstall",
		Short: "从系统路径移除已安装的 docvision",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if runtime.GOOS == "windows" {
				return fmt.Errorf("Windows 请直接删除 PATH 目录中的 docvision.exe")
			}
			removed := 0
			for _, target := range installTargets() {
				if err := os.Remove(target); err == nil {
					fmt.Printf("✓ 已移除 %s\n", target)
					removed++
				}
			}
			if removed == 0 {
				fmt.Println("未找到已安装的 docvision")
			}
			fmt.Println("注意: ~/.docvision（配置与工作目录）已保留，如需删除请手动处理")
			return nil
		},
	}
}
