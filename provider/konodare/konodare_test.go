package konodare

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/metatube-community/metatube-sdk-go/common/parser"
	"github.com/metatube-community/metatube-sdk-go/provider"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func offline(t *testing.T, f func(*http.Request) (int, string)) *Konodare {
	t.Helper()
	p := New()
	p.ClonedCollector().WithTransport(transportFunc(func(r *http.Request) (*http.Response, error) {
		code, body := f(r)
		return &http.Response{StatusCode: code, Header: http.Header{"Content-Type": {"text/html; charset=utf-8"}}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	}))
	return p
}

const (
	oldHTML       = `<article><h1>PNME-333</h1><div class="entry-content"><h9>作品タイトル</h9><br><div>紹介文</div><table class="product"><tr><th>PNME-333</th></tr><tr><td rowspan="5" class="product_image"><img src="data:image/gif;base64,AA"><img src="https://img.example/cover.jpg"></td><td class="product_item">配信日</td><td>2026/10/04</td></tr><tr><td class="product_item">時間</td><td>46分</td></tr><tr><td class="product_item">サークル</td><td>ぷにもえ</td></tr><tr><td class="product_item">女優</td><td><a>さくらまひるっぽい</a></td></tr></table><table class="cosplay-table"><tr><td class="chara">キャラクター</td><td><a>キャラ</a><a>キャラ</a><em>English</em></td></tr><tr><td class="chara">出典</td><td><a>原作</a></td></tr></table></div></article><article><div class="entry-content"><h9>汚染</h9><table class="product"><tr><td class="product_item">女優</td><td><a>汚染</a></td></tr></table></div></article>`
	newHTML       = `<article class="mdb-work"><h1 class="mdb-work-title">新作品</h1><div class="mdb-work-item-no">COSH-308</div><div class="mdb-work-cover"><img src="//img.example/new.jpg"></div><div class="mdb-summary-row"><span class="mdb-summary-key">メーカー：</span><span class="mdb-summary-value">こすっち</span></div><div class="mdb-summary-row"><span class="mdb-summary-key">シリーズ：</span><span class="mdb-summary-value">---</span></div><div class="mdb-work-content">新紹介</div><div class="mdb-cosplay-meta-row"><span class="mdb-cosplay-label">女優</span><span class="mdb-cosplay-value"><a>さくらまひるっぽい</a></span></div><article class="mdb-source-card"><div class="mdb-source-meta-row"><span>配信日</span><strong>2026年10月17日</strong></div><div class="mdb-source-meta-row"><span>時間</span><strong>42分</strong></div></article><article class="mdb-source-card"><div class="mdb-source-meta-row"><span>時間</span><strong>99分</strong></div></article></article><article class="mdb-work"><h1 class="mdb-work-title">汚染</h1></article>`
	oldSearchHTML = `<main id="main"><div id="list"><a class="entry-card-wrap" href="/pnme-333/"><article><table><tr><th class="card-number">PNME-333</th></tr><tr><td><img src="https://img.example/cover.jpg"></td><td class="product_item"><b class="entry-card-title">作品タイトル</b>配信日:2026/10/04<br>女優：<a href="/actress/a/">さくらまひるっぽい</a></td></tr></table></article></a></div></main>`
	newSearchHTML = `<main class="mdb-main"><div class="mdb-search-grid"><article class="mdb-card"><h2 class="mdb-card-title"><a href="/cosh-308/">新作品</a></h2><div class="mdb-card-id"><span class="mdb-card-label">品番： COSH-308</span></div><div class="mdb-card-date">発売日：2026年10月17日</div></article></div></main>`
)

const (
	oldEmptyHTML = `<body class="search search-no-results"><main id="main"><div class="posts-not-found"><h2>NOT FOUND</h2><p>投稿が見つかりませんでした。</p></div></main></body>`
	newEmptyHTML = `<main class="mdb-main"><div class="mdb-empty"><p>「HHSI-024」に一致する作品が見つかりませんでした。</p></div></main>`
)

func TestRealEmptySearchTemplates(t *testing.T) {
	for _, tc := range []struct {
		name, modern string
		count        int
	}{
		{"new_hit", newSearchHTML, 1}, {"new_empty", newEmptyHTML, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := []string{}
			p := offline(t, func(r *http.Request) (int, string) {
				calls = append(calls, r.URL.Host)
				if r.URL.Host == "konodare.wew.jp" {
					return 200, oldEmptyHTML
				}
				return 200, tc.modern
			})
			rs, err := p.SearchMovie("HHSI-024")
			if err != nil || len(rs) != tc.count || strings.Join(calls, ",") != "konodare.wew.jp,konodare.jp" {
				t.Fatal(rs, err, calls)
			}
		})
	}
}

