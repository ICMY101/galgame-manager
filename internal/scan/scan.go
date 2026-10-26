// Package scan 负责游戏目录发现、启动文件识别与封面识别。
package scan

import (
	"bufio"
	"encoding/binary"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

// GameDir 是一个被识别出来的游戏目录。
type GameDir struct {
	Folder    string
	Title     string
	Exe       string
	Cover     string
	SizeBytes int64
	ExeCount  int
}

var (
	// 明显不是主程序的 exe 关键字
	excludeKeywords = []string{
		"unins", "uninstall", "setup", "install", "update", "updater", "upgrade",
		"redist", "vcredist", "dotnet", "directx", "dxsetup", "d3dx",
		"dxwebsetup", "crash", "crashpad", "crashreport", "bugreport",
		"qt5", "qt6", "qtwebengine", "cef", "webview", "electron",
		"checkforupdate", "patcher", "patch", "config", "setting",
		"register", "activator", "keygen", "helper", "service", "daemon",
		"msvc", "python", "node", "uninst", "isuninst",
	}
	excludeRe = regexp.MustCompile(`(?i)(?:^|[_\-. ])(` +
		strings.Join(excludeKeywords, "|") + `)(?:[_\-. ]|$)`)

	whitelistRe = regexp.MustCompile(`(?i)^(game|galgame|start|play|main|run)$`)

	goodExeNames = map[string]bool{
		"game": true, "galgame": true, "start": true, "startup": true,
		"play": true, "run": true, "main": true, "launch": true,
		"launcher": true, "client": true, "app": true, "adv": true,
		"vn": true, "boot": true,
	}

	binDirNames = map[string]bool{
		"bin": true, "bin32": true, "bin64": true, "binary": true, "binaries": true,
		"exe": true, "exes": true, "program": true, "app": true, "game": true,
		"win": true, "win32": true, "win64": true, "x64": true, "x86": true,
		"release": true, "build": true, "dist": true,
	}

	imageExts = map[string]bool{
		".jpg": true, ".jpeg": true, ".png": true, ".webp": true, ".bmp": true,
	}

	coverExact = []string{
		"cover", "package", "boxart", "box", "front", "poster", "folder",
		"jacket", "keyvisual", "key_visual", "mainvisual", "cover_art", "title",
	}
	coverLoose = []string{"cover", "package", "boxart", "front", "poster", "jacket", "keyvisual"}

	coverBadWords = []string{
		"bgm", "logo", "icon", "screenshot", "screen", "shot", "thumb_",
		"banner", "button", "cursor", "sprite", "chara", "face", "ui_",
		"loading", "arrow", "frame", "effect", "particle", "map", "bg_",
	}

	// 目录名里的发布组标记 [Group]、版本号 (v1.02) 等
	bracketRe = regexp.MustCompile(`[\[\(【（〔][^\]\)】）〕]{0,40}[\]\)】）〕]`)
	tailJunkRe = regexp.MustCompile(`(?i)(?:[_\-\s]+(?:v|ver|version)\s?\d[\w.]*` +
		`|[_\-\s]+(?:full|cracked|uncensored|repack|rip|portable|eng|chs|jpn|cn|zh|ja)\b` +
		`|[_\-\s]+(?:crack|patch|update)\b)`)
	spaceRe = regexp.MustCompile(`[_\s]+`)

	// 扫描时跳过的系统/缓存目录
	skipDirs = map[string]bool{
		"$recycle.bin": true, "system volume information": true,
		"node_modules": true, "__pycache__": true, ".git": true, ".svn": true,
		"windows": true, "program files": true, "program files (x86)": true,
	}
)

// MaxDepth 是递归发现游戏目录的默认最大深度。
const MaxDepth = 6

// CleanTitle 把目录名清洗成适合显示的标题。
func CleanTitle(name string) string {
	if strings.TrimSpace(name) == "" {
		return ""
	}
	t := bracketRe.ReplaceAllString(name, " ")
	t = tailJunkRe.ReplaceAllString(t, " ")
	t = spaceRe.ReplaceAllString(t, " ")
	t = strings.Join(strings.Fields(t), " ")
	t = strings.Trim(t, " -_·.")
	if t == "" {
		return strings.TrimSpace(name)
	}
	return t
}

// ---------------------------------------------------------------- 目录遍历

func readDirNames(dir string) []os.DirEntry {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	return entries
}

func subDirs(dir string) []string {
	var out []string
	for _, e := range readDirNames(dir) {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasPrefix(name, ".") || skipDirs[strings.ToLower(name)] {
			continue
		}
		out = append(out, filepath.Join(dir, name))
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(filepath.Base(out[i])) < strings.ToLower(filepath.Base(out[j]))
	})
	return out
}

