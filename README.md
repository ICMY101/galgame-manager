# Galgame 管理器与启动器

一个 Windows 桌面应用，把散落在硬盘里的 galgame 收进一个库：自动识别启动文件和封面，
从 Bangumi / VNDB 补全简介、评分与标签，支持分类、标签、搜索、评分、游玩时长统计。

技术栈：**Vue 3 + Vite**（前端） · **Go + Wails v2**（后端与窗口） · **SQLite**（本地库）

---

## 功能

### 导入与识别

- **选一个根目录，一键扫描**：递归遍历（默认最多 6 层），把每个「含启动程序」的目录识别为一款游戏。
- **剪枝策略**：识别出游戏目录后不再往下钻，避免把 `bin/`、`data/` 当成独立游戏；
  `根目录/系列名/游戏名` 这种多层结构也能正确识别。
- **启动文件打分**：按「与目录名/标题的匹配度、层级、文件体积、常见启动器名」综合排序，
  并排除 `unins000`、`setup`、`vcredist`、`crashpad`、`UnityCrashHandler` 等安装器与运行库。
- **标题自动清洗**：`[Key] Little Busters! [v1.02]` → `Little Busters!`；
  还会读 exe 的版本信息（ProductName / FileDescription）作为标题，并过滤掉运行库描述。
- **封面三级兜底**：本地图片（按文件名、宽高比、分辨率打分）→ 网络封面 → 从启动文件提取图标。
- **增量导入**：已入库的游戏会标记出来，默认不勾选；再次导入只补空缺字段，不覆盖你的编辑。

### 元数据

- **Bangumi / VNDB 双源**，可设首选源，失败自动回退到另一个。
- 抓取简介、开发商、发售日、社区评分（统一换算成 10 分制）、标签、封面。
- **数据源熔断**：启动时探测连通性，不可达的源会被跳过 5 分钟，
  避免每个游戏都白等一次超时（设置页可查看两个源的当前状态）。

### 库管理

- 网格 / 列表两种视图，三档卡片尺寸；搜索覆盖标题、原名、开发商、简介、标签、备注。
- 6 种排序：标题、加入时间、我的评分、游玩时长、最近游玩、开发商。
- **自定义分类**（侧边栏右键新建/重命名/删除）+ **多标签**（VNDB/Bangumi 自动带入，可手动编辑）。
- 星级评分（0–10）、游玩状态（未游玩 / 游玩中 / 已通关 / 弃坑）、收藏。

### 启动

- 记录启动次数与最近游玩时间，**后台监控进程树**，游戏退出后自动累计游玩时长（秒级精度）。
- 首次启动自动把状态从「未游玩」改为「游玩中」。
- 每款游戏可配启动参数、窗口模式（默认/窗口化/全屏/无边框，对应参数在设置里统一配置）。
- **翻译工具集成**：配置 Textractor / LunaTranslator 等，参数支持 `{folder}`、`{exe}`、`{title}` 占位符，
  可在详情页一键启动，或勾选「启动游戏时自动启动」。
- 支持把文件夹直接拖进窗口开始导入。

---

## 快速开始

1. 运行 `GalgameManager.exe`（首次启动是空库）。
2. 点右上角「导入游戏」→「选择文件夹」，指向装着 galgame 的根目录。
3. 勾选「递归扫描」和「抓取网络信息」，等扫描 + 封面下载完成。
4. 在列表里勾选要入库的游戏，点「导入」。
5. 双击封面（或点「启动游戏」）开始玩，退出后时长会自动记录。

> 也可以直接把文件夹拖进窗口。数据默认存在 `%APPDATA%\GalgameManager`，
> 想做成绿色版就在 exe 旁边放一个空的 `portable.txt`。

---

## 开发

需要：Go 1.26+、Node 18+、WebView2 Runtime（Win10/11 一般已自带）。

```bash
# 后端（浏览器调试模式，端口 8765）
go run . --serve --port 8765

# 前端热更新（另开一个终端，Vite 会代理 /api 到 8765）
cd frontend && npm install && npm run dev
```

