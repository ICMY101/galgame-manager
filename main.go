// Galgame 管理器与启动器 —— Go + Vue 桌面应用入口。
package main

import (
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"path"
	"strings"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

const appVersion = "1.0.0"

//go:embed all:frontend/dist
var embedded embed.FS

func main() {
	serveMode := flag.Bool("serve", false, "以浏览器模式运行（调试用）")
	port := flag.Int("port", 8765, "浏览器模式监听端口")
	flag.Parse()

	app := NewApp()
	handler, err := app.buildHandler()
	if err != nil {
		log.Fatalf("初始化失败: %v", err)
	}

	if *serveMode {
		addr := fmt.Sprintf("127.0.0.1:%d", *port)
		log.Printf("Galgame 管理器（调试模式）→ http://%s", addr)
		if err := http.ListenAndServe(addr, handler); err != nil {
			log.Fatal(err)
		}
		return
	}

	// 原生窗口：把整个请求链交给同一个 handler，
	// 保证桌面模式与浏览器模式跑的是完全一致的代码路径。
	err = wails.Run(&options.App{
		Title:            "Galgame 管理器",
		Width:            1380,
		Height:           880,
		MinWidth:         1040,
		MinHeight:        660,
		BackgroundColour: &options.RGBA{R: 14, G: 15, B: 19, A: 255},
		AssetServer: &assetserver.Options{
			Handler: handler,
		},
		OnStartup:  app.Startup,
		OnShutdown: app.Shutdown,
		Windows: &windows.Options{
			WebviewIsTransparent: false,
			WindowIsTranslucent:  false,
			Theme:                windows.Dark,
		},
	})
	if err != nil {
		log.Fatalf("启动失败: %v", err)
	}
}

// buildHandler 组装统一的 HTTP handler：/api 接口 + 内嵌前端静态资源 + SPA 回退。
func (a *App) buildHandler() (http.Handler, error) {
	sub, err := fs.Sub(embedded, "frontend/dist")
	if err != nil {
		return nil, fmt.Errorf("加载前端资源失败: %w", err)
	}

	api := a.APIRouter()
	a.attachDialogs(api)
	fileServer := http.FileServer(http.FS(sub))

	mux := http.NewServeMux()
	mux.Handle("/api/", api)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
		if p != "" && p != "." {
			if st, err := fs.Stat(sub, p); err == nil && !st.IsDir() {
				// 静态资源可长缓存，index.html 不缓存
				if strings.HasPrefix(p, "assets/") {
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				}
				fileServer.ServeHTTP(w, r)
				return
			}
		}
		index, err := fs.ReadFile(sub, "index.html")
		if err != nil {
			http.Error(w, "index.html 缺失", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(index)
	})
	return mux, nil
}

// attachDialogs 注册系统原生对话框接口。
// 桌面模式下调用 Wails 运行时；浏览器调试模式返回错误，由前端提示手动输入路径。
func (a *App) attachDialogs(m *http.ServeMux) {
	m.HandleFunc("POST /api/dialog/folder", func(w http.ResponseWriter, r *http.Request) {
		if a.ctx == nil {
			fail(w, fmt.Errorf("当前为浏览器调试模式，请直接输入路径"))
			return
		}
		dir, err := wailsruntime.OpenDirectoryDialog(a.ctx, wailsruntime.OpenDialogOptions{
			Title: "选择包含 galgame 的目录",
		})
		if err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"path": dir})
	})

	m.HandleFunc("POST /api/dialog/file", func(w http.ResponseWriter, r *http.Request) {
		if a.ctx == nil {
			fail(w, fmt.Errorf("当前为浏览器调试模式，请直接输入路径"))
			return
		}
		var body struct {
			Title  string `json:"title"`
			Filter string `json:"filter"` // 例如 "可执行文件|*.exe"
		}
		_ = json.NewDecoder(r.Body).Decode(&body)

		opts := wailsruntime.OpenDialogOptions{Title: body.Title}
		if body.Filter != "" {
			parts := strings.SplitN(body.Filter, "|", 2)
			if len(parts) == 2 {
				opts.Filters = []wailsruntime.FileFilter{
					{DisplayName: parts[0], Pattern: parts[1]},
				}
			}
		}
		p, err := wailsruntime.OpenFileDialog(a.ctx, opts)
		if err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"path": p})
	})
}