// topExes 返回目录顶层的 exe（已按可信度过滤）。
func topExes(dir string) []string {
	var out []string
	for _, e := range readDirNames(dir) {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.EqualFold(filepath.Ext(name), ".exe") {
			continue
		}
		if plausibleExe(strings.TrimSuffix(name, filepath.Ext(name))) {
			out = append(out, filepath.Join(dir, name))
		}
	}
	return out
}

func allExes(dir string, maxDepth int) []string {
	base := len(strings.Split(filepath.Clean(dir), string(os.PathSeparator)))
	var out []string
	filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			if d != nil && d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			depth := len(strings.Split(filepath.Clean(p), string(os.PathSeparator))) - base
			if depth > maxDepth {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.EqualFold(filepath.Ext(p), ".exe") {
			out = append(out, p)
		}
		return nil
	})
	return out
}

// plausibleExe 从文件名层面判断是否可能是游戏主程序。
func plausibleExe(stem string) bool {
	if whitelistRe.MatchString(stem) {
		return true
	}
	low := strings.ToLower(stem)
	// 精确词匹配（如 game_update.exe）
	if excludeRe.MatchString(stem) {
		return false
	}
	// 前缀匹配（如 unins000、vcredist_x86、dxwebsetup）
	for _, kw := range excludeKeywords {
		if strings.HasPrefix(low, kw) {
			return false
		}
	}
	// 包含匹配：这类词出现在任何位置都不像主程序
	for _, kw := range []string{"crashhandler", "crashpad", "vcredist", "dxwebsetup", "unins", "unitycrash"} {
		if strings.Contains(low, kw) {
			return false
		}
	}
	return true
}

