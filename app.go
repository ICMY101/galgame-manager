// Package main 应用核心：持有存储、设置、扫描任务与封面缓存。
package main

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image/png"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"galgame-manager/internal/meta"
	"galgame-manager/internal/model"
	"galgame-manager/internal/scan"
	"galgame-manager/internal/store"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// App 是应用的运行时容器，同时暴露给前端（Wails 绑定）使用。
type App struct {
	ctx context.Context

	dataDir   string
	coverDir  string
	dbPath    string
	settingsP string

	store *store.Store
	mu    sync.RWMutex
	set   model.Settings

	scan *scanTask
}

// NewApp 创建 App 实例。
func NewApp() *App {
	root := ""
	// 便携模式：把数据放在 exe 同级的 data 目录
	if env := os.Getenv("GALGAME_DATA_DIR"); env != "" {
		root = env
	} else if exe, err := os.Executable(); err == nil {
		portable := filepath.Join(filepath.Dir(exe), "portable.txt")
		if _, err := os.Stat(portable); err == nil {
			root = filepath.Join(filepath.Dir(exe), "data")
		}
	}
	if root == "" {
		base, err := os.UserConfigDir()
		if err != nil || base == "" {
			base = "."
		}
		root = filepath.Join(base, "GalgameManager")
	}
	a := &App{
		dataDir:   filepath.Join(root, "data"),
		coverDir:  filepath.Join(root, "covers"),
		dbPath:    filepath.Join(root, "data", "library.db"),
		settingsP: filepath.Join(root, "data", "settings.json"),
	}
	_ = os.MkdirAll(a.dataDir, 0o755)
	_ = os.MkdirAll(a.coverDir, 0o755)

	st, err := store.Open(a.dbPath)
	if err != nil {
		panic(err)
	}
	a.store = st
	a.set = a.loadSettings()
	meta.Warmup()
	return a
}

// Startup 由 Wails 在启动时调用。
func (a *App) Startup(ctx context.Context) {
	a.ctx = ctx
	// 支持把文件夹直接拖进窗口
	wailsruntime.OnFileDrop(ctx, func(x, y int, paths []string) {
		if len(paths) > 0 {
			wailsruntime.EventsEmit(ctx, "folder-dropped", paths)
		}
	})
}

// Shutdown 由 Wails 在退出时调用。
func (a *App) Shutdown(ctx context.Context) {
	if a.store != nil {
		_ = a.store.Close()
	}
}

// ---------------------------------------------------------------- 设置

func (a *App) loadSettings() model.Settings {
	s := model.DefaultSettings()
	data, err := os.ReadFile(a.settingsP)
	if err != nil {
		return s
	}
	if err := json.Unmarshal(data, &s); err != nil {
		return model.DefaultSettings()
	}
	if s.WindowFlags == nil {
		s.WindowFlags = map[string]string{}
	}
	if s.TranslationTools == nil {
		s.TranslationTools = []model.Tool{}
	}
	if s.LibraryRoots == nil {
		s.LibraryRoots = []string{}
	}
	return s
}

// GetSettings 返回当前设置。
func (a *App) GetSettings() model.Settings {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.set
}

// SaveSettings 保存设置。
func (a *App) SaveSettings(s model.Settings) error {
	a.mu.Lock()
	a.set = s
	data, _ := json.MarshalIndent(s, "", "  ")
	a.mu.Unlock()
	return os.WriteFile(a.settingsP, data, 0o644)
}

// ---------------------------------------------------------------- 封面

func (a *App) coverPathFor(seed string) string {
	sum := sha1.Sum([]byte(seed))
	return filepath.Join(a.coverDir, hex.EncodeToString(sum[:])+".png")
}

// cacheExeIcon 把 exe 图标导出为 PNG 作为封面兜底，返回路径。
func (a *App) cacheExeIcon(exe string) string {
	if exe == "" {
		return ""
	}
	img := scan.ExeIcon(exe)
	if img == nil {
		return ""
	}
	path := a.coverPathFor("icon:" + strings.ToLower(exe))
	if st, err := os.Stat(path); err == nil && st.Size() > 512 {
		return path
	}
	f, err := os.Create(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		return ""
	}
	return path
}

