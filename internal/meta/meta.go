// Package meta 从 Bangumi / VNDB 抓取游戏元数据与封面。
package meta

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// UserAgent 是访问公共 API 时使用的标识。
const UserAgent = "GalgameManager/1.0 (Windows desktop app)"

var httpClient = &http.Client{
	Timeout: 12 * time.Second,
	Transport: &http.Transport{
		DialContext:           (&net.Dialer{Timeout: 4 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		TLSHandshakeTimeout:   4 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second,
		MaxIdleConnsPerHost:   4,
		Proxy:                 http.ProxyFromEnvironment,
	},
}

// 数据源熔断：某个源出现网络级故障时，在一段时间内直接跳过，
// 避免每次请求都白等超时。
var (
	breakerMu sync.Mutex
	downUntil = map[string]time.Time{}
)

const breakerCooldown = 5 * time.Minute

func sourceDown(name string) bool {
	breakerMu.Lock()
	defer breakerMu.Unlock()
	t, ok := downUntil[name]
	return ok && time.Now().Before(t)
}

func markSourceDown(name string) {
	breakerMu.Lock()
	defer breakerMu.Unlock()
	downUntil[name] = time.Now().Add(breakerCooldown)
}

func markSourceOK(name string) {
	breakerMu.Lock()
	defer breakerMu.Unlock()
	delete(downUntil, name)
}

// Warmup 在后台探测两个数据源的连通性，把不可达的源提前熔断，
// 这样首次扫描不必为每个游戏白等超时。
func Warmup() {
	probe := func(name, url string) {
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		defer cancel()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return
		}
		req.Header.Set("User-Agent", UserAgent)
		resp, err := httpClient.Do(req)
		if err != nil {
			markSourceDown(name)
			return
		}
		io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
		resp.Body.Close()
		markSourceOK(name)
	}
	go probe("bangumi", "https://api.bgm.tv/")
	go probe("vndb", "https://api.vndb.org/kana/schema")
}

// SourceStatus 返回各数据源当前是否可用（供设置界面展示）。
func SourceStatus() map[string]bool {
	return map[string]bool{
		"bangumi": !sourceDown("bangumi"),
		"vndb":    !sourceDown("vndb"),
	}
}

// Result 是归一化后的元数据。
type Result struct {
	Title         string   `json:"title"`
	OriginalTitle string   `json:"originalTitle"`
	Developer     string   `json:"developer"`
	ReleaseDate   string   `json:"releaseDate"`
	Description   string   `json:"description"`
	CoverURL      string   `json:"coverUrl"`
	Rating        float64  `json:"rating"`
	Tags          []string `json:"tags"`
	Source        string   `json:"source"`
	SourceID      string   `json:"sourceId"`
}

// ---------------------------------------------------------------- 工具

var (
	htmlTagRe = regexp.MustCompile(`<[^>]+>`)
	spaceRe   = regexp.MustCompile(`[ \t]+`)
)

func stripHTML(s string) string {
	s = strings.ReplaceAll(s, "<br />", "\n")
	s = strings.ReplaceAll(s, "<br/>", "\n")
	s = strings.ReplaceAll(s, "<br>", "\n")
	s = htmlTagRe.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimSpace(spaceRe.ReplaceAllString(l, " "))
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func postJSON(ctx context.Context, url string, payload any, out any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", UserAgent)

	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusTooManyRequests {
		return fmt.Errorf("请求过于频繁（429），请稍后再试")
	}
	if resp.StatusCode >= 400 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 200))
		return fmt.Errorf("接口返回 %d: %s", resp.StatusCode, string(snippet))
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// ---------------------------------------------------------------- Bangumi

type bgmSubject struct {
	ID      int    `json:"id"`
	Name    string `json:"name"`
	NameCN  string `json:"name_cn"`
	Summary string `json:"summary"`
	Date    string `json:"date"`
	Images  struct {
		Large  string `json:"large"`
		Common string `json:"common"`
		Medium string `json:"medium"`
	} `json:"images"`
	Rating struct {
		Score float64 `json:"score"`
		Total int     `json:"total"`
	} `json:"rating"`
	Tags []struct {
		Name  string `json:"name"`
		Count int    `json:"count"`
	} `json:"tags"`
	Infobox []struct {
		Key   string          `json:"key"`
		Value json.RawMessage `json:"value"`
	} `json:"infobox"`
}

// SearchBangumi 在 Bangumi 搜索游戏条目。
func SearchBangumi(ctx context.Context, query string) (*Result, error) {
	payload := map[string]any{
		"keyword": query,
		"sort":    "match",
		"filter": map[string]any{
			"type": []int{4}, // 4 = 游戏
			"nsfw": false,
		},
	}
	var resp struct {
		Data []bgmSubject `json:"data"`
	}
	if err := postJSON(ctx, "https://api.bgm.tv/v0/search/subjects?limit=8", payload, &resp); err != nil {
		return nil, fmt.Errorf("Bangumi 搜索失败: %w", err)
	}
	if len(resp.Data) == 0 {
		return nil, nil
	}
	return resultFromBangumi(pickBangumi(resp.Data, query)), nil
}

// resultFromBangumi 把 Bangumi 条目转换成统一的 Result。
func resultFromBangumi(best bgmSubject) *Result {
	r := &Result{
		Title:         firstNonEmpty(best.NameCN, best.Name),
		OriginalTitle: best.Name,
		Description:   stripHTML(best.Summary),
		Rating:        best.Rating.Score,
		ReleaseDate:   best.Date,
		Source:        "bangumi",
		SourceID:      fmt.Sprintf("%d", best.ID),
	}
	r.CoverURL = firstNonEmpty(best.Images.Large, best.Images.Common, best.Images.Medium)

	// 开发商：优先从 infobox 的「开发」字段取，其次从标签里猜
	for _, ib := range best.Infobox {
		k := strings.ToLower(ib.Key)
		if k == "开发" || k == "開発" || k == "developer" || k == "出版社" {
			if v := infoboxValue(ib.Value); v != "" {
				r.Developer = v
				break
			}
		}
	}
	if r.Developer == "" {
		for _, t := range best.Tags {
			if strings.HasSuffix(t.Name, "社") || strings.Contains(t.Name, "制作") {
				r.Developer = t.Name
				break
			}
		}
	}

	for i, t := range best.Tags {
		if i >= 12 {
			break
		}
		r.Tags = append(r.Tags, t.Name)
	}
	return r
}

func infoboxValue(raw json.RawMessage) string {
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return strings.TrimSpace(s)
	}
	var arr []struct {
		V string `json:"v"`
	}
	if err := json.Unmarshal(raw, &arr); err == nil && len(arr) > 0 {
		return strings.TrimSpace(arr[0].V)
	}
	return ""
}

