package avgood

import (
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/metatube-community/metatube-sdk-go/common/parser"
	"github.com/metatube-community/metatube-sdk-go/provider"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func offline(t *testing.T, f func(*http.Request) (int, string)) *AvGood {
	t.Helper()
	p := New()
	p.ClonedCollector().WithTransport(transportFunc(func(r *http.Request) (*http.Response, error) {
		code, body := f(r)
		return &http.Response{StatusCode: code, Header: http.Header{"Content-Type": {"text/html; charset=utf-8"}}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	}))
	return p
}

const detailHTML = `<h1 class="content-title">DVDES-221 テスト作品</h1>
<a class="bigImage" href="/cover.jpg"><img src="data:image/gif;base64,AA" data-original="/cover.jpg"></a>
<div class="content-info"><p><span class="header">識別子:</span><span>DVDES-221</span></p>
<p><span class="header">発行日:</span>2009-09-05</p><p><span class="header">長さ:</span>117分</p>
<p><span class="header">メーカー:</span><a>メーカー</a></p><p><span class="header">出版社:</span><a>レーベル</a></p>
<p><span class="header">監督:</span>監督</p><p><span class="header">シリーズ:</span>シリーズ</p>
<p><span class="genre"><a>カテゴリー</a></span><span class="genre"><a>カテゴリー</a></span></p>
<div class="star-name"><a>女優</a></div><div class="star-name"><a>女優</a></div>
<p><span class="genre" onmouseover="hoverdiv()"><a>女優</a></span></p></div>
<a class="sample-box" href="//img.example/sample.jpg"></a><a class="sample-box" href="//img.example/sample.jpg"></a>
<a class="sample-box" href="magnet:?xt=invalid"></a><a href="magnet:?xt=invalid">下载</a>
<aside><div class="star-name"><a>汚染</a></div></aside>`

const searchHTML = `<div class="container"><div class="search-info">約 1 件の結果</div><h3 class="result-title"><a href="/c/53247.html"><em>DVDES</em> - <em>221</em> テスト作品</a></h3></div>`

const emptyHTML = `<div class="container"><div class="search-info">約 0 件の結果</div><div class="no-results"><h3>関連結果が見つかりません</h3></div></div>`

func TestDetails(t *testing.T) {
	for _, input := range []string{"https://avgood.com/c/53247.html", "https://avgood.com/e/53247.html", "https://avgood.com/j/53247.html", "https://avgood.com/k/53247.html"} {
		p := offline(t, func(r *http.Request) (int, string) {
			if r.URL.String() != "https://avgood.com/j/53247.html" {
				t.Fatal(r.URL)
			}
			return 200, detailHTML
		})
		m, err := p.GetMovieInfoByURL(input)
		if err != nil {
			t.Fatal(err)
		}
		if !m.IsValid() || m.ID != "53247" || m.Provider != Name || m.Number != "DVDES-221" || m.Homepage != "https://avgood.com/j/53247.html" || m.Runtime != 117 || m.ReleaseDate != parser.ParseDate("2009-09-05") {
			t.Fatalf("%+v", m)
		}
		if m.Maker != "メーカー" || m.Label != "レーベル" || m.Director != "監督" || m.Series != "シリーズ" || strings.Join(m.Actors, ",") != "女優" || strings.Join(m.Genres, ",") != "カテゴリー" {
			t.Fatalf("%+v", m)
		}
		if m.CoverURL != "https://avgood.com/cover.jpg" || m.ThumbURL != m.CoverURL || len(m.PreviewImages) != 1 || m.PreviewImages[0] != "https://img.example/sample.jpg" || m.PreviewVideoURL != "" {
			t.Fatalf("%+v", m)
		}
	}
}

func TestLazyCover(t *testing.T) {
	for _, key := range []string{"data-original", "data-src", "src"} {
		body := strings.Replace(detailHTML, `<a class="bigImage" href="/cover.jpg"><img src="data:image/gif;base64,AA" data-original="/cover.jpg"></a>`, `<a class="bigImage"><img `+key+`="/lazy.jpg"></a>`, 1)
		p := offline(t, func(*http.Request) (int, string) { return 200, body })
		m, err := p.GetMovieInfoByID("53247")
		if err != nil || m.CoverURL != "https://avgood.com/lazy.jpg" {
			t.Fatal(m, err)
		}
	}
}

func TestSearch(t *testing.T) {
	p := offline(t, func(r *http.Request) (int, string) {
		if r.URL.Path != "/j/s/" || r.URL.Query().Get("q") != "a & 日本" {
			t.Fatal(r.URL)
		}
		return 200, searchHTML + searchHTML
	})
	rs, err := p.SearchMovie("a & 日本")
	if err != nil || len(rs) != 1 {
		t.Fatal(rs, err)
	}
	if !rs[0].IsValid() || rs[0].Number != "DVDES-221" || rs[0].Title != "テスト作品" || rs[0].Homepage != "https://avgood.com/j/53247.html" {
		t.Fatal(rs[0])
	}
}

func TestNumberLookup(t *testing.T) {
	for _, tc := range []struct {
		input, searchNumber, detailNumber string
		want                              bool
		calls                             int
	}{
		{"dvdes221", "DVDES-221", "DVDES-221", true, 2},
		{" DVDES_221 ", "DVDES-221", "DVDES221", true, 2},
		{"SD-008", "484SD-008", "484SD-008", false, 1},
		{"DVDES-221", "DVDES-0221", "DVDES-0221", false, 1},
		{"DVDES-221", "DVDES-221", "DVDES-222", false, 2},
		{"DVDES-221", "DVDES-221", "DVDES-221A", false, 2},
	} {
		t.Run(tc.input+tc.searchNumber+tc.detailNumber, func(t *testing.T) {
			calls := 0
			p := offline(t, func(r *http.Request) (int, string) {
				calls++
				if r.URL.Path == "/j/s/" {
					return 200, strings.ReplaceAll(searchHTML, "<em>DVDES</em> - <em>221</em>", tc.searchNumber)
				}
				return 200, strings.ReplaceAll(detailHTML, "DVDES-221", tc.detailNumber)
			})
			m, err := p.GetMovieInfoByID(tc.input)
			if calls != tc.calls {
				t.Fatal(calls, tc.calls)
			}
			if tc.want {
				if err != nil || m.Number != tc.detailNumber {
					t.Fatal(m, err)
				}
			} else if !errors.Is(err, provider.ErrInfoNotFound) {
				t.Fatal(m, err)
			}
		})
	}
}

func TestSearchErrors(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
		wantError  bool
	}{
		{"empty", emptyHTML, 200, false},
		{"unknown", "<html>unknown</html>", 200, true},
		{"unanchored", `<div class="no-results">関連結果が見つかりません</div>`, 200, true},
		{"challenge", `<title>Just a moment...</title>` + emptyHTML, 200, true},
		{"captcha", `<form id="challenge-form"></form>` + searchHTML, 200, true},
		{"forbidden", emptyHTML, 403, true},
		{"broken", strings.ReplaceAll(searchHTML, "/c/53247.html", "https://evil.example/c/53247.html"), 200, true},
		{"missing_number", strings.ReplaceAll(searchHTML, "<em>DVDES</em> - <em>221</em>", "作品"), 200, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := offline(t, func(*http.Request) (int, string) { return tc.status, tc.body })
			rs, err := p.SearchMovie("DVDES-221")
			if (err != nil) != tc.wantError {
				t.Fatal(rs, err)
			}
			if !tc.wantError && len(rs) != 0 {
				t.Fatal(rs)
			}
		})
	}
}