// ---------------------------------------------------------------- 扫描任务

type scanTask struct {
	mu         sync.Mutex
	running    bool
	done       int
	total      int
	current    string
	stage      string
	items      []model.ScanCandidate
	err        string
	started    time.Time
	cancel     context.CancelFunc
	canceled   bool
	coverDone  int
	coverTotal int
}

// StartScan 开始扫描目录。返回错误表示已有任务在跑。
func (a *App) StartScan(root string, recursive, fetchMeta bool) error {
	if strings.TrimSpace(root) == "" {
		return fmt.Errorf("请先选择游戏根目录")
	}
	if st, err := os.Stat(root); err != nil || !st.IsDir() {
		return fmt.Errorf("目录不存在: %s", root)
	}

	a.mu.Lock()
	running := a.scan != nil && a.scan.isRunning()
	if running {
		a.mu.Unlock()
		return fmt.Errorf("已有扫描任务在进行中")
	}
	ctx, cancel := context.WithCancel(context.Background())
	task := &scanTask{
		running: true,
		stage:   "discovering",
		started: time.Now(),
		cancel:  cancel,
	}
	a.scan = task
	a.mu.Unlock()

	go a.runScan(ctx, task, root, recursive, fetchMeta)
	return nil
}

func (a *App) currentScan() *scanTask {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.scan
}

func (t *scanTask) isRunning() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.running
}