打开 http://localhost:5173 即可。前后端通过同源 `/api` 通信，
**桌面模式与浏览器模式跑的是同一套 handler**，所以浏览器里调好的界面就是桌面里的界面。

```bash
# 原生窗口开发模式
wails dev
```

## 构建

```bash
wails build -clean -o GalgameManager.exe
```

产物：`build/bin/GalgameManager.exe`（单文件，约 17 MB，无需安装 Go / Node）。

> 双击 `build.bat` 也能走完整流程：检查环境 → 装 wails → `npm install` → `wails build`。
> 没装 wails CLI 的话先执行：`go install github.com/wailsapp/wails/v2/cmd/wails@latest`

## 界面

液态玻璃（Liquid Glass）风格：整张二次元壁纸铺底，侧边栏和主面板都是半透明磨砂玻璃，
四边有棱镜高光和顶部掠射反光，鼠标划过卡片会有一道液态扫光；按钮、卡片、开关按下时
带果冻回弹（`cubic-bezier(.34,1.56,.64,1)`）。

**换背景图**：替换 `frontend/src/assets/bg.jpg`（建议 1920×1080 或更宽），
然后 `wails build` 重新打包即可。程序会对它做压暗和偏冷色调处理，
让白字在玻璃上依然清晰 —— 这段处理在 CSS 的 `body::before / body::after` 里，
想调整浓度改那两个 `rgba()` 即可。

## 数据位置

| 内容     | 路径                                                     |
| ------ | ------------------------------------------------------ |
| 数据库    | `%APPDATA%\GalgameManager\data\library.db`             |
| 设置     | `%APPDATA%\GalgameManager\data\settings.json`          |
| 封面缓存   | `%APPDATA%\GalgameManager\covers\`                     |

**便携模式**：在 exe 同目录放一个空的 `portable.txt`，数据会改存到 exe 旁边的 `data\` 目录。
也可以用环境变量 `GALGAME_DATA_DIR=<路径>` 指定。

---

## 目录结构

```
galgame-manager/
├── main.go                    # 入口：Wails 窗口 / --serve 调试模式
├── app.go                     # 应用核心：扫描调度、导入、设置、封面缓存
├── api.go                     # HTTP API 路由
├── internal/
│   ├── model/model.go         # 数据结构
│   ├── store/store.go         # SQLite 持久化（games / categories / 关联表）
│   ├── scan/
│   │   ├── scan.go            # 目录发现、启动文件打分、封面识别、图片尺寸
│   │   └── wininfo_windows.go # exe 版本信息读取、exe 图标提取（纯 syscall）
│   ├── meta/meta.go           # Bangumi / VNDB、封面下载、数据源熔断
│   └── launch/launch.go       # 进程启动、进程树监控、翻译工具
├── frontend/
│   ├── src/App.vue            # 主界面：侧边栏 / 顶栏 / 网格 / 列表 / 右键菜单
│   ├── src/api.js             # API 封装与格式化工具
│   └── src/components/        # GameCard / Sidebar / DetailDrawer / ImportModal / SettingsModal
├── build/                     # 图标与打包中间产物
└── wails.json                 # Wails 构建配置
```

## 说明与已知限制

- **Bangumi 接口是 `api.bgm.tv`**。如果本机 DNS 被污染（比如解析到无关 IP），
  程序会自动熔断并回退到 VNDB，此时标签会是英文的 —— 这是数据源问题，不是程序问题。
- **封面 CDN（t.vndb.org）在国内可能很慢**。所以封面下载是独立阶段：
  元数据先出结果，封面在后台继续下，慢也不会卡住扫描；导入时还会再补一次。
- 只识别文件夹形式安装的 galgame，不处理压缩包（`.zip` / `.rar` / `.7z`），需要先解压。
- 启动器监控的是启动的进程及其子进程；如果游戏靠第三方启动器（Steam 等）中转，
  请在详情里把启动文件指到实际 exe。
- 时长统计要求游戏进程在运行期间可被查询；异常退出（如蓝屏）会丢失当次时长。
