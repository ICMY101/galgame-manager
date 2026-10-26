// Package main 的 HTTP API 层：前端通过 fetch 调用这些接口。
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"galgame-manager/internal/launch"
	"galgame-manager/internal/meta"
	"galgame-manager/internal/model"
	"galgame-manager/internal/scan"
)

// APIRouter 构建 API 路由表。
func (a *App) APIRouter() *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "version": appVersion})
	})

	// ---- 游戏列表 / 详情
	mux.HandleFunc("GET /api/games", func(w http.ResponseWriter, r *http.Request) {
		q := model.Query{
			Keyword:  r.URL.Query().Get("keyword"),
			Status:   r.URL.Query().Get("status"),
			Tag:      r.URL.Query().Get("tag"),
			Category: r.URL.Query().Get("category"),
			Favorite: r.URL.Query().Get("favorite") == "1",
			Sort:     r.URL.Query().Get("sort"),
		}
		games, err := a.store.Query(q)
		if err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, http.StatusOK, games)
	})

	mux.HandleFunc("GET /api/games/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := pathInt(r, "id")
		detail, err := a.GetGame(id)
		if err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, http.StatusOK, detail)
	})

	mux.HandleFunc("POST /api/games/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := pathInt(r, "id")
		g, err := a.store.Get(id)
		if err != nil || g == nil {
			fail(w, fmt.Errorf("游戏不存在"))
			return
		}
		var patch model.Game
		if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
			fail(w, err)
			return
		}
		patch.ID = id
		patch.Folder = g.Folder
		patch.PlayTime = g.PlayTime
		patch.PlayCount = g.PlayCount
		patch.LastPlayed = g.LastPlayed
		patch.DateAdded = g.DateAdded
		if patch.CoverPath != g.CoverPath {
			patch.CoverVersion = g.CoverVersion + 1
		}
		if err := a.store.Update(&patch); err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	})

	mux.HandleFunc("DELETE /api/games/{id}", func(w http.ResponseWriter, r *http.Request) {
		if err := a.store.Delete(pathInt(r, "id")); err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	})

	mux.HandleFunc("POST /api/games/{id}/favorite", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Value bool `json:"value"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if err := a.store.SetFavorite(pathInt(r, "id"), body.Value); err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	})

	mux.HandleFunc("POST /api/games/{id}/status", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Value string `json:"value"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if err := a.store.SetStatus(pathInt(r, "id"), body.Value); err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	})

	mux.HandleFunc("POST /api/games/{id}/categories", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Names []string `json:"names"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if err := a.store.SetGameCategories(pathInt(r, "id"), body.Names); err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	})

	// ---- 封面
	mux.HandleFunc("GET /api/cover/{id}", func(w http.ResponseWriter, r *http.Request) {
		g, err := a.store.Get(pathInt(r, "id"))
		if err != nil || g == nil || g.CoverPath == "" {
			http.NotFound(w, r)
			return
		}
		serveImage(w, r, g.CoverPath)
	})

	mux.HandleFunc("GET /api/preview", func(w http.ResponseWriter, r *http.Request) {
		serveImage(w, r, r.URL.Query().Get("path"))
	})

	mux.HandleFunc("POST /api/games/{id}/cover", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Path string `json:"path"`
			Mode string `json:"mode"` // file | exe | clear
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		id := pathInt(r, "id")
		var err error
		switch body.Mode {
		case "exe":
			_, err = a.RegenerateCoverFromExe(id)
		case "clear":
			err = a.ClearCover(id)
		default:
			err = a.SetCover(id, body.Path)
		}
		if err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	})

	// ---- 侧边栏
	mux.HandleFunc("GET /api/facets", func(w http.ResponseWriter, r *http.Request) {
		f, err := a.Facets()
		if err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, http.StatusOK, f)
	})

	mux.HandleFunc("GET /api/categories", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, a.AllCategories())
	})

	mux.HandleFunc("POST /api/categories", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Name    string `json:"name"`
			OldName string `json:"oldName"`
			Action  string `json:"action"` // add | rename | delete
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		var err error
		switch body.Action {
		case "rename":
			err = a.store.RenameCategory(body.OldName, body.Name)
		case "delete":
			err = a.store.DeleteCategory(body.Name)
		default:
			err = a.store.AddCategory(body.Name)
		}
		if err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	})

	// ---- 扫描
	mux.HandleFunc("POST /api/scan", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Root      string `json:"root"`
			Recursive bool   `json:"recursive"`
			FetchMeta bool   `json:"fetchMeta"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if err := a.StartScan(body.Root, body.Recursive, body.FetchMeta); err != nil {
			writeJSON(w, http.StatusConflict, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	})

	mux.HandleFunc("GET /api/scan/status", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, a.ScanStatus())
	})

	mux.HandleFunc("POST /api/scan/cancel", func(w http.ResponseWriter, r *http.Request) {
		a.CancelScan()
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	})

	mux.HandleFunc("POST /api/import", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Items []ImportItem `json:"items"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, http.StatusOK, a.ImportGames(body.Items))
	})

	// ---- 启动
	mux.HandleFunc("POST /api/games/{id}/launch", func(w http.ResponseWriter, r *http.Request) {
		id := pathInt(r, "id")
		g, err := a.store.Get(id)
		if err != nil || g == nil {
			fail(w, fmt.Errorf("游戏不存在"))
			return
		}
		if g.ExePath == "" {
			fail(w, fmt.Errorf("该游戏还没有设置启动文件"))
			return
		}
		if _, err := os.Stat(g.ExePath); err != nil {
			fail(w, fmt.Errorf("启动文件不存在，可能已被移动或删除"))
			return
		}
		settings := a.GetSettings()
		args := launch.BuildArgs(g, settings.WindowFlags)
		_, err = launch.Start(id, g.ExePath, args, filepath.Dir(g.ExePath),
			func(gameID int64, seconds int64) {
				_ = a.store.AddPlayTime(gameID, seconds)
				if g.Status == model.StatusUnplayed {
					_ = a.store.SetStatus(gameID, model.StatusPlaying)
				}
			})
		if err != nil {
			fail(w, err)
			return
		}
		_ = a.store.RecordLaunch(id)

		if settings.AutoStartTools {
			for _, t := range settings.TranslationTools {
				if t.Auto && t.Path != "" {
					_ = launch.StartTool(t.Path, t.Args, g)
				}
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	})

	mux.HandleFunc("POST /api/games/{id}/tool", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Index int `json:"index"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		g, err := a.store.Get(pathInt(r, "id"))
		if err != nil || g == nil {
			fail(w, fmt.Errorf("游戏不存在"))
			return
		}
		tools := a.GetSettings().TranslationTools
		if body.Index < 0 || body.Index >= len(tools) {
			fail(w, fmt.Errorf("工具不存在"))
			return
		}
		if err := launch.StartTool(tools[body.Index].Path, tools[body.Index].Args, g); err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	})

	mux.HandleFunc("POST /api/open", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Path string `json:"path"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if err := launch.OpenFolder(body.Path); err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	})

	// ---- 元数据
	mux.HandleFunc("POST /api/metadata/refresh", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			ID    int64  `json:"id"`
			Query string `json:"query"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		res, err := a.RefreshMetadata(body.ID, body.Query)
		if err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})

	mux.HandleFunc("GET /api/exes", func(w http.ResponseWriter, r *http.Request) {
		folder := r.URL.Query().Get("folder")
		exes := scan.CandidateExes(folder, 2)
		if exes == nil {
			exes = []string{}
		}
		writeJSON(w, http.StatusOK, exes)
	})

	// ---- 设置
	mux.HandleFunc("GET /api/settings", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, a.GetSettings())
	})

	mux.HandleFunc("GET /api/sources", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, meta.SourceStatus())
	})

	mux.HandleFunc("POST /api/settings", func(w http.ResponseWriter, r *http.Request) {
		var s model.Settings
		if err := json.NewDecoder(r.Body).Decode(&s); err != nil {
			fail(w, err)
			return
		}
		if err := a.SaveSettings(s); err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	})

	return mux
}

// ---------------------------------------------------------------- 辅助

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func fail(w http.ResponseWriter, err error) {
	writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
}

func pathInt(r *http.Request, name string) int64 {
	v, _ := strconv.ParseInt(r.PathValue(name), 10, 64)
	return v
}

var imageExts = map[string]string{
	".jpg": "image/jpeg", ".jpeg": "image/jpeg", ".png": "image/png",
	".webp": "image/webp", ".bmp": "image/bmp", ".gif": "image/gif",
}

// serveImage 只允许读取图片文件，避免任意文件读取。
func serveImage(w http.ResponseWriter, r *http.Request, path string) {
	if path == "" {
		http.NotFound(w, r)
		return
	}
	ext := strings.ToLower(filepath.Ext(path))
	mime, ok := imageExts[ext]
	if !ok {
		http.Error(w, "unsupported file type", http.StatusForbidden)
		return
	}
	st, err := os.Stat(path)
	if err != nil || st.IsDir() {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", mime)
	w.Header().Set("Cache-Control", "public, max-age=86400")
	http.ServeFile(w, r, path)
}

var errNotFound = errors.New("not found")