func TestEmptySearchRequiresTemplateAnchors(t *testing.T) {
	for _, body := range []string{
		`<p>投稿が見つかりませんでした。</p>`,
		strings.Replace(oldEmptyHTML, `class="search search-no-results"`, `class="search"`, 1),
		strings.Replace(oldEmptyHTML, `id="main"`, `id="sidebar"`, 1),
		strings.Replace(oldEmptyHTML, "投稿が見つかりませんでした。", "未知のエラー", 1),
		oldEmptyHTML + `<form id="challenge-form"></form>`,
	} {
		p := offline(t, func(r *http.Request) (int, string) {
			if r.URL.Host != "konodare.wew.jp" {
				t.Fatal("不应回退新站")
			}
			return 200, body
		})
		if _, err := p.SearchMovie("HHSI-024"); err == nil {
			t.Fatal(body)
		}
	}
	for _, body := range []string{
		`<div class="mdb-empty"><p>「HHSI-024」に一致する作品が見つかりませんでした。</p></div>`,
		strings.Replace(newEmptyHTML, "mdb-main", "sidebar", 1),
		strings.Replace(newEmptyHTML, "HHSI-024", "別の検索", 1),
		strings.Replace(newEmptyHTML, "に一致する作品が見つかりませんでした。", "エラー", 1),
	} {
		p := offline(t, func(r *http.Request) (int, string) {
			if r.URL.Host == "konodare.wew.jp" {
				return 200, oldEmptyHTML
			}
			return 200, body
		})
		if _, err := p.SearchMovie("HHSI-024"); err == nil {
			t.Fatal(body)
		}
	}
}

func TestSummaryLineBreaks(t *testing.T) {
	for _, tc := range []struct{ id, body, old string }{
		{"wew:pnme-333", oldHTML, `<div>紹介文</div>`},
		{"jp:cosh-308", newHTML, `<div class="mdb-work-content">新紹介</div>`},
	} {
		replacement := `<div><p>一行目<br>二行目</p><p>三行目 <b>強調</b></p></div>`
		if strings.HasPrefix(tc.id, "jp:") {
			replacement = strings.Replace(replacement, "<div>", `<div class="mdb-work-content">`, 1)
		}
		p := offline(t, func(*http.Request) (int, string) { return 200, strings.Replace(tc.body, tc.old, replacement, 1) })
		m, err := p.GetMovieInfoByID(tc.id)
		if err != nil || m.Summary != "一行目\n二行目\n三行目 強調" {
			t.Fatal(m, err)
		}
	}
}

func TestPreviewVideoSource(t *testing.T) {
	for _, tc := range []struct{ sources, want string }{
		{`<source src="https://video.example/sample.mp4?a=1&amp;b=2">`, "https://video.example/sample.mp4?a=1&b=2"},
		{`<source src="data:video/mp4;base64,AA"><source src="/sample.mp4">`, "https://konodare.jp/sample.mp4"},
		{`<source src="javascript:alert(1)"><source src="ftp://video.example/sample.mp4">`, ""},
		{`<source src="">`, ""},
	} {
		body := strings.Replace(newHTML, `<h1 class="mdb-work-title">`, `<video>`+tc.sources+`</video><h1 class="mdb-work-title">`, 1) + `<article><video><source src="https://video.example/pollution.mp4"></video></article>`
		p := offline(t, func(*http.Request) (int, string) { return 200, body })
		m, err := p.GetMovieInfoByID("jp:cosh-308")
		if err != nil || m.PreviewVideoURL != tc.want || m.CoverURL != "https://img.example/new.jpg" {
			t.Fatal(m, err)
		}
	}
	p := offline(t, func(*http.Request) (int, string) {
		return 200, strings.Replace(newHTML, `<img src="//img.example/new.jpg">`, `<video><source src="https://video.example/sample.mp4"></video>`, 1)
	})
	if _, err := p.GetMovieInfoByID("jp:cosh-308"); !errors.Is(err, provider.ErrIncompleteMetadata) {
		t.Fatal(err)
	}
}

