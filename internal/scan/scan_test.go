package scan

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCleanTitle(t *testing.T) {
	cases := map[string]string{
		"[Key] Little Busters! [v1.02]": "Little Busters!",
		"CLANNAD_HD_Edition":            "CLANNAD HD Edition",
		"Steins;Gate (CHS)":             "Steins;Gate",
		"白色相簿2":                         "白色相簿2",
		"Grisaia no Kajitsu [Repack]":   "Grisaia no Kajitsu",
	}
	for in, want := range cases {
		if got := CleanTitle(in); got != want {
			t.Errorf("清洗 %q 得到 %q，期望 %q", in, got, want)
		}
	}
}

func TestPlausibleExe(t *testing.T) {
	bad := []string{"unins000", "setup", "vcredist_x86", "dxwebsetup",
		"crashpad_handler", "UnityCrashHandler64", "Qt5Core", "update"}
	for _, name := range bad {
		if plausibleExe(name) {
			t.Errorf("%q 不应被当作主程序", name)
		}
	}
	good := []string{"CLANNAD", "game", "SGGame", "start", "Rewrite", "Launcher"}
	for _, name := range good {
		if !plausibleExe(name) {
			t.Errorf("%q 应被当作主程序", name)
		}
	}
}

// 构造一个多层目录，验证剪枝与识别是否正确。
func TestDiscover(t *testing.T) {
	root := t.TempDir()
	mk := func(parts ...string) string {
		p := filepath.Join(append([]string{root}, parts...)...)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("MZ"), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}

	mk("CLANNAD", "CLANNAD.exe")
	mk("CLANNAD", "unins000.exe")
	mk("Steins Gate", "SGGame.exe")
	mk("Series", "Little Busters", "bin", "LB.exe")
	mk("Series", "Rewrite", "Rewrite.exe")
	os.MkdirAll(filepath.Join(root, "Music"), 0o755)

	got := Discover(root, 6, false)
	names := map[string]bool{}
	for _, g := range got {
		names[filepath.Base(g.Folder)] = true
	}

	for _, want := range []string{"CLANNAD", "Steins Gate", "Little Busters", "Rewrite"} {
		if !names[want] {
			t.Errorf("未识别出 %q，实际: %v", want, names)
		}
	}
	if names["Series"] {
		t.Error("合集目录不应被识别为游戏")
	}
	if names["Music"] {
		t.Error("没有 exe 的目录不应被识别为游戏")
	}

	// 校验启动文件：CLANNAD 应选 CLANNAD.exe 而不是 unins000.exe
	for _, g := range got {
		if filepath.Base(g.Folder) == "CLANNAD" {
			if filepath.Base(g.Exe) != "CLANNAD.exe" {
				t.Errorf("启动文件识别错误: %s", g.Exe)
			}
		}
		if filepath.Base(g.Folder) == "Little Busters" {
			if filepath.Base(g.Exe) != "LB.exe" {
				t.Errorf("子目录启动文件识别错误: %s", g.Exe)
			}
		}
	}
}

func TestImageSize(t *testing.T) {
	// 1x1 PNG
	png := []byte{
		0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a,
		0, 0, 0, 0x0d, 'I', 'H', 'D', 'R',
		0, 0, 0, 1, 0, 0, 0, 1, 8, 6, 0, 0, 0,
		0x1f, 0x15, 0xc4, 0x89,
	}
	path := filepath.Join(t.TempDir(), "a.png")
	if err := os.WriteFile(path, png, 0o644); err != nil {
		t.Fatal(err)
	}
	w, h := ImageSize(path)
	if w != 1 || h != 1 {
		t.Errorf("PNG 尺寸读取错误: %dx%d", w, h)
	}
}