func TestDetailErrors(t *testing.T) {
	for _, tc := range []struct {
		body   string
		status int
		want   error
	}{
		{"", 404, provider.ErrInfoNotFound},
		{"", 200, provider.ErrIncompleteMetadata},
		{strings.ReplaceAll(detailHTML, "DVDES-221", ""), 200, provider.ErrIncompleteMetadata},
		{strings.ReplaceAll(detailHTML, "/cover.jpg", "data:image/gif;base64,AA"), 200, provider.ErrIncompleteMetadata},
	} {
		p := offline(t, func(*http.Request) (int, string) { return tc.status, tc.body })
		if _, err := p.GetMovieInfoByID("53247"); !errors.Is(err, tc.want) {
			t.Fatal(err)
		}
	}
}

func TestBoundaries(t *testing.T) {
	p := offline(t, func(*http.Request) (int, string) { t.Fatal("不应发起请求"); return 500, "" })
	for _, raw := range []string{"https://avgood.com.evil/j/53247.html", "https://user@avgood.com/j/53247.html", "https://avgood.com:443/j/53247.html", "ftp://avgood.com/j/53247.html", "https://avgood.com/j/53247.html?x=1", "https://avgood.com/j/53247.html#x", "https://avgood.com/j/%35%33%32%34%37.html", "https://avgood.com/j/53247.html?", "https://avgood.com/j/s/?q=abc", "https://avgood.com/j/t/53247/", "https://avgood.com/j/0.html"} {
		if _, err := p.GetMovieInfoByURL(raw); !errors.Is(err, provider.ErrInvalidURL) {
			t.Fatal(raw, err)
		}
	}
	for _, id := range []string{"", "../53247", "https://evil.example", "UNKNOWN", "0"} {
		if _, err := p.GetMovieInfoByID(id); !errors.Is(err, provider.ErrInvalidID) {
			t.Fatal(id, err)
		}
	}
	if _, err := p.SearchMovie("  "); !errors.Is(err, provider.ErrInvalidKeyword) {
		t.Fatal(err)
	}
	if p.Name() != Name || p.Priority() != 300 {
		t.Fatal(p)
	}
}