func pickBangumi(items []bgmSubject, query string) bgmSubject {
	q := strings.ToLower(strings.TrimSpace(query))
	for _, it := range items {
		if strings.ToLower(it.NameCN) == q || strings.ToLower(it.Name) == q {
			return it
		}
	}
	// 退而求其次：标题互相包含且评分人数最多
	var best bgmSubject
	bestScore := -1.0
	for _, it := range items {
		name := strings.ToLower(firstNonEmpty(it.NameCN, it.Name))
		score := float64(it.Rating.Total) / 100.0
		if strings.Contains(name, q) || strings.Contains(q, name) {
			score += 5
		}
		if score > bestScore {
			best, bestScore = it, score
		}
	}
	return best
}

// ---------------------------------------------------------------- VNDB

type vndbVN struct {
	ID          string  `json:"id"`
	Title       string  `json:"title"`
	Alttitle    string  `json:"alttitle"`
	Released    string  `json:"released"`
	Description string  `json:"description"`
	Rating      float64 `json:"rating"`
	Image       struct {
		URL string `json:"url"`
	} `json:"image"`
	Developers []struct {
		Name string `json:"name"`
	} `json:"developers"`
	Tags []struct {
		Name   string  `json:"name"`
		Rating float64 `json:"rating"`
	} `json:"tags"`
}