func TestKonodareIdentity(t *testing.T) {
	p := New()
	if p.Name() != "Konodare" || p.Priority() != 400 {
		t.Fatal(p)
	}
}

func TestDetails(t *testing.T) {
	for _, tc := range []struct {
		id, body, number, title string
		runtime                 int
	}{
		{"wew:pnme-333", oldHTML, "PNME-333", "作品タイトル", 46},
		{"wew:instc665", strings.ReplaceAll(oldHTML, "PNME-333", "INSTC665"), "INSTC665", "作品タイトル", 46},
		{"jp:cosh-308", newHTML, "COSH-308", "新作品", 42},
	} {
		t.Run(tc.id, func(t *testing.T) {
			calls := 0
			p := offline(t, func(r *http.Request) (int, string) {
				calls++
				if strings.HasPrefix(tc.id, "jp:") && r.URL.Host != "konodare.jp" {
					t.Fatal(r.URL)
				}
				return 200, tc.body
			})
			m, err := p.GetMovieInfoByID(tc.id)
			if err != nil {
				t.Fatal(err)
			}
			if !m.IsValid() || m.ID != tc.id || m.Number != tc.number || m.Title != tc.title || m.Runtime != tc.runtime || calls != 1 {
				t.Fatalf("%+v calls=%d", m, calls)
			}
			if len(m.Actors) != 1 || m.Actors[0] != "さくらまひるっぽい" || m.Series != "" {
				t.Fatal(m)
			}
			if m.Summary == "" || strings.Contains(strings.Join(m.Genres, ","), "English") {
				t.Fatal(m)
			}
			if strings.HasPrefix(tc.id, "wew:") {
				if m.Maker != "ぷにもえ" || m.Label != "ぷにもえ" || len(m.Genres) != 2 || m.Genres[0] != "キャラ" || m.ReleaseDate != parser.ParseDate("2026/10/04") || m.CoverURL != "https://img.example/cover.jpg" {
					t.Fatalf("old fields: %+v", m)
				}
			} else if m.Maker != "こすっち" || m.ReleaseDate != parser.ParseDate("2026/10/17") || m.CoverURL != "https://img.example/new.jpg" {
				t.Fatalf("new fields: %+v", m)
			}
			id, err := p.ParseMovieIDFromURL(m.Homepage)
			if err != nil || id != tc.id {
				t.Fatal(id, err)
			}
		})
	}
}

func TestCocoonSearchTemplate(t *testing.T) {
	const body = `<div id="list"><a href="https://konodare.wew.jp/azm044/" class="entry-card-wrap"><article><img src="https://img.example/azm044.jpg"><h2 class="entry-card-title">AZM044</h2><div class="entry-card-snippet"><b>テスト作品</b></div></article></a></div>`
	p := offline(t, func(r *http.Request) (int, string) {
		if r.URL.Host != "konodare.wew.jp" {
			t.Fatal(r.URL)
		}
		return 200, body
	})
	rs, err := p.SearchMovie("AZM-044")
	if err != nil || len(rs) != 1 {
		t.Fatal(rs, err)
	}
	if rs[0].Number != "AZM044" || rs[0].Title != "テスト作品" || rs[0].ID != "wew:azm044" || rs[0].CoverURL != "https://img.example/azm044.jpg" || !rs[0].IsValid() {
		t.Fatalf("unexpected search result: %+v", rs[0])
	}
}