func cleanKey(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func fileSize(p string) int64 {
	st, err := os.Stat(p)
	if err != nil {
		return 0
	}
	return st.Size()
}

// ScoreExe 给候选 exe 打分，分越高越可能是主程序。
func ScoreExe(exe, folder, title string) float64 {
	stem := strings.TrimSuffix(filepath.Base(exe), filepath.Ext(exe))
	if !plausibleExe(stem) {
		return -100
	}
	clean := cleanKey(stem)
	folderKey := cleanKey(filepath.Base(folder))
	titleKey := cleanKey(title)

	score := 0.0

	// 层级：越靠近游戏根目录越可能是主程序
	depth := strings.Count(strings.TrimPrefix(exe, folder), string(os.PathSeparator))
	switch depth {
	case 1:
		score += 5
	case 2:
		score += 2
	case 3:
		score -= 1
	default:
		score -= 4
	}

	// 名称与目录名/标题的匹配（最强信号）
	if clean != "" && folderKey != "" {
		switch {
		case clean == folderKey:
			score += 9
		case strings.Contains(folderKey, clean), strings.Contains(clean, folderKey):
			score += 5
		}
	}
	if clean != "" && titleKey != "" {
		switch {
		case clean == titleKey:
			score += 6
		case strings.Contains(titleKey, clean), strings.Contains(clean, titleKey):
			score += 3
		}
	}

	if goodExeNames[clean] {
		score += 2.5
	} else if strings.HasSuffix(clean, "game") || strings.HasSuffix(clean, "launcher") ||
		strings.HasSuffix(clean, "start") {
		score += 1.5
	}

	switch sz := fileSize(exe); {
	case sz >= 5<<20:
		score += 3
	case sz >= 1<<20:
		score += 2
	case sz >= 200<<10:
		score += 0.5
	default:
		score -= 2
	}

	return score
}

// DetectExe 在目录（含浅层子目录）中选出最可能的启动文件。
func DetectExe(folder, title string, maxDepth int) string {
	best, bestScore := "", 0.0
	for _, exe := range allExes(folder, maxDepth) {
		if s := ScoreExe(exe, folder, title); s > bestScore {
			best, bestScore = exe, s
		}
	}
	return best
}

// CandidateExes 列出可用作启动入口的 exe，按可信度降序。
func CandidateExes(folder string, maxDepth int) []string {
	type pair struct {
		s float64
		p string
	}
	var list []pair
	for _, exe := range allExes(folder, maxDepth) {
		if s := ScoreExe(exe, folder, ""); s > -50 {
			list = append(list, pair{s, exe})
		}
	}
	sort.Slice(list, func(i, j int) bool { return list[i].s > list[j].s })
	out := make([]string, 0, len(list))
	for _, p := range list {
		out = append(out, p.p)
	}
	return out
}

// ---------------------------------------------------------------- 图片尺寸

// ImageSize 读取常见图片格式的宽高，失败返回 (0,0)。
func ImageSize(path string) (int, int) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0
	}
	defer f.Close()

	head := make([]byte, 32)
	n, _ := io.ReadFull(f, head)
	if n < 26 {
		return 0, 0
	}

	// PNG
	if head[0] == 0x89 && head[1] == 'P' && head[2] == 'N' && head[3] == 'G' {
		w := int(binary.BigEndian.Uint32(head[16:20]))
		h := int(binary.BigEndian.Uint32(head[20:24]))
		return w, h
	}
	// BMP
	if head[0] == 'B' && head[1] == 'M' {
		w := int(int32(binary.LittleEndian.Uint32(head[18:22])))
		h := int(int32(binary.LittleEndian.Uint32(head[22:26])))
		if w < 0 {
			w = -w
		}
		if h < 0 {
			h = -h
		}
		return w, h
	}
	// WebP (VP8X)
	if string(head[0:4]) == "RIFF" && string(head[8:12]) == "WEBP" &&
		string(head[12:16]) == "VP8X" && n >= 30 {
		w := int(uint32(head[24]) | uint32(head[25])<<8 | uint32(head[26])<<16) + 1
		h := int(uint32(head[27]) | uint32(head[28])<<8 | uint32(head[29])<<16) + 1
		return w, h
	}
	// JPEG：扫描 SOFn 段
	if head[0] == 0xFF && head[1] == 0xD8 {
		f.Seek(2, io.SeekStart)
		br := bufio.NewReader(f)
		for {
			b, err := br.ReadByte()
			if err != nil {
				return 0, 0
			}
			if b != 0xFF {
				continue
			}
			marker, err := br.ReadByte()
			if err != nil {
				return 0, 0
			}
			for marker == 0xFF {
				if marker, err = br.ReadByte(); err != nil {
					return 0, 0
				}
			}
			if marker == 0xD8 || marker == 0xD9 || (marker >= 0xD0 && marker <= 0xD7) {
				continue
			}
			var lenBuf [2]byte
			if _, err := io.ReadFull(br, lenBuf[:]); err != nil {
				return 0, 0
			}
			segLen := int(binary.BigEndian.Uint16(lenBuf[:]))
			if segLen < 2 {
				return 0, 0
			}
			if marker >= 0xC0 && marker <= 0xCF &&
				marker != 0xC4 && marker != 0xC8 && marker != 0xCC {
				buf := make([]byte, 5)
				if _, err := io.ReadFull(br, buf); err != nil {
					return 0, 0
				}
				h := int(binary.BigEndian.Uint16(buf[1:3]))
				w := int(binary.BigEndian.Uint16(buf[3:5]))
				return w, h
			}
			if _, err := br.Discard(segLen - 2); err != nil {
				return 0, 0
			}
		}
	}
	return 0, 0
}

// ---------------------------------------------------------------- 封面

func scoreImage(path string) float64 {
	name := strings.ToLower(strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)))
	for _, bad := range coverBadWords {
		if strings.Contains(name, bad) {
			return -100
		}
	}
	score := 0.0
	exact := false
	for _, c := range coverExact {
		if name == c {
			score += 12
			exact = true
			break
		}
	}
	if !exact {
		for _, c := range coverLoose {
			if strings.Contains(name, c) {
				score += 7
				break
			}
		}
	}

	w, h := ImageSize(path)
	if w > 0 && h > 0 {
		ratio := float64(h) / float64(w)
		switch {
		case ratio >= 1.2:
			score += 4
		case ratio >= 0.9:
			score += 1
		default:
			score -= 2
		}
		if w < 150 || h < 150 {
			score -= 6
		}
		score += min(3.0, float64(w*h)/400000.0)
	} else {
		score -= 3
	}

	score += min(1.5, float64(fileSize(path))/float64(600*1024))
	return score
}