func (a *App) runScan(ctx context.Context, task *scanTask, root string, recursive, fetchMeta bool) {
	defer func() {
		task.mu.Lock()
		task.running = false
		task.stage = "done"
		task.mu.Unlock()
	}()

	depth := 1
	if recursive {
		depth = scan.MaxDepth
	}

	task.mu.Lock()
	task.stage = "scanning"
	task.mu.Unlock()

	dirs := scan.Discover(root, depth, true)
	known, _ := a.store.AllFolders()

	task.mu.Lock()
	task.total = len(dirs)
	task.mu.Unlock()

	if len(dirs) == 0 {
		task.mu.Lock()
		task.err = "没有在该目录下找到可识别的游戏（未发现含启动程序的子目录）"
		task.mu.Unlock()
		return
	}

	// 阶段一：识别启动文件与封面
	for _, d := range dirs {
		if ctx.Err() != nil {
			task.mu.Lock()
			task.canceled = true
			task.mu.Unlock()
			return
		}
		cover := d.Cover
		if cover == "" {
			cover = a.cacheExeIcon(d.Exe)
		}
		title := d.Title
		if d.Exe != "" {
			if t := scan.BestTitle(filepath.Dir(d.Exe), d.Exe, title); t != "" {
				title = t
			}
		}

		coverStatus := "none"
		if cover != "" {
			coverStatus = "ok"
		}
		cand := model.ScanCandidate{
			Folder:       d.Folder,
			Title:        title,
			ExePath:      d.Exe,
			CoverPath:    cover,
			SizeBytes:    d.SizeBytes,
			ExeCount:     d.ExeCount,
			AlreadyInLib: known[strings.ToLower(d.Folder)],
			MetaStatus:   "pending",
			CoverStatus:  coverStatus,
		}
		task.mu.Lock()
		task.items = append(task.items, cand)
		task.done++
		task.current = title
		task.mu.Unlock()
	}

	// 阶段二：并发抓取网络元数据
	if !fetchMeta {
		task.mu.Lock()
		for i := range task.items {
			task.items[i].MetaStatus = "skipped"
			task.items[i].CoverStatus = "skipped"
		}
		task.mu.Unlock()
		return
	}

	task.mu.Lock()
	task.stage = "metadata"
	task.mu.Unlock()

	src := a.GetSettings().MetadataSource
	sem := make(chan struct{}, 4)
	var wg sync.WaitGroup

	for i := range dirs {
		if ctx.Err() != nil {
			break
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(idx int) {
			defer wg.Done()
			defer func() { <-sem }()

			task.mu.Lock()
			if idx >= len(task.items) {
				task.mu.Unlock()
				return
			}
			query := task.items[idx].Title
			task.current = query
			task.mu.Unlock()

			t0 := time.Now()
			res, err := meta.Fetch(ctx, query, src)

			task.mu.Lock()
			if idx >= len(task.items) {
				task.mu.Unlock()
				return
			}
			it := &task.items[idx]
			switch {
			case err != nil:
				it.MetaStatus = "error"
				it.MetaError = err.Error()
			case res == nil:
				it.MetaStatus = "none"
			default:
				it.MetaStatus = "ok"
				it.Source = res.Source
				it.SourceID = res.SourceID
				it.Developer = res.Developer
				it.ReleaseDate = res.ReleaseDate
				it.Description = res.Description
				it.Rating = res.Rating
				it.Tags = strings.Join(res.Tags, ",")
				if res.Title != "" && it.Title == "" {
					it.Title = res.Title
				}
				// 本地没有封面时才用网络封面（下载放到下一阶段，避免拖慢扫描）
				if it.CoverPath == "" && res.CoverURL != "" {
					it.CoverURL = res.CoverURL
					it.CoverStatus = "pending"
				}
			}
			task.mu.Unlock()

			log.Printf("[scan] meta %-20s %5.2fs src=%s", query, time.Since(t0).Seconds(), it.Source)
		}(i)
	}
	wg.Wait()

	if ctx.Err() != nil {
		return
	}

	// 阶段三：并发下载封面（网络慢时只影响封面，不阻塞元数据结果）
	task.mu.Lock()
	var pending []int
	for i := range task.items {
		if task.items[i].CoverStatus == "pending" && task.items[i].CoverURL != "" {
			pending = append(pending, i)
		}
	}
	task.stage = "covers"
	task.coverTotal = len(pending)
	task.coverDone = 0
	task.mu.Unlock()

	if len(pending) == 0 {
		return
	}

	coverSem := make(chan struct{}, 4)
	var cwg sync.WaitGroup
	for _, idx := range pending {
		if ctx.Err() != nil {
			break
		}
		cwg.Add(1)
		coverSem <- struct{}{}
		go func(i int) {
			defer cwg.Done()
			defer func() { <-coverSem }()

			task.mu.Lock()
			url := task.items[i].CoverURL
			task.current = task.items[i].Title
			task.mu.Unlock()

			cctx, cancel := context.WithTimeout(ctx, 12*time.Second)
			local, err := meta.DownloadCover(cctx, url, a.coverDir)
			cancel()

			task.mu.Lock()
			if err == nil && local != "" {
				task.items[i].CoverPath = local
				task.items[i].CoverStatus = "ok"
			} else {
				task.items[i].CoverStatus = "failed"
			}
			task.coverDone++
			task.mu.Unlock()
		}(idx)
	}
	cwg.Wait()
}

// ScanStatus 返回扫描进度快照。
func (a *App) ScanStatus() model.ScanProgress {
	t := a.currentScan()
	if t == nil {
		return model.ScanProgress{Stage: "idle", Items: []model.ScanCandidate{}}
	}
	return t.snapshot()
}

func (t *scanTask) snapshot() model.ScanProgress {
	t.mu.Lock()
	defer t.mu.Unlock()
	items := make([]model.ScanCandidate, len(t.items))
	copy(items, t.items)
	return model.ScanProgress{
		Running:    t.running,
		Done:       t.done,
		Total:      t.total,
		Current:    t.current,
		Stage:      t.stage,
		Items:      items,
		Error:      t.err,
		Elapsed:    time.Since(t.started).Seconds(),
		CoverDone:  t.coverDone,
		CoverTotal: t.coverTotal,
	}
}

// CancelScan 取消正在进行的扫描。
func (a *App) CancelScan() {
	if t := a.currentScan(); t != nil && t.cancel != nil {
		t.cancel()
	}
}

// ---------------------------------------------------------------- 导入

// ImportItem 是导入请求中的一项。
type ImportItem struct {
	Folder     string   `json:"folder"`
	Title      string   `json:"title"`
	ExePath    string   `json:"exePath"`
	CoverPath  string   `json:"coverPath"`
	CoverURL   string   `json:"coverUrl"`
	Developer  string   `json:"developer"`
	Release    string   `json:"releaseDate"`
	Desc       string   `json:"description"`
	Rating     float64  `json:"rating"`
	Tags       string   `json:"tags"`
	SizeBytes  int64    `json:"sizeBytes"`
	Source     string   `json:"source"`
	SourceID   string   `json:"sourceId"`
	Categories []string `json:"categories"`
}

// ImportResult 是导入结果。
type ImportResult struct {
	Imported int      `json:"imported"`
	Updated  int      `json:"updated"`
	Errors   []string `json:"errors"`
}

// ImportGames 把扫描结果写入库中。
func (a *App) ImportGames(items []ImportItem) ImportResult {
	res := ImportResult{Errors: []string{}}
	now := time.Now().Format("2006-01-02 15:04")

	// 兜底：扫描阶段没下完的封面，在导入时补下（并发、限时，失败不影响导入）
	var needCover []int
	for i, it := range items {
		if it.CoverPath == "" && it.CoverURL != "" {
			needCover = append(needCover, i)
		}
	}
	if len(needCover) > 0 {
		sem := make(chan struct{}, 4)
		var wg sync.WaitGroup
		for _, idx := range needCover {
			wg.Add(1)
			sem <- struct{}{}
			go func(i int) {
				defer wg.Done()
				defer func() { <-sem }()
				ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
				defer cancel()
				if local, err := meta.DownloadCover(ctx, items[i].CoverURL, a.coverDir); err == nil {
					items[i].CoverPath = local
				}
			}(idx)
		}
		wg.Wait()
	}

	for _, it := range items {
		if strings.TrimSpace(it.Folder) == "" {
			continue
		}
		existing, err := a.store.GetByFolder(it.Folder)
		if err != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("%s: %v", it.Title, err))
			continue
		}
		if existing != nil {
			// 已存在：只补齐缺失字段，不覆盖用户已编辑的内容
			if it.Title != "" {
				existing.Title = it.Title
			}
			if it.ExePath != "" {
				existing.ExePath = it.ExePath
			}
			if it.CoverPath != "" {
				existing.CoverPath = it.CoverPath
				existing.CoverVersion++
			}
			if it.CoverURL != "" {
				existing.CoverURL = it.CoverURL
			}
			if existing.Developer == "" {
				existing.Developer = it.Developer
			}
			if existing.Description == "" {
				existing.Description = it.Desc
			}
			if existing.CommunityRating == 0 {
				existing.CommunityRating = it.Rating
			}
			if existing.Tags == "" {
				existing.Tags = it.Tags
			}
			if existing.ReleaseDate == "" {
				existing.ReleaseDate = it.Release
			}
			if it.SizeBytes > 0 {
				existing.SizeBytes = it.SizeBytes
			}
			if err := a.store.Update(existing); err != nil {
				res.Errors = append(res.Errors, fmt.Sprintf("%s: %v", it.Title, err))
				continue
			}
			if len(it.Categories) > 0 {
				_ = a.store.SetGameCategories(existing.ID, it.Categories)
			}
			res.Updated++
			continue
		}

		g := &model.Game{
			Folder:          it.Folder,
			Title:           firstNonEmpty(it.Title, filepath.Base(it.Folder)),
			ExePath:         it.ExePath,
			CoverPath:       it.CoverPath,
			CoverURL:        it.CoverURL,
			Developer:       it.Developer,
			ReleaseDate:     it.Release,
			Description:     it.Desc,
			CommunityRating: it.Rating,
			Tags:            it.Tags,
			Status:          model.StatusUnplayed,
			DateAdded:       now,
			SizeBytes:       it.SizeBytes,
			Source:          it.Source,
			SourceID:        it.SourceID,
			WindowMode:      "default",
			CoverVersion:    1,
		}
		id, err := a.store.Upsert(g)
		if err != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("%s: %v", it.Title, err))
			continue
		}
		if len(it.Categories) > 0 && id > 0 {
			_ = a.store.SetGameCategories(id, it.Categories)
		}
		res.Imported++
	}
	return res
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// ---------------------------------------------------------------- 列表与详情

