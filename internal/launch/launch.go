// Package launch 负责启动游戏进程、追踪游玩时长、启动外部工具。
package launch

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/shirou/gopsutil/v4/process"

	"galgame-manager/internal/model"
)

// Session 是一次启动会话。
type Session struct {
	GameID  int64
	PID     int
	Started time.Time
	done    chan struct{}
	once    sync.Once
}

// Start 启动游戏并在后台监控进程树，退出时回调累计秒数。
func Start(gameID int64, exe string, args []string, cwd string,
	onExit func(gameID int64, seconds int64)) (*Session, error) {

	if _, err := os.Stat(exe); err != nil {
		return nil, fmt.Errorf("启动文件不存在: %s", exe)
	}
	if cwd == "" {
		cwd = filepath.Dir(exe)
	}

	cmd := exec.Command(exe, args...)
	cmd.Dir = cwd
	// 让游戏以独立进程组运行，避免随主程序退出
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x00000008} // DETACHED_PROCESS
	if err := cmd.Start(); err != nil {
		// 某些游戏不接受 DETACHED_PROCESS，退回普通启动
		cmd2 := exec.Command(exe, args...)
		cmd2.Dir = cwd
		if err2 := cmd2.Start(); err2 != nil {
			return nil, fmt.Errorf("无法启动游戏: %w", err)
		}
		cmd = cmd2
	}
	// 释放子进程句柄，避免僵尸
	go func() { _ = cmd.Wait() }()

	s := &Session{
		GameID:  gameID,
		PID:     cmd.Process.Pid,
		Started: time.Now(),
		done:    make(chan struct{}),
	}

	go func() {
		seconds := s.monitor()
		if onExit != nil {
			onExit(gameID, seconds)
		}
		s.once.Do(func() { close(s.done) })
	}()

	return s, nil
}

// monitor 轮询进程树，直到全部退出，返回运行秒数。
func (s *Session) monitor() int64 {
	root, err := process.NewProcess(int32(s.PID))
	if err != nil {
		time.Sleep(2 * time.Second)
		return int64(time.Since(s.Started).Seconds())
	}

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	// 启动初期给游戏一点时间，避免主程序瞬时退出被误判
	time.Sleep(1500 * time.Millisecond)

	for {
		select {
		case <-ticker.C:
			if !treeAlive(root) {
				// 再确认一次，避免启动器切进程的瞬间误判
				time.Sleep(2 * time.Second)
				if !treeAlive(root) {
					return int64(time.Since(s.Started).Seconds())
				}
			}
		}
	}
}

func treeAlive(root *process.Process) bool {
	if root == nil {
		return false
	}
	if running, err := root.IsRunning(); err == nil && running {
		return true
	}
	// 主进程退出后，检查是否有子进程仍在运行
	if children, err := root.Children(); err == nil {
		for _, c := range children {
			if running, err := c.IsRunning(); err == nil && running {
				return true
			}
		}
	}
	return false
}

// BuildArgs 组装启动参数（额外参数 + 窗口模式标志）。
func BuildArgs(g *model.Game, windowFlags map[string]string) []string {
	var args []string
	if extra := strings.TrimSpace(g.LaunchArgs); extra != "" {
		args = append(args, splitArgs(extra)...)
	}
	if flag, ok := windowFlags[g.WindowMode]; ok {
		flag = strings.TrimSpace(flag)
		if flag != "" {
			args = append(args, splitArgs(flag)...)
		}
	}
	return args
}

// splitArgs 按空格切分参数，支持双引号包裹。
func splitArgs(s string) []string {
	var out []string
	var cur strings.Builder
	inQuote := false
	for _, r := range s {
		switch {
		case r == '"':
			inQuote = !inQuote
		case (r == ' ' || r == '\t') && !inQuote:
			if cur.Len() > 0 {
				out = append(out, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteRune(r)
		}
	}
	if cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out
}

// ExpandToolArgs 展开翻译工具参数里的占位符。
func ExpandToolArgs(pattern string, g *model.Game) []string {
	replaced := strings.NewReplacer(
		"{folder}", g.Folder,
		"{exe}", g.ExePath,
		"{title}", g.Title,
	).Replace(pattern)
	return splitArgs(replaced)
}

// StartTool 启动一个外部工具（不追踪时长）。
func StartTool(path, args string, g *model.Game) error {
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("工具不存在: %s", path)
	}
	cmd := exec.Command(path, ExpandToolArgs(args, g)...)
	cmd.Dir = filepath.Dir(path)
	return cmd.Start()
}

// OpenFolder 在资源管理器中打开目录。
func OpenFolder(path string) error {
	if path == "" {
		return fmt.Errorf("路径为空")
	}
	return exec.Command("explorer.exe", path).Start()
}

// Kill 结束游戏进程树（用于「强制结束」）。
func Kill(pid int) error {
	p, err := process.NewProcess(int32(pid))
	if err != nil {
		return err
	}
	children, _ := p.Children()
	for _, c := range children {
		_ = c.Kill()
	}
	return p.Kill()
}