// SearchVNDB 在 VNDB 搜索视觉小说。
func SearchVNDB(ctx context.Context, query string) (*Result, error) {
	payload := map[string]any{
		"filters": []any{"search", "=", query},
		"fields":  "title,alttitle,released,image.url,description,rating,developers.name,tags.name,tags.rating",
		"sort":    "searchrank",
		"results": 8,
	}
	var resp struct {
		Results []vndbVN `json:"results"`
	}
	if err := postJSON(ctx, "https://api.vndb.org/kana/vn", payload, &resp); err != nil {
		return nil, fmt.Errorf("VNDB 搜索失败: %w", err)
	}
	if len(resp.Results) == 0 {
		return nil, nil
	}

	q := strings.ToLower(strings.TrimSpace(query))
	best := resp.Results[0]
	for _, vn := range resp.Results {
		if strings.ToLower(vn.Title) == q || strings.ToLower(vn.Alttitle) == q {
			best = vn
			break
		}
	}

	r := &Result{
		Title:         best.Title,
		OriginalTitle: best.Alttitle,
		Description:   stripHTML(best.Description),
		CoverURL:      best.Image.URL,
		ReleaseDate:   best.Released,
		Source:        "vndb",
		SourceID:      best.ID,
	}
	// VNDB 评分是 0-100，换算成 0-10
	r.Rating = vndbRating(best.Rating)
	for _, d := range best.Developers {
		if d.Name != "" {
			if r.Developer != "" {
				r.Developer += " / "
			}
			r.Developer += d.Name
		}
	}
	// 标签按关联度排序，取关联度 >= 0.5 的前 10 个
	type tag struct {
		name string
		rel  float64
	}
	var tags []tag
	for _, t := range best.Tags {
		if t.Rating >= 0.5 {
			tags = append(tags, tag{t.Name, t.Rating})
		}
	}
	for i := 1; i < len(tags); i++ {
		for j := i; j > 0 && tags[j].rel > tags[j-1].rel; j-- {
			tags[j], tags[j-1] = tags[j-1], tags[j]
		}
	}
	for i, t := range tags {
		if i >= 10 {
			break
		}
		r.Tags = append(r.Tags, t.name)
	}
	return r, nil
}

// ---------------------------------------------------------------- 统一入口

// Fetch 按指定来源抓取元数据，失败时自动回退到另一个来源。
func Fetch(ctx context.Context, query, source string) (*Result, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("标题为空")
	}
	order := []string{"bangumi", "vndb"}
	if source == "vndb" {
		order = []string{"vndb", "bangumi"}
	}

	var lastErr error
	for _, s := range order {
		if sourceDown(s) {
			continue
		}
		var (
			res *Result
			err error
		)
		if s == "bangumi" {
			res, err = SearchBangumi(ctx, query)
		} else {
			res, err = SearchVNDB(ctx, query)
		}
		if err != nil {
			// 网络级错误 → 熔断该源；解析/业务错误 → 仅本次失败
			if isNetworkError(err) {
				markSourceDown(s)
			}
			lastErr = err
			continue
		}
		markSourceOK(s)
		if res != nil {
			return res, nil
		}
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, nil
}

// isNetworkError 判断是否是连接级失败（超时、DNS、拒绝连接）。
func isNetworkError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return true
	}
	msg := strings.ToLower(err.Error())
	for _, kw := range []string{"no such host", "connection refused", "i/o timeout",
		"tls handshake", "network is unreachable", "connection reset", "eof"} {
		if strings.Contains(msg, kw) {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------- 封面下载

// DownloadCover 下载封面到缓存目录，返回本地路径。
func DownloadCover(ctx context.Context, url, cacheDir string) (string, error) {
	if strings.TrimSpace(url) == "" {
		return "", fmt.Errorf("封面地址为空")
	}
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return "", err
	}
	sum := sha1.Sum([]byte(url))
	ext := strings.ToLower(filepath.Ext(strings.Split(url, "?")[0]))
	switch ext {
	case ".jpg", ".jpeg", ".png", ".webp", ".bmp":
	default:
		ext = ".jpg"
	}
	path := filepath.Join(cacheDir, hex.EncodeToString(sum[:])+ext)
	if st, err := os.Stat(path); err == nil && st.Size() > 1024 {
		return path, nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Referer", "https://bgm.tv/")

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("封面下载失败: HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 12<<20))
	if err != nil {
		return "", err
	}
	if len(data) < 1024 {
		return "", fmt.Errorf("封面文件异常（%d 字节）", len(data))
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, path); err != nil {
		return "", err
	}
	return path, nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// vndbRating 把 VNDB 的 0-100 分制换算成 0-10 分制。
func vndbRating(raw float64) float64 {
	if raw <= 0 {
		return 0
	}
	if raw > 10 {
		return raw / 10
	}
	return raw
}
