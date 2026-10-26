// Package store 提供基于 SQLite 的持久化层。
package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"galgame-manager/internal/model"
)

const schema = `
CREATE TABLE IF NOT EXISTS games (
	id               INTEGER PRIMARY KEY AUTOINCREMENT,
	folder           TEXT UNIQUE NOT NULL,
	title            TEXT DEFAULT '',
	original_title   TEXT DEFAULT '',
	exe_path         TEXT DEFAULT '',
	cover_path       TEXT DEFAULT '',
	cover_url        TEXT DEFAULT '',
	developer        TEXT DEFAULT '',
	release_date     TEXT DEFAULT '',
	description      TEXT DEFAULT '',
	rating           REAL DEFAULT 0,
	community_rating REAL DEFAULT 0,
	tags             TEXT DEFAULT '',
	status           TEXT DEFAULT 'unplayed',
	favorite         INTEGER DEFAULT 0,
	last_played      TEXT DEFAULT '',
	play_count       INTEGER DEFAULT 0,
	play_time        INTEGER DEFAULT 0,
	date_added       TEXT DEFAULT '',
	launch_args      TEXT DEFAULT '',
	window_mode      TEXT DEFAULT 'default',
	save_paths       TEXT DEFAULT '',
	size_bytes       INTEGER DEFAULT 0,
	source           TEXT DEFAULT '',
	source_id        TEXT DEFAULT '',
	cover_version    INTEGER DEFAULT 0,
	notes            TEXT DEFAULT ''
);

CREATE TABLE IF NOT EXISTS categories (
	id   INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT UNIQUE NOT NULL,
	sort INTEGER DEFAULT 0
);

CREATE TABLE IF NOT EXISTS game_categories (
	game_id     INTEGER NOT NULL,
	category_id INTEGER NOT NULL,
	PRIMARY KEY (game_id, category_id)
);

CREATE INDEX IF NOT EXISTS idx_games_folder ON games(folder);
CREATE INDEX IF NOT EXISTS idx_gc_cat ON game_categories(category_id);
`

// Store 封装数据库连接。
type Store struct {
	db *sql.DB
}

// Open 打开（或创建）数据库。
func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	dsn := path + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(schema); err != nil {
		return nil, fmt.Errorf("初始化数据库失败: %w", err)
	}
	return &Store{db: db}, nil
}

// Close 关闭数据库。
func (s *Store) Close() error { return s.db.Close() }

// ---------------------------------------------------------------- 写入

// Upsert 按 folder 唯一键插入或更新，返回游戏 ID。
func (s *Store) Upsert(g *model.Game) (int64, error) {
	if g.DateAdded == "" {
		g.DateAdded = time.Now().Format("2006-01-02 15:04")
	}
	if g.Status == "" {
		g.Status = model.StatusUnplayed
	}
	res, err := s.db.Exec(`
INSERT INTO games(folder,title,original_title,exe_path,cover_path,cover_url,developer,
	release_date,description,rating,community_rating,tags,status,favorite,last_played,
	play_count,play_time,date_added,launch_args,window_mode,save_paths,size_bytes,
	source,source_id,cover_version,notes)
VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT(folder) DO UPDATE SET
	title=excluded.title, original_title=excluded.original_title,
	exe_path=excluded.exe_path, cover_path=excluded.cover_path,
	cover_url=excluded.cover_url, developer=excluded.developer,
	release_date=excluded.release_date, description=excluded.description,
	rating=CASE WHEN excluded.rating>0 THEN excluded.rating ELSE games.rating END,
	community_rating=excluded.community_rating,
	tags=excluded.tags, status=excluded.status, size_bytes=excluded.size_bytes,
	source=excluded.source, source_id=excluded.source_id,
	cover_version=excluded.cover_version, notes=excluded.notes`,
		g.Folder, g.Title, g.OriginalTitle, g.ExePath, g.CoverPath, g.CoverURL,
		g.Developer, g.ReleaseDate, g.Description, g.Rating, g.CommunityRating,
		g.Tags, g.Status, boolInt(g.Favorite), g.LastPlayed, g.PlayCount, g.PlayTime,
		g.DateAdded, g.LaunchArgs, g.WindowMode, g.SavePaths, g.SizeBytes,
		g.Source, g.SourceID, g.CoverVersion, g.Notes)
	if err != nil {
		return 0, err
	}
	if id, _ := res.LastInsertId(); id > 0 {
		return id, nil
	}
	return s.idByFolder(g.Folder)
}