// GameDetail 是详情面板需要的完整数据。
type GameDetail struct {
	Game       *model.Game `json:"game"`
	Categories []string    `json:"categories"`
	ExeOptions []string    `json:"exeOptions"`
	Exists     bool        `json:"exists"`
	FolderSize int64       `json:"folderSize"`
}

// GetGame 返回单款游戏的详情。
func (a *App) GetGame(id int64) (*GameDetail, error) {
	g, err := a.store.Get(id)
	if err != nil || g == nil {
		return nil, fmt.Errorf("游戏不存在")
	}
	cats, _ := a.store.GameCategories(id)
	exes := []string{}
	if g.ExePath != "" {
		exes = scan.CandidateExes(g.Folder, 2)
	}
	_, statErr := os.Stat(g.Folder)
	return &GameDetail{
		Game:       g,
		Categories: cats,
		ExeOptions: exes,
		Exists:     statErr == nil,
	}, nil
}

// RefreshMetadata 重新抓取某款游戏的元数据。
func (a *App) RefreshMetadata(id int64, query string) (*meta.Result, error) {
	g, err := a.store.Get(id)
	if err != nil || g == nil {
		return nil, fmt.Errorf("游戏不存在")
	}
	if strings.TrimSpace(query) == "" {
		query = g.Title
	}
	src := a.GetSettings().MetadataSource
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	res, err := meta.Fetch(ctx, query, src)
	if err != nil {
		return nil, err
	}
	if res == nil {
		return nil, fmt.Errorf("没有找到「%s」的匹配条目", query)
	}

	g.Developer = res.Developer
	g.ReleaseDate = res.ReleaseDate
	g.Description = res.Description
	g.CommunityRating = res.Rating
	g.Source = res.Source
	g.SourceID = res.SourceID
	if len(res.Tags) > 0 {
		g.Tags = strings.Join(res.Tags, ",")
	}
	if res.OriginalTitle != "" {
		g.OriginalTitle = res.OriginalTitle
	}
	if g.CoverPath == "" && res.CoverURL != "" {
		g.CoverURL = res.CoverURL
		if local, derr := meta.DownloadCover(ctx, res.CoverURL, a.coverDir); derr == nil {
			g.CoverPath = local
			g.CoverVersion++
		}
	}
	if err := a.store.Update(g); err != nil {
		return nil, err
	}
	return res, nil
}