func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

// DetectCover 在目录及浅层子目录中找最像封面的图片。
func DetectCover(folder string, maxDepth int) string {
	base := len(strings.Split(filepath.Clean(folder), string(os.PathSeparator)))
	type cand struct {
		score float64
		path  string
	}
	var cands []cand

	filepath.WalkDir(folder, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			if d != nil && d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			depth := len(strings.Split(filepath.Clean(p), string(os.PathSeparator))) - base
			if depth > maxDepth {
				return filepath.SkipDir
			}
			if depth > 0 {
				switch strings.ToLower(d.Name()) {
				case "voice", "se", "sfx", "bgm", "sound", "movie", "video", "save", "savedata":
					return filepath.SkipDir
				}
			}
			return nil
		}
		if !imageExts[strings.ToLower(filepath.Ext(p))] {
			return nil
		}
		if s := scoreImage(p); s > -50 {
			cands = append(cands, cand{s, p})
		}
		if len(cands) > 500 {
			return io.EOF
		}
		return nil
	})

	if len(cands) == 0 {
		return ""
	}
	sort.Slice(cands, func(i, j int) bool { return cands[i].score > cands[j].score })
	return cands[0].path
}

// ---------------------------------------------------------------- 目录发现

func looksLikeGameDir(dir string) bool {
	if len(topExes(dir)) > 0 {
		return true
	}
	for _, sub := range subDirs(dir) {
		if binDirNames[strings.ToLower(filepath.Base(sub))] && len(topExes(sub)) > 0 {
			return true
		}
	}
	return false
}

// DirSize 统计目录大小（限制文件数上限，避免超大目录卡住）。
func DirSize(dir string, limit int) int64 {
	var total int64
	n := 0
	filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if info, err := d.Info(); err == nil {
			total += info.Size()
		}
		n++
		if n >= limit {
			return io.EOF
		}
		return nil
	})
	return total
}

// Discover 遍历 root，返回识别到的游戏目录列表。
// 采用剪枝策略：识别出游戏目录后不再深入其子目录。
func Discover(root string, maxDepth int, withSize bool) []GameDir {
	if maxDepth <= 0 {
		maxDepth = MaxDepth
	}
	st, err := os.Stat(root)
	if err != nil || !st.IsDir() {
		return nil
	}

	// 根目录本身就是单个游戏
	if looksLikeGameDir(root) {
		return []GameDir{buildGameDir(root, withSize)}
	}

	var results []GameDir
	type node struct {
		path  string
		depth int
	}
	stack := []node{{root, 0}}
	for len(stack) > 0 {
		cur := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if cur.depth >= maxDepth {
			continue
		}
		for _, sub := range subDirs(cur.path) {
			if looksLikeGameDir(sub) {
				results = append(results, buildGameDir(sub, withSize))
			} else {
				stack = append(stack, node{sub, cur.depth + 1})
			}
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return strings.ToLower(results[i].Title) < strings.ToLower(results[j].Title)
	})
	return results
}

func buildGameDir(folder string, withSize bool) GameDir {
	title := CleanTitle(filepath.Base(folder))
	exe := DetectExe(folder, title, 1)
	if exe == "" {
		for _, sub := range subDirs(folder) {
			if binDirNames[strings.ToLower(filepath.Base(sub))] {
				if e := DetectExe(sub, title, 1); e != "" {
					exe = e
					break
				}
			}
		}
	}
	g := GameDir{
		Folder: folder,
		Title:  title,
		Exe:    exe,
		Cover:  DetectCover(folder, 2),
	}
	g.ExeCount = len(CandidateExes(folder, 2))
	if withSize {
		g.SizeBytes = DirSize(folder, 40000)
	}
	return g
}

// HumanSize 把字节数格式化成可读字符串。
func HumanSize(n int64) string {
	const unit = 1024
	if n < unit {
		return strconv.FormatInt(n, 10) + " B"
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	units := []string{"KB", "MB", "GB", "TB", "PB"}
	return strconv.FormatFloat(float64(n)/float64(div), 'f', 1, 64) + " " + units[exp]
}
