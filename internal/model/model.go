// Package model 定义领域数据结构。
package model

import (
	"fmt"
	"strings"
)

// 游玩状态
const (
	StatusUnplayed  = "unplayed"
	StatusPlaying   = "playing"
	StatusPlayed    = "played"
	StatusAbandoned = "abandoned"
)

// Game 是一款 galgame 的完整记录。
type Game struct {
	ID              int64   `json:"id"`
	Folder          string  `json:"folder"`
	Title           string  `json:"title"`
	OriginalTitle   string  `json:"originalTitle"`
	ExePath         string  `json:"exePath"`
	CoverPath       string  `json:"coverPath"`
	CoverURL        string  `json:"coverUrl"`
	Developer       string  `json:"developer"`
	ReleaseDate     string  `json:"releaseDate"`
	Description     string  `json:"description"`
	Rating          float64 `json:"rating"`          // 我的评分 0-10
	CommunityRating float64 `json:"communityRating"` // 网络评分 0-10
	Tags            string  `json:"tags"`
	Status          string  `json:"status"`
	Favorite        bool    `json:"favorite"`
	LastPlayed      string  `json:"lastPlayed"`
	PlayCount       int     `json:"playCount"`
	PlayTime        int64   `json:"playTime"` // 累计秒数
	DateAdded       string  `json:"dateAdded"`
	LaunchArgs      string  `json:"launchArgs"`
	WindowMode      string  `json:"windowMode"`
	SavePaths       string  `json:"savePaths"`
	SizeBytes       int64   `json:"sizeBytes"`
	Source          string  `json:"source"`    // bangumi / vndb / manual
	SourceID        string  `json:"sourceId"`
	CoverVersion    int64   `json:"coverVersion"` // 封面缓存版本号，用于前端破缓存
	Notes           string  `json:"notes"`
}

// TagList 返回去重后的标签切片。
func (g *Game) TagList() []string {
	return splitClean(g.Tags)
}

// PlayTimeText 把秒数格式化成「x 小时 y 分」。
func (g *Game) PlayTimeText() string {
	h := g.PlayTime / 3600
	m := (g.PlayTime % 3600) / 60
	if h > 0 {
		return fmt.Sprintf("%d 小时 %d 分", h, m)
	}
	return fmt.Sprintf("%d 分钟", m)
}

func splitClean(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	seen := map[string]bool{}
	out := []string{}
	for _, part := range strings.Split(s, ",") {
		p := strings.TrimSpace(part)
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	return out
}

// ScanCandidate 是扫描阶段发现的候选游戏（尚未入库）。
type ScanCandidate struct {
	Folder          string  `json:"folder"`
	Title           string  `json:"title"`
	ExePath         string  `json:"exePath"`
	CoverPath       string  `json:"coverPath"`
	CoverURL        string  `json:"coverUrl"`
	Developer       string  `json:"developer"`
	ReleaseDate     string  `json:"releaseDate"`
	Description     string  `json:"description"`
	Rating          float64 `json:"rating"`
	Tags            string  `json:"tags"`
	SizeBytes       int64   `json:"sizeBytes"`
	Source          string  `json:"source"`
	SourceID        string  `json:"sourceId"`
	ExeCount        int     `json:"exeCount"`
	AlreadyInLib    bool    `json:"alreadyInLib"`
	MetaStatus      string  `json:"metaStatus"` // pending / ok / none / error / skipped
	MetaError       string  `json:"metaError"`
	CoverStatus     string  `json:"coverStatus"` // pending / ok / failed / skipped
	CoverVersion    int64   `json:"coverVersion"`
}

// ScanProgress 是扫描进度快照（前端轮询）。
type ScanProgress struct {
	Running    bool            `json:"running"`
	Done       int             `json:"done"`
	Total      int             `json:"total"`
	Current    string          `json:"current"`
	Stage      string          `json:"stage"` // discovering / scanning / metadata / covers / done
	Items      []ScanCandidate `json:"items"`
	Error      string          `json:"error"`
	Elapsed    float64         `json:"elapsed"`
	CoverDone  int             `json:"coverDone"`
	CoverTotal int             `json:"coverTotal"`
}

// Facets 是侧边栏需要的聚合数据。
type Facets struct {
	Categories []CategoryCount `json:"categories"`
	Tags       []TagCount      `json:"tags"`
	Statuses   []CategoryCount `json:"statuses"`
	Total      int             `json:"total"`
	Favorites  int             `json:"favorites"`
}

type CategoryCount struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

type TagCount struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// Settings 是全局设置。
type Settings struct {
	MetadataSource   string            `json:"metadataSource"`   // bangumi / vndb
	AutoFetchMeta    bool              `json:"autoFetchMeta"`
	ScanRecursive    bool              `json:"scanRecursive"`
	PreferLocalCover bool              `json:"preferLocalCover"`
	WindowFlags      map[string]string `json:"windowFlags"`
	TranslationTools []Tool            `json:"translationTools"`
	AutoStartTools   bool              `json:"autoStartTools"`
	LibraryRoots     []string          `json:"libraryRoots"`
	AccentColor      string            `json:"accentColor"`
	GridSize         string            `json:"gridSize"` // sm / md / lg
}

type Tool struct {
	Name string `json:"name"`
	Path string `json:"path"`
	Args string `json:"args"`
	Auto bool   `json:"auto"`
}

// DefaultSettings 返回默认设置。
func DefaultSettings() Settings {
	return Settings{
		MetadataSource:   "bangumi",
		AutoFetchMeta:    true,
		ScanRecursive:    true,
		PreferLocalCover: true,
		WindowFlags:      map[string]string{"windowed": "", "fullscreen": "", "borderless": ""},
		TranslationTools: []Tool{},
		AutoStartTools:   false,
		LibraryRoots:     []string{},
		AccentColor:      "#7c6cf6",
		GridSize:         "md",
	}
}

// Query 是游戏列表查询条件。
type Query struct {
	Keyword  string `json:"keyword"`
	Status   string `json:"status"`
	Tag      string `json:"tag"`
	Category string `json:"category"`
	Favorite bool   `json:"favorite"`
	Sort     string `json:"sort"` // title / added / rating / developer / lastplayed / playtime
}