func TestRedirects(t *testing.T) {
	for _, target := range []string{"https://avgood.com/j/53247.html", "https://avgood.com/c/53247.html", "https://avgood.com/e/53247.html", "https://avgood.com/k/53247.html", "https://evil.example/j/53247.html", "https://avgood.com/j/999.html", "https://avgood.com/"} {
		p := New()
		calls := 0
		p.ClonedCollector().WithTransport(transportFunc(func(r *http.Request) (*http.Response, error) {
			calls++
			header := http.Header{"Content-Type": {"text/html"}}
			status, body := 200, detailHTML
			if calls == 1 {
				status = 302
				header.Set("Location", target)
				body = ""
			}
			return &http.Response{StatusCode: status, Header: header, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
		}))
		m, err := p.GetMovieInfoByID("53247")
		if target == "https://avgood.com/j/53247.html" {
			if err != nil || m.ID != "53247" {
				t.Fatal(m, err)
			}
		} else if err == nil {
			t.Fatal(m)
		}
		if (strings.Contains(target, "evil.example") || !strings.Contains(target, "/j/")) && calls != 1 {
			t.Fatal(calls)
		}
	}
}

func TestNetworkError(t *testing.T) {
	p := New()
	calls := 0
	p.ClonedCollector().WithTransport(transportFunc(func(*http.Request) (*http.Response, error) { calls++; return nil, errors.New("offline") }))
	if _, err := p.SearchMovie("DVDES-221"); err == nil || calls != 1 {
		t.Fatal(err, calls)
	}
}

func TestLive(t *testing.T) {
	proxy := os.Getenv("AVGOOD_TEST_PROXY")
	if proxy == "" {
		t.Skip("设置 AVGOOD_TEST_PROXY 后运行联网验证")
	}
	p := New()
	if err := p.SetProxy(proxy); err != nil {
		t.Fatal(err)
	}
	for _, number := range []string{"DVDES-163", "DVDES-221"} {
		m, err := p.GetMovieInfoByID(number)
		if err != nil || !sameNumber(m.Number, number) {
			t.Fatal(number, m, err)
		}
		t.Logf("%s: ID=%s, Runtime=%d, Actors=%d, Images=%d", number, m.ID, m.Runtime, len(m.Actors), len(m.PreviewImages))
	}
}
