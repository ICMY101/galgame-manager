package meta

import (
	"encoding/json"
	"strings"
	"testing"
)

// 取自 Bangumi v0 /search/subjects 的真实响应结构（字段裁剪）
const bangumiSample = `{
  "data": [
    {
      "id": 876,
      "name": "CLANNAD",
      "name_cn": "CLANNAD -クラナド-",
      "summary": "这是一个关于家庭与羁绊的故事。<br />主角冈崎朋也……",
      "date": "2004-04-28",
      "images": {
        "large": "https://lain.bgm.tv/pic/cover/l/xx.jpg",
        "common": "https://lain.bgm.tv/pic/cover/c/xx.jpg",
        "medium": "https://lain.bgm.tv/pic/cover/m/xx.jpg"
      },
      "rating": { "score": 8.7, "total": 5200 },
      "tags": [
        { "name": "Key", "count": 900 },
        { "name": "泣きゲー", "count": 300 }
      ],
      "infobox": [
        { "key": "开发", "value": "Key" },
        { "key": "平台", "value": [{"v": "PC"}, {"v": "PS2"}] }
      ]
    }
  ]
}`

func TestBangumiParse(t *testing.T) {
	var resp struct {
		Data []bgmSubject `json:"data"`
	}
	if err := json.Unmarshal([]byte(bangumiSample), &resp); err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if len(resp.Data) != 1 {
		t.Fatalf("期望 1 条，得到 %d", len(resp.Data))
	}

	r := resultFromBangumi(pickBangumi(resp.Data, "CLANNAD"))
	if r.Title != "CLANNAD -クラナド-" {
		t.Errorf("中文标题错误: %q", r.Title)
	}
	if r.OriginalTitle != "CLANNAD" {
		t.Errorf("原始标题错误: %q", r.OriginalTitle)
	}
	if r.Developer != "Key" {
		t.Errorf("开发商错误: %q", r.Developer)
	}
	if r.Rating != 8.7 {
		t.Errorf("评分错误: %v", r.Rating)
	}
	if r.ReleaseDate != "2004-04-28" {
		t.Errorf("发售日错误: %q", r.ReleaseDate)
	}
	if !strings.HasPrefix(r.CoverURL, "https://lain.bgm.tv/pic/cover/l/") {
		t.Errorf("封面应优先取 large: %q", r.CoverURL)
	}
	if strings.Contains(r.Description, "<br") {
		t.Errorf("简介未清洗 HTML: %q", r.Description)
	}
	if len(r.Tags) != 2 {
		t.Errorf("标签数量错误: %v", r.Tags)
	}
	if r.Source != "bangumi" || r.SourceID != "876" {
		t.Errorf("来源标记错误: %s/%s", r.Source, r.SourceID)
	}
}

func TestPickBangumiPrefersExact(t *testing.T) {
	var resp struct {
		Data []bgmSubject `json:"data"`
	}
	_ = json.Unmarshal([]byte(`{"data":[
		{"id":1,"name":"CLANNAD Anthology","rating":{"total":99}},
		{"id":2,"name":"CLANNAD","rating":{"total":10}}
	]}`), &resp)

	got := pickBangumi(resp.Data, "CLANNAD")
	if got.ID != 2 {
		t.Errorf("应优先精确匹配，实际选中 %d", got.ID)
	}
}

func TestStripHTML(t *testing.T) {
	in := "第一行<br />第二行 <b>加粗</b> &amp; 符号"
	out := stripHTML(in)
	if strings.Contains(out, "<") || strings.Contains(out, "&amp;") {
		t.Errorf("HTML 未清洗干净: %q", out)
	}
	if !strings.Contains(out, "\n") {
		t.Errorf("换行未被保留: %q", out)
	}
}

func TestVNDBRatingScale(t *testing.T) {
	// VNDB 评分为 0-100，应换算成 0-10
	if got := vndbRating(87); got != 8.7 {
		t.Errorf("评分换算错误: %v", got)
	}
	if got := vndbRating(0); got != 0 {
		t.Errorf("零分处理错误: %v", got)
	}
}