// Update 按 ID 更新全部可编辑字段。
func (s *Store) Update(g *model.Game) error {
	_, err := s.db.Exec(`
UPDATE games SET title=?, original_title=?, exe_path=?, cover_path=?, cover_url=?,
	developer=?, release_date=?, description=?, rating=?, community_rating=?, tags=?,
	status=?, favorite=?, launch_args=?, window_mode=?, save_paths=?, notes=?,
	cover_version=?
WHERE id=?`,
		g.Title, g.OriginalTitle, g.ExePath, g.CoverPath, g.CoverURL,
		g.Developer, g.ReleaseDate, g.Description, g.Rating, g.CommunityRating,
		g.Tags, g.Status, boolInt(g.Favorite), g.LaunchArgs, g.WindowMode,
		g.SavePaths, g.Notes, g.CoverVersion, g.ID)
	return err
}

// Delete 删除一条记录（不删除磁盘文件）。
func (s *Store) Delete(id int64) error {
	if _, err := s.db.Exec(`DELETE FROM games WHERE id=?`, id); err != nil {
		return err
	}
	_, err := s.db.Exec(`DELETE FROM game_categories WHERE game_id=?`, id)
	return err
}

// SetFavorite 设置收藏状态。
func (s *Store) SetFavorite(id int64, fav bool) error {
	_, err := s.db.Exec(`UPDATE games SET favorite=? WHERE id=?`, boolInt(fav), id)
	return err
}

// SetStatus 设置游玩状态。
func (s *Store) SetStatus(id int64, status string) error {
	_, err := s.db.Exec(`UPDATE games SET status=? WHERE id=?`, status, id)
	return err
}

// RecordLaunch 记录一次启动。
func (s *Store) RecordLaunch(id int64) error {
	_, err := s.db.Exec(
		`UPDATE games SET play_count=play_count+1, last_played=? WHERE id=?`,
		time.Now().Format("2006-01-02 15:04"), id)
	return err
}

// AddPlayTime 累加游玩时长（秒）。
func (s *Store) AddPlayTime(id int64, seconds int64) error {
	if seconds <= 0 {
		return nil
	}
	_, err := s.db.Exec(`UPDATE games SET play_time=play_time+? WHERE id=?`, seconds, id)
	return err
}

// ---------------------------------------------------------------- 读取

const cols = `id,folder,title,original_title,exe_path,cover_path,cover_url,developer,
release_date,description,rating,community_rating,tags,status,favorite,last_played,
play_count,play_time,date_added,launch_args,window_mode,save_paths,size_bytes,
source,source_id,cover_version,notes`

func scanGame(sc interface{ Scan(...any) error }) (*model.Game, error) {
	var g model.Game
	var fav int
	err := sc.Scan(&g.ID, &g.Folder, &g.Title, &g.OriginalTitle, &g.ExePath,
		&g.CoverPath, &g.CoverURL, &g.Developer, &g.ReleaseDate, &g.Description,
		&g.Rating, &g.CommunityRating, &g.Tags, &g.Status, &fav, &g.LastPlayed,
		&g.PlayCount, &g.PlayTime, &g.DateAdded, &g.LaunchArgs, &g.WindowMode,
		&g.SavePaths, &g.SizeBytes, &g.Source, &g.SourceID, &g.CoverVersion, &g.Notes)
	if err != nil {
		return nil, err
	}
	g.Favorite = fav != 0
	return &g, nil
}