func TestSearchFallback(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
		fallback   bool
	}{
		{"empty", `<main id="main"><div id="list"></div></main>`, 200, true},
		{"found", oldSearchHTML, 200, false},
		{"forbidden", "denied", 403, false},
		{"captcha", `<div id="list"></div><form id="challenge-form">captcha</form>`, 200, false},
		{"template", "<html>unknown</html>", 200, false},
		{"incomplete", `<div id="list"><a class="entry-card-wrap"><article>broken</article></a></div>`, 200, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			p := offline(t, func(r *http.Request) (int, string) {
				calls++
				if r.URL.Query().Get("s") != "a & 日本" {
					t.Fatal(r.URL)
				}
				if r.URL.Host == "konodare.jp" {
					return 200, newSearchHTML
				}
				return tc.status, tc.body
			})
			rs, err := p.SearchMovie("a & 日本")
			if tc.fallback {
				if err != nil || calls != 2 || len(rs) != 1 || rs[0].ID != "jp:cosh-308" {
					t.Fatal(rs, err, calls)
				}
			} else if calls != 1 {
				t.Fatal(calls)
			}
			if tc.name != "empty" && tc.name != "found" && err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestExactAndDedup(t *testing.T) {
	p := offline(t, func(r *http.Request) (int, string) {
		if r.URL.RawQuery == "" {
			t.Fatal("fuzzy match fetched")
		}
		return 200, strings.Replace(oldSearchHTML, "</div></main>", strings.TrimPrefix(oldSearchHTML, `<main id="main"><div id="list">`), 1)
	})
	rs, err := p.SearchMovie("COSH-308")
	if err != nil || len(rs) != 1 {
		t.Fatal(rs, err)
	}
	_, err = p.GetMovieInfoByID("COSH-308")
	if !errors.Is(err, provider.ErrInfoNotFound) {
		t.Fatal(err)
	}
}

func TestBoundaries(t *testing.T) {
	p := offline(t, func(*http.Request) (int, string) { t.Fatal("invalid input requested"); return 500, "" })
	for _, u := range []string{"https://konodare.jp.evil/a/", "ftp://konodare.jp/a/", "https://konodare.jp/", "https://konodare.jp/?s=a", "https://konodare.jp/cosh-308/?s=a", "https://konodare.jp/category/a/", "https://konodare.jp/search/", "https://u@konodare.jp/cosh-308/", "https://konodare.jp:443/cosh-308/", "https://konodare.jp/%2fcosh-308/", "https://konodare.jp/2026/", "https://konodare.jp/2026/10/", "https://konodare.jp//cosh-308/"} {
		if _, err := p.GetMovieInfoByURL(u); err == nil {
			t.Fatal(u)
		}
	}
	for _, id := range []string{"jp:../a", "other:a", "jp:", "wew:a/b", "jp:search"} {
		if _, err := p.GetMovieInfoByID(id); err == nil {
			t.Fatal(id)
		}
	}
}

func TestNetworkErrorDoesNotFallback(t *testing.T) {
	p := New()
	calls := 0
	p.ClonedCollector().WithTransport(transportFunc(func(*http.Request) (*http.Response, error) { calls++; return nil, errors.New("offline network error") }))
	if _, err := p.SearchMovie("PNME-333"); err == nil || calls != 1 {
		t.Fatal(err, calls)
	}
}

func TestExactNumberRoundtrip(t *testing.T) {
	for _, mismatch := range []bool{false, true} {
		p := offline(t, func(r *http.Request) (int, string) {
			if r.URL.Query().Has("s") {
				return 200, oldSearchHTML
			}
			if r.URL.Path != "/pnme-333/" {
				t.Fatal(r.URL)
			}
			if mismatch {
				return 200, strings.ReplaceAll(oldHTML, "PNME-333", "COSH-263")
			}
			return 200, oldHTML
		})
		m, err := p.GetMovieInfoByID(" pnme-333 ")
		if mismatch {
			if !errors.Is(err, provider.ErrInfoNotFound) {
				t.Fatal(m, err)
			}
		} else if err != nil || m.ID != "wew:pnme-333" {
			t.Fatal(m, err)
		}
	}
}

func TestNumberComparisonPreservesLeadingZeros(t *testing.T) {
	for _, tc := range []struct {
		name, input, searchNumber, detailNumber string
		match                                   bool
		requests                                int
	}{
		{"无横杠番号", "PNME333", "PNME-333", "PNME-333", true, 2},
		{"大小写空白", " pnme 333 ", "PNME-333", "PNME-333", true, 2},
		{"搜索前导零不同", "PNME0333", "PNME-333", "PNME-333", false, 1},
		{"搜索数字不同", "PNME334", "PNME-333", "PNME-333", false, 1},
		{"详情横杠不同", "PNME-333", "PNME-333", "PNME333", true, 2},
		{"详情前导零不同", "PNME333", "PNME-333", "PNME-0333", false, 2},
		{"详情数字不同", "PNME333", "PNME-333", "PNME-334", false, 2},
		{"保留原始无横杠格式", "INSTC-665", "INSTC665", "INSTC665", true, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			p := offline(t, func(r *http.Request) (int, string) {
				calls++
				if r.URL.Query().Has("s") {
					return 200, strings.ReplaceAll(oldSearchHTML, "PNME-333", tc.searchNumber)
				}
				return 200, strings.ReplaceAll(oldHTML, "PNME-333", tc.detailNumber)
			})
			m, err := p.GetMovieInfoByID(tc.input)
			if calls != tc.requests {
				t.Fatalf("请求次数 %d，预期 %d", calls, tc.requests)
			}
			if tc.match {
				if err != nil || m.Number != tc.detailNumber {
					t.Fatal(m, err)
				}
			} else if !errors.Is(err, provider.ErrInfoNotFound) {
				t.Fatal(m, err)
			}
		})
	}
}

func TestCrossHostDedupAndFields(t *testing.T) {
	// 两站的来源 ID 保持独立，同番号重复结果仅保留第一条。
	body := strings.Replace(oldSearchHTML, "</div></main>", strings.Replace(strings.TrimPrefix(oldSearchHTML, `<main id="main"><div id="list">`), `href="/pnme-333/"`, `href="https://konodare.jp/pnme-333/"`, 1), 1)
	p := offline(t, func(*http.Request) (int, string) { return 200, body })
	rs, err := p.SearchMovie("PNME-333")
	if err != nil || len(rs) != 1 || rs[0].ID != "wew:pnme-333" || len(rs[0].Actors) != 1 || rs[0].Actors[0] != "さくらまひるっぽい" {
		t.Fatal(rs, err)
	}
	if rs[0].ReleaseDate != parser.ParseDate("2026/10/04") {
		t.Fatal(rs[0].ReleaseDate)
	}
}

func TestRedirectBoundaries(t *testing.T) {
	for _, target := range []string{"https://konodare.jp/cosh-308/", "https://evil.example/cosh-308/", "https://konodare.jp/"} {
		p := New()
		calls := 0
		p.ClonedCollector().WithTransport(transportFunc(func(r *http.Request) (*http.Response, error) {
			calls++
			code, body := http.StatusOK, newHTML
			header := http.Header{"Content-Type": {"text/html"}}
			if calls == 1 {
				code = http.StatusFound
				body = ""
				header.Set("Location", target)
			} else if r.URL.Host != "konodare.jp" {
				t.Fatal("external redirect followed")
			}
			return &http.Response{StatusCode: code, Header: header, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
		}))
		m, err := p.GetMovieInfoByID("wew:pnme-333")
		if target == "https://konodare.jp/cosh-308/" {
			if err != nil || m.ID != "jp:cosh-308" || calls != 2 {
				t.Fatal(m, err, calls)
			}
		} else if err == nil || calls != 1 {
			t.Fatal(m, err, calls)
		}
	}
}

func TestCanonicalAndErrors(t *testing.T) {
	for _, tc := range []struct {
		name, head string
		status     int
		want       error
	}{
		{"external", `<link rel="canonical" href="https://evil/pnme-333/">`, 200, provider.ErrInvalidURL},
		{"home", `<link rel="canonical" href="/">`, 200, provider.ErrInvalidURL},
		{"404", "", 404, provider.ErrInfoNotFound},
		{"incomplete", "", 200, provider.ErrIncompleteMetadata},
		{"canonical", `<link rel="canonical" href="https://konodare.jp/pnme-333/">`, 200, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := oldHTML
			if tc.name == "incomplete" {
				body = `<article><div class="entry-content"><h9>title</h9></div></article>`
			}
			p := offline(t, func(*http.Request) (int, string) { return tc.status, tc.head + body })
			m, err := p.GetMovieInfoByID("wew:item4063410")
			if !errors.Is(err, tc.want) {
				t.Fatal(err)
			}
			if tc.name == "canonical" && m.ID != "jp:pnme-333" {
				t.Fatal(m)
			}
		})
	}
}