// SetCover 手动指定封面图片。
func (a *App) SetCover(id int64, path string) error {
	g, err := a.store.Get(id)
	if err != nil || g == nil {
		return fmt.Errorf("游戏不存在")
	}
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("文件不存在")
	}
	g.CoverPath = path
	g.CoverVersion++
	return a.store.Update(g)
}

// ClearCover 清除封面（回到占位图）。
func (a *App) ClearCover(id int64) error {
	g, err := a.store.Get(id)
	if err != nil || g == nil {
		return fmt.Errorf("游戏不存在")
	}
	g.CoverPath = ""
	g.CoverURL = ""
	g.CoverVersion++
	return a.store.Update(g)
}

// RegenerateCoverFromExe 从启动文件重新提取图标当封面。
func (a *App) RegenerateCoverFromExe(id int64) (string, error) {
	g, err := a.store.Get(id)
	if err != nil || g == nil {
		return "", fmt.Errorf("游戏不存在")
	}
	if g.ExePath == "" {
		return "", fmt.Errorf("未设置启动文件")
	}
	_ = os.Remove(a.coverPathFor("icon:" + strings.ToLower(g.ExePath)))
	path := a.cacheExeIcon(g.ExePath)
	if path == "" {
		return "", fmt.Errorf("无法从该文件提取图标")
	}
	g.CoverPath = path
	g.CoverVersion++
	return path, a.store.Update(g)
}

// ListExes 列出某目录下所有候选启动文件。
func (a *App) ListExes(folder string) []string {
	return scan.CandidateExes(folder, 2)
}

// ---------------------------------------------------------------- 侧边栏聚合

// Facets 返回侧边栏统计。
func (a *App) Facets() (*model.Facets, error) {
	return a.store.Facets()
}

// AllCategories 返回分类名列表。
func (a *App) AllCategories() []string {
	cats, _ := a.store.ListCategories()
	out := make([]string, 0, len(cats))
	for _, c := range cats {
		out = append(out, c.Name)
	}
	sort.Strings(out)
	return out
}