// Get 按 ID 取一条记录。
func (s *Store) Get(id int64) (*model.Game, error) {
	row := s.db.QueryRow(`SELECT `+cols+` FROM games WHERE id=?`, id)
	g, err := scanGame(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return g, err
}

// GetByFolder 按目录取一条记录。
func (s *Store) GetByFolder(folder string) (*model.Game, error) {
	row := s.db.QueryRow(`SELECT `+cols+` FROM games WHERE folder=?`, folder)
	g, err := scanGame(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return g, err
}

// AllFolders 返回库中所有目录，用于扫描时标记「已存在」。
func (s *Store) AllFolders() (map[string]bool, error) {
	rows, err := s.db.Query(`SELECT folder FROM games`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var f string
		if err := rows.Scan(&f); err != nil {
			return nil, err
		}
		out[strings.ToLower(f)] = true
	}
	return out, rows.Err()
}

// Query 按条件查询游戏列表。
func (s *Store) Query(q model.Query) ([]*model.Game, error) {
	var sb strings.Builder
	sb.WriteString(`SELECT ` + cols + ` FROM games WHERE 1=1`)
	var args []any

	if kw := strings.TrimSpace(q.Keyword); kw != "" {
		sb.WriteString(` AND (title LIKE ? OR original_title LIKE ? OR developer LIKE ?
			OR description LIKE ? OR tags LIKE ? OR notes LIKE ?)`)
		like := "%" + kw + "%"
		args = append(args, like, like, like, like, like, like)
	}
	if q.Status != "" {
		sb.WriteString(` AND status=?`)
		args = append(args, q.Status)
	}
	if q.Tag != "" {
		sb.WriteString(` AND (','||tags||',') LIKE ?`)
		args = append(args, "%,"+q.Tag+",%")
	}
	if q.Category != "" {
		sb.WriteString(` AND id IN (SELECT gc.game_id FROM game_categories gc
			JOIN categories c ON c.id=gc.category_id WHERE c.name=?)`)
		args = append(args, q.Category)
	}
	if q.Favorite {
		sb.WriteString(` AND favorite=1`)
	}

	order := map[string]string{
		"title":      `title COLLATE NOCASE ASC`,
		"added":      `date_added DESC, id DESC`,
		"rating":     `rating DESC, community_rating DESC, title COLLATE NOCASE ASC`,
		"developer":  `developer COLLATE NOCASE ASC, title COLLATE NOCASE ASC`,
		"lastplayed": `last_played DESC, title COLLATE NOCASE ASC`,
		"playtime":   `play_time DESC, title COLLATE NOCASE ASC`,
	}
	o, ok := order[q.Sort]
	if !ok {
		o = order["title"]
	}
	sb.WriteString(` ORDER BY ` + o)

	rows, err := s.db.Query(sb.String(), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []*model.Game{}
	for rows.Next() {
		g, err := scanGame(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------- 分类

// ListCategories 返回所有分类及包含的游戏数。
func (s *Store) ListCategories() ([]model.CategoryCount, error) {
	rows, err := s.db.Query(`
SELECT c.name, COUNT(gc.game_id) FROM categories c
LEFT JOIN game_categories gc ON gc.category_id=c.id
GROUP BY c.id ORDER BY c.sort ASC, c.name COLLATE NOCASE ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.CategoryCount{}
	for rows.Next() {
		var c model.CategoryCount
		if err := rows.Scan(&c.Name, &c.Count); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// AddCategory 新建分类（已存在则忽略）。
func (s *Store) AddCategory(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("分类名不能为空")
	}
	_, err := s.db.Exec(`INSERT OR IGNORE INTO categories(name) VALUES(?)`, name)
	return err
}

// RenameCategory 重命名分类。
func (s *Store) RenameCategory(oldName, newName string) error {
	newName = strings.TrimSpace(newName)
	if newName == "" {
		return fmt.Errorf("分类名不能为空")
	}
	_, err := s.db.Exec(`UPDATE OR IGNORE categories SET name=? WHERE name=?`, newName, oldName)
	return err
}

// DeleteCategory 删除分类并解除关联。
func (s *Store) DeleteCategory(name string) error {
	var id int64
	if err := s.db.QueryRow(`SELECT id FROM categories WHERE name=?`, name).Scan(&id); err != nil {
		return err
	}
	if _, err := s.db.Exec(`DELETE FROM game_categories WHERE category_id=?`, id); err != nil {
		return err
	}
	_, err := s.db.Exec(`DELETE FROM categories WHERE id=?`, id)
	return err
}

// SetGameCategories 覆盖设置某游戏的分类。
func (s *Store) SetGameCategories(gameID int64, names []string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM game_categories WHERE game_id=?`, gameID); err != nil {
		return err
	}
	for _, n := range names {
		n = strings.TrimSpace(n)
		if n == "" {
			continue
		}
		if _, err := tx.Exec(`INSERT OR IGNORE INTO categories(name) VALUES(?)`, n); err != nil {
			return err
		}
		var cid int64
		if err := tx.QueryRow(`SELECT id FROM categories WHERE name=?`, n).Scan(&cid); err != nil {
			return err
		}
		if _, err := tx.Exec(
			`INSERT OR IGNORE INTO game_categories(game_id,category_id) VALUES(?,?)`,
			gameID, cid); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// GameCategories 返回某游戏的分类列表。
func (s *Store) GameCategories(gameID int64) ([]string, error) {
	rows, err := s.db.Query(`SELECT c.name FROM categories c
		JOIN game_categories gc ON gc.category_id=c.id
		WHERE gc.game_id=? ORDER BY c.name COLLATE NOCASE ASC`, gameID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// AddGameToCategory 把游戏加入分类。
func (s *Store) AddGameToCategory(gameID int64, name string) error {
	cats, err := s.GameCategories(gameID)
	if err != nil {
		return err
	}
	for _, c := range cats {
		if c == name {
			return nil
		}
	}
	return s.SetGameCategories(gameID, append(cats, name))
}

// ---------------------------------------------------------------- 聚合

// Facets 返回侧边栏聚合数据。
func (s *Store) Facets() (*model.Facets, error) {
	f := &model.Facets{
		Categories: []model.CategoryCount{},
		Tags:       []model.TagCount{},
		Statuses:   []model.CategoryCount{},
	}

	cats, err := s.ListCategories()
	if err != nil {
		return nil, err
	}
	f.Categories = cats

	rows, err := s.db.Query(`SELECT tags FROM games WHERE tags<>''`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	tagCount := map[string]int{}
	for rows.Next() {
		var tags string
		if err := rows.Scan(&tags); err != nil {
			return nil, err
		}
		for _, t := range strings.Split(tags, ",") {
			t = strings.TrimSpace(t)
			if t != "" {
				tagCount[t]++
			}
		}
	}
	for name, n := range tagCount {
		f.Tags = append(f.Tags, model.TagCount{Name: name, Count: n})
	}
	sortTagCounts(f.Tags)

	if err := s.db.QueryRow(`SELECT COUNT(*) FROM games`).Scan(&f.Total); err != nil {
		return nil, err
	}
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM games WHERE favorite=1`).Scan(&f.Favorites); err != nil {
		return nil, err
	}

	statusRows, err := s.db.Query(`SELECT status, COUNT(*) FROM games GROUP BY status`)
	if err != nil {
		return nil, err
	}
	defer statusRows.Close()
	for statusRows.Next() {
		var c model.CategoryCount
		if err := statusRows.Scan(&c.Name, &c.Count); err != nil {
			return nil, err
		}
		f.Statuses = append(f.Statuses, c)
	}
	return f, nil
}

func sortTagCounts(tags []model.TagCount) {
	// 按出现次数降序，次数相同按名称升序，保证侧边栏顺序稳定
	sort.Slice(tags, func(i, j int) bool {
		if tags[i].Count != tags[j].Count {
			return tags[i].Count > tags[j].Count
		}
		return strings.ToLower(tags[i].Name) < strings.ToLower(tags[j].Name)
	})
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func (s *Store) idByFolder(folder string) (int64, error) {
	var id int64
	err := s.db.QueryRow(`SELECT id FROM games WHERE folder=?`, folder).Scan(&id)
	return id, err
}
