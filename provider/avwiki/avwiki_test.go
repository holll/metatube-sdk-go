package avwiki

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/metatube-community/metatube-sdk-go/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAVWikiIdentity(t *testing.T) {
	p := New()
	require.Equal(t, "AVWiki", p.Name())
	require.Equal(t, float64(350), p.Priority())
}

func TestCleanTitleRecognitionQuestions(t *testing.T) {
	for _, title := range []string{"誰にも言えない", "誰も知らない秘密", "名前はまだない"} {
		assert.Equal(t, title, cleanTitle(title, "", ""))
	}
	assert.Equal(t, "誰にも言えない", cleanTitle("誰にも言えない（えちえち素人 - HHSI-024）", "えちえち素人", "HHSI-024"))
	for _, title := range []string{"誰？", "誰?", " あかりに出てるAV女優は誰？ 名前は？ ", "AV女優は誰?", "名前は？", "名前は?", "誰？名前は？"} {
		assert.Empty(t, cleanTitle(title, "", ""), title)
	}
}

// 使用最小人工样本，不复制整篇文章；所有请求均经 mock transport 处理。
const detail = `<link rel="canonical" href="https://av-wiki.net/hhsi-024/"><article class="article"><header><h1>誰？</h1><div class="article-thumbnail"><img src="data:image/gif;base64,AA" data-src="https://images.example/current.jpg?w=300&amp;h=300" alt="あかり"></div></header><section class="article-body"><div class="qaContainer"><a>別の女優</a></div><dl class="dltable"><dt>作品タイトル</dt><dd>あかり（えちえち素人 - HHSI-024）</dd><dt>レーベル</dt><dd>えちえち素人</dd><dt>AV女優名</dt><dd><a>皆瀬あかり</a></dd><dt>メーカー品番</dt><dd>HHSI-024</dd><dt>FANZA品番</dt><dd>hhsi024</dd><dt>配信開始日</dt><dd>2022-09-25</dd></dl><div class="col_4"><a>推薦女優</a><img src="https://images.example/related.jpg"></div><dl class="dltable"><dt>メーカー品番</dt><dd>OTHER-001</dd><dt>AV女優名</dt><dd>汚染女優</dd></dl></section></article>`

func card(slug string) string {
	return `<article class="archive-list"><div class="eye-catch"><img src="data:image/gif;base64,AA" data-src="https://images.example/thumb.jpg?w=125&amp;h=125" alt="あかり"></div><h2 class="archive-header-title"><a href="https://al.fanza.co.jp/">あかり</a></h2><ul class="post-meta"><li>HHSI-024</li></ul><div class="read-more"><a href="https://av-wiki.net/` + slug + `/">続きを読む</a></div><a href="https://av-wiki.net/av-actress/other/">女優</a></article>`
}

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func offline(t *testing.T, f func(*http.Request) (int, string, error)) *AVWiki {
	t.Helper()
	p := New()
	p.ClonedCollector().WithTransport(transportFunc(func(r *http.Request) (*http.Response, error) {
		require.Equal(t, "av-wiki.net", r.URL.Host)
		status, body, err := f(r)
		if err != nil {
			return nil, err
		}
		return &http.Response{StatusCode: status, Status: fmt.Sprint(status), Header: http.Header{"Content-Type": []string{"text/html; charset=utf-8"}}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	}))
	return p
}
func fixed(t *testing.T, status int, body string) *AVWiki {
	return offline(t, func(*http.Request) (int, string, error) { return status, body, nil })
}
func TestRegistration(t *testing.T) {
	found := false
	provider.RangeMovieFactory(func(name string, f provider.MovieFactory) bool {
		if name == Name {
			found = true
			assert.Equal(t, float64(350), f().Priority())
			_, ok := f().(provider.MovieSearcher)
			assert.True(t, ok)
		}
		return true
	})
	require.True(t, found)
	provider.RangeActorFactory(func(name string, _ provider.ActorFactory) bool { assert.NotEqual(t, Name, name); return true })
}
func TestURLAndIDValidation(t *testing.T) {
	p := offline(t, func(*http.Request) (int, string, error) { t.Fatal("invalid input must not request"); return 0, "", nil })
	for _, raw := range []string{"https://av-wiki.net/", "https://av-wiki.net/?s=hhsi024", "https://av-wiki.net/hhsi-024/?s=x", "https://evilav-wiki.net/hhsi-024/", "https://av-wiki.net.evil/hhsi-024/", "https://av-wiki.net@evil/hhsi-024/", "https://user@av-wiki.net/hhsi-024/", "ftp://av-wiki.net/hhsi-024/", "https://av-wiki.net:443/hhsi-024/", "https://av-wiki.net/av-actress/name/", "https://av-wiki.net/category/", "https://av-wiki.net/fanza-shirouto/", "https://av-wiki.net/2022/", "https://av-wiki.net/a%2Fb-024/", "https://av-wiki.net/../hhsi-024/", "https://av-wiki.net/hhsi-024/#x"} {
		_, err := p.GetMovieInfoByURL(raw)
		require.ErrorIs(t, err, provider.ErrInvalidURL, raw)
	}
	for _, id := range []string{"../hhsi-024", "x/y", "x?s=y", "https://av-wiki.net/hhsi-024/", "x%2fy", ""} {
		assert.Empty(t, p.NormalizeMovieID(id))
		_, err := p.GetMovieInfoByID(id)
		require.ErrorIs(t, err, provider.ErrInvalidID)
	}
	for _, id := range []string{"HHSI-024", "hhsi-024c", "abc-00001-2", "300abc-001"} {
		assert.Equal(t, strings.ToLower(id), p.NormalizeMovieID(id))
		got, err := p.ParseMovieIDFromURL("https://av-wiki.net/" + id + "/")
		require.NoError(t, err)
		assert.Equal(t, strings.ToLower(id), got)
	}
}
func TestDetail(t *testing.T) {
	info, err := fixed(t, 200, detail).GetMovieInfoByID("HHSI-024")
	require.NoError(t, err)
	assert.Equal(t, "hhsi-024", info.ID)
	assert.Equal(t, "HHSI-024", info.Number)
	assert.Equal(t, "AVWiki", info.Provider)
	assert.Equal(t, "https://av-wiki.net/hhsi-024/", info.Homepage)
	assert.Equal(t, "あかり", info.Title)
	assert.Equal(t, "えちえち素人", info.Label)
	assert.Equal(t, []string{"皆瀬あかり"}, []string(info.Actors))
	assert.Equal(t, "2022-09-25", time.Time(info.ReleaseDate).Format("2006-01-02"))
	assert.Equal(t, "https://images.example/current.jpg?w=300&h=300", info.CoverURL)
	assert.Zero(t, info.Runtime)
	assert.Empty(t, info.Summary)
	assert.Empty(t, info.PreviewImages)
}
func TestActorsAndTitleFallback(t *testing.T) {
	for _, tc := range []struct {
		actors string
		want   []string
	}{
		{`<a>皆瀬あかり</a><a>皆瀬あかり</a><a>別名</a><a>(≥o≤)</a><a>＊＊＊</a><a>不明</a>`, []string{"皆瀬あかり", "別名"}},
		{`不明`, []string{}}, {`皆瀬あかり<br>別名、皆瀬あかり`, []string{"皆瀬あかり", "別名"}},
	} {
		body := strings.Replace(detail, `<a>皆瀬あかり</a>`, tc.actors, 1)
		body = strings.Replace(body, `alt="あかり"`, `alt=""`, 1)
		info, err := fixed(t, 200, body).GetMovieInfoByURL("https://av-wiki.net/hhsi-024/")
		require.NoError(t, err)
		assert.Equal(t, tc.want, []string(info.Actors))
		assert.Equal(t, "あかり", info.Title)
	}
}
func TestSearchAndCID(t *testing.T) {
	calls := []string{}
	p := offline(t, func(r *http.Request) (int, string, error) {
		calls = append(calls, r.URL.RequestURI())
		if r.URL.Path == "/" {
			assert.Equal(t, "hhsi024", r.URL.Query().Get("s"))
			return 200, card("actual-024c"), nil
		}
		require.Equal(t, "/actual-024c/", r.URL.Path)
		return 200, strings.ReplaceAll(detail, "https://av-wiki.net/hhsi-024/", "https://av-wiki.net/actual-024c/"), nil
	})
	info, err := p.GetMovieInfoByID("hhsi024")
	require.NoError(t, err)
	assert.Equal(t, "actual-024c", info.ID)
	require.Len(t, calls, 2)
	keyword := "あかり & +/?=#"
	p = offline(t, func(r *http.Request) (int, string, error) {
		assert.Equal(t, keyword, r.URL.Query().Get("s"))
		return 200, card("hhsi-024"), nil
	})
	results, err := p.SearchMovie(keyword)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "https://av-wiki.net/hhsi-024/", results[0].Homepage)
	assert.Equal(t, "あかり", results[0].Title)
	assert.Equal(t, "https://images.example/thumb.jpg?w=125&h=125", results[0].ThumbURL)
}
func TestCIDToCanonicalIDRoundTrip(t *testing.T) {
	for _, slug := range []string{"actual-024c", "hhsi-024c", "actual024c", "actual_024c"} {
		t.Run(slug, func(t *testing.T) {
			calls := []string{}
			p := offline(t, func(r *http.Request) (int, string, error) {
				calls = append(calls, r.URL.Path)
				if r.URL.Path == "/" {
					return 200, card(slug), nil
				}
				require.Equal(t, "/"+slug+"/", r.URL.Path)
				return 200, strings.ReplaceAll(detail, "/hhsi-024/", "/"+slug+"/"), nil
			})
			first, err := p.GetMovieInfoByID("hhsi024")
			require.NoError(t, err)
			require.Equal(t, slug, first.ID)
			second, err := p.GetMovieInfoByID(first.ID)
			require.NoError(t, err)
			assert.Equal(t, first, second)
			wantCalls := []string{"/", "/" + slug + "/", "/" + slug + "/"}
			if !strings.Contains(slug, "-") {
				wantCalls = []string{"/", "/" + slug + "/", "/", "/" + slug + "/"}
			}
			assert.Equal(t, wantCalls, calls)
		})
	}
	// canonical 和番号/CID 均不对应请求时，不能把另一条记录当作该 slug。
	_, err := fixed(t, 200, detail).GetMovieInfoByID("actual-024c")
	assert.ErrorIs(t, err, provider.ErrInfoNotFound)
}

func TestSearchUnknownAndBrokenTemplates(t *testing.T) {
	for _, body := range []string{
		`<main></main>`,
		`<main><h1>503 Service Unavailable</h1></main>`,
		`<main><p>未知模板</p></main>`,
		`<p>見つかりませんでした</p>`,
		strings.Replace(card("hhsi-024"), `<li>HHSI-024</li>`, `<li></li>`, 1),
		strings.Replace(strings.Replace(card("hhsi-024"), `>あかり</a>`, `></a>`, 1), `alt="あかり"`, `alt=""`, 1),
		strings.Replace(card("hhsi-024"), `href="https://av-wiki.net/hhsi-024/"`, ``, 1),
		card("hhsi-024") + strings.Replace(card("broken-001"), `<li>HHSI-024</li>`, `<li></li>`, 1),
	} {
		p := fixed(t, 200, body)
		_, err := p.SearchMovie("hhsi024")
		assert.ErrorIs(t, err, provider.ErrIncompleteMetadata, body)
		_, err = p.GetMovieInfoByID("hhsi024")
		assert.ErrorIs(t, err, provider.ErrIncompleteMetadata, body)
	}
	// 明确的广告或演员归档入口只跳过，不作为损坏作品卡片。
	ads := strings.ReplaceAll(card("ignored-001"), "https://av-wiki.net/ignored-001/", "https://al.fanza.co.jp/ignored-001/")
	actors := strings.ReplaceAll(card("ignored-002"), "https://av-wiki.net/ignored-002/", "https://av-wiki.net/av-actress/person/")
	results, err := fixed(t, 200, ads+actors).SearchMovie("hhsi024")
	require.NoError(t, err)
	assert.Empty(t, results)
}

func TestExactMatchAndLeadingZeros(t *testing.T) {
	for _, query := range []string{"hhsi24", "hhsi0024", "hhsi-025"} {
		p := offline(t, func(r *http.Request) (int, string, error) {
			if r.URL.Path == "/" {
				return 200, card("hhsi-024"), nil
			}
			return 200, detail, nil
		})
		_, err := p.GetMovieInfoByID(query)
		assert.ErrorIs(t, err, provider.ErrInfoNotFound, query)
	}
}
func TestErrorsAndFallback(t *testing.T) {
	sentinel := errors.New("transport failed")
	for _, tc := range []struct {
		name              string
		status            int
		body              string
		neterr, errorWant error
	}{
		{"challenge", 200, `<title>One moment, please...</title><p>Your request is being verified</p>`, nil, ErrChallenge},
		{"missing", 404, "", nil, provider.ErrInfoNotFound},
		{"incomplete", 200, `<article><h1>誰？</h1></article>`, nil, provider.ErrIncompleteMetadata},
		{"network", 0, "", sentinel, sentinel}, {"server", 503, "error", nil, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			p := offline(t, func(*http.Request) (int, string, error) { calls++; return tc.status, tc.body, tc.neterr })
			_, err := p.GetMovieInfoByURL("https://av-wiki.net/hhsi-024/")
			require.Error(t, err)
			if tc.errorWant != nil {
				assert.ErrorIs(t, err, tc.errorWant)
			}
			assert.Equal(t, 1, calls)
			if tc.name != "incomplete" {
				_, err = p.SearchMovie("hhsi024")
				require.Error(t, err)
				if tc.errorWant != nil {
					assert.ErrorIs(t, err, tc.errorWant)
				}
			}
		})
	}
	calls := 0
	p := offline(t, func(r *http.Request) (int, string, error) {
		calls++
		if r.URL.Path == "/hhsi-024/" {
			return 404, "", nil
		}
		if r.URL.Path == "/" {
			return 200, card("actual-024c"), nil
		}
		return 200, strings.ReplaceAll(detail, "/hhsi-024/", "/actual-024c/"), nil
	})
	info, err := p.GetMovieInfoByID("hhsi-024")
	require.NoError(t, err)
	assert.Equal(t, "actual-024c", info.ID)
	assert.Equal(t, 3, calls)
	for _, status := range []int{200, 503} {
		calls = 0
		p = offline(t, func(*http.Request) (int, string, error) {
			calls++
			if status == 200 {
				return 200, `<title>One moment, please...</title>`, nil
			}
			return 503, "bad", nil
		})
		_, err = p.GetMovieInfoByID("hhsi-024")
		require.Error(t, err)
		assert.Equal(t, 1, calls)
	}
	results, err := fixed(t, 200, `<main><p>見つかりませんでした</p></main>`).SearchMovie("hhsi024")
	require.NoError(t, err)
	assert.Empty(t, results)
	_, err = fixed(t, 200, `<main><p>見つかりませんでした</p></main>`).GetMovieInfoByID("hhsi024")
	assert.ErrorIs(t, err, provider.ErrInfoNotFound)
}
func TestTableIsScopedToCurrentArticle(t *testing.T) {
	decoy := `<aside><dl class="dltable"><dt>メーカー品番</dt><dd>BAD-001</dd><dt>AV女優名</dt><dd>汚染</dd></dl></aside>`
	info, err := fixed(t, 200, decoy+detail).GetMovieInfoByURL("https://av-wiki.net/hhsi-024/")
	require.NoError(t, err)
	assert.Equal(t, "HHSI-024", info.Number)
	assert.Equal(t, []string{"皆瀬あかり"}, []string(info.Actors))
}

func TestImagesAndIncompleteMetadata(t *testing.T) {
	const main = `<div class="article-thumbnail"><img src="data:image/gif;base64,AA" data-src="https://images.example/current.jpg?w=300&amp;h=300" alt="あかり"></div>`
	// 可使用明确标记的当前作品图片，不能使用推荐作品区块。
	body := strings.Replace(detail, main, "", 1)
	product := `<div class="product"><img src="https://images.example/icon.svg"><img src="data:image/png;base64,AA" data-src="//images.example/product.jpg?size=300&amp;v=2"></div>`
	body = strings.Replace(body, `</section>`, product+`<div class="related"><div class="product"><img src="https://images.example/pollution.jpg"></div></div></section>`, 1)
	info, err := fixed(t, 200, body).GetMovieInfoByURL("https://av-wiki.net/hhsi-024/")
	require.NoError(t, err)
	assert.Equal(t, "https://images.example/product.jpg?size=300&v=2", info.CoverURL)
	assert.Equal(t, "あかり", info.Title)
	for _, body := range []string{
		strings.Replace(detail, main, "", 1),
		strings.Replace(detail, main, `<div class="article-thumbnail"><img src="data:image/png;base64,AA"><img src="/icon.svg"><img src="/logo.png"></div>`, 1),
		strings.Replace(strings.Replace(detail, `alt="あかり"`, `alt="誰？"`, 1), `<dd>あかり（えちえち素人 - HHSI-024）</dd>`, `<dd>誰？名前は？</dd>`, 1),
	} {
		_, err := fixed(t, 200, body).GetMovieInfoByURL("https://av-wiki.net/hhsi-024/")
		assert.ErrorIs(t, err, provider.ErrIncompleteMetadata)
	}
	body = strings.Replace(detail, `<dd>2022-09-25</dd>`, `<dd></dd>`, 1)
	info, err = fixed(t, 200, body).GetMovieInfoByURL("https://av-wiki.net/hhsi-024/")
	require.NoError(t, err)
	assert.True(t, time.Time(info.ReleaseDate).IsZero())
}

func TestCIDIsNotNumberAndSearchDoesNotTakeFirst(t *testing.T) {
	calls := []string{}
	p := offline(t, func(r *http.Request) (int, string, error) {
		calls = append(calls, r.URL.Path)
		if r.URL.Path == "/" {
			return 200, card("wrong-001") + card("hhsi-024"), nil
		}
		if r.URL.Path == "/wrong-001/" {
			return 200, strings.ReplaceAll(strings.ReplaceAll(detail, "HHSI-024", "HHSI-025"), "hhsi024", "hhsi025"), nil
		}
		return 200, strings.Replace(detail, "<dd>hhsi024</dd>", "<dd>hhsi00024</dd>", 1), nil
	})
	info, err := p.GetMovieInfoByID("hhsi00024")
	require.NoError(t, err)
	assert.Equal(t, "HHSI-024", info.Number)
	assert.Equal(t, []string{"/", "/wrong-001/", "/hhsi-024/"}, calls)
	// 精确查询必须保留候选详情的验证或网络错误，不能跳过错误继续尝试。
	for _, tc := range []struct {
		status int
		body   string
		err    error
	}{
		{200, `<title>One moment, please...</title>`, ErrChallenge},
		{200, `<main></main>`, provider.ErrIncompleteMetadata},
	} {
		p = offline(t, func(r *http.Request) (int, string, error) {
			if r.URL.Path == "/" {
				return 200, card("hhsi-024"), nil
			}
			return tc.status, tc.body, nil
		})
		_, err = p.GetMovieInfoByID("hhsi024")
		assert.ErrorIs(t, err, tc.err)
	}
}

func TestSearchRejectsAdsArchivesAndDuplicates(t *testing.T) {
	body := card("hhsi-024") + card("hhsi-024") + strings.ReplaceAll(card("ignored-001"), "https://av-wiki.net/ignored-001/", "https://al.fanza.co.jp/ignored-001/") + strings.ReplaceAll(card("ignored-002"), "https://av-wiki.net/ignored-002/", "https://av-wiki.net/av-actress/person/")
	results, err := fixed(t, 200, body).SearchMovie("HHSI-024")
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "HHSI-024", results[0].Number)
	require.True(t, results[0].IsValid())
	_, err = fixed(t, 200, body).SearchMovie("  ")
	assert.ErrorIs(t, err, provider.ErrInvalidKeyword)
}

func TestRedirectValidation(t *testing.T) {
	for _, destination := range []string{"https://evil.example/hhsi-024/", "https://av-wiki.net/av-actress/name/", "https://av-wiki.net/?s=hhsi024", "https://av-wiki.net/hhsi-024c/"} {
		t.Run(destination, func(t *testing.T) {
			p := New()
			calls := 0
			p.ClonedCollector().WithTransport(transportFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				require.Equal(t, "av-wiki.net", r.URL.Host)
				status := 200
				headers := http.Header{"Content-Type": []string{"text/html; charset=utf-8"}}
				body := detail
				if r.URL.Path == "/hhsi-024/" {
					status = 302
					headers.Set("Location", destination)
					body = ""
				}
				return &http.Response{StatusCode: status, Status: fmt.Sprint(status), Header: headers, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
			}))
			_, err := p.GetMovieInfoByURL("https://av-wiki.net/hhsi-024/")
			if strings.HasSuffix(destination, "/hhsi-024c/") {
				require.NoError(t, err)
				assert.Equal(t, 2, calls)
			} else {
				require.ErrorIs(t, err, provider.ErrInvalidURL)
				assert.Equal(t, 1, calls)
			}
		})
	}
}

func TestIDNetworkFailureDoesNotRetry(t *testing.T) {
	calls := 0
	sentinel := errors.New("offline failure")
	p := offline(t, func(*http.Request) (int, string, error) { calls++; return 0, "", sentinel })
	_, err := p.GetMovieInfoByID("hhsi-024")
	require.ErrorIs(t, err, sentinel)
	assert.Equal(t, 1, calls)
}

func TestChallengeWithErrorStatus(t *testing.T) {
	for _, status := range []int{200, 403, 503} {
		p := fixed(t, status, `<p>Your request is being verified</p>`)
		_, err := p.GetMovieInfoByID("hhsi024")
		assert.ErrorIs(t, err, ErrChallenge)
	}
}

func TestCanonicalAndMissingFields(t *testing.T) {
	body := strings.ReplaceAll(detail, "/hhsi-024/", "/hhsi-024c/")
	info, err := fixed(t, 200, body).GetMovieInfoByURL("https://av-wiki.net/hhsi-024/")
	require.NoError(t, err)
	assert.Equal(t, "hhsi-024c", info.ID)
	for _, raw := range []string{"https://evil.example/hhsi-024/", "https://av-wiki.net/?s=x"} {
		_, err := fixed(t, 200, strings.ReplaceAll(detail, "https://av-wiki.net/hhsi-024/", raw)).GetMovieInfoByURL("https://av-wiki.net/hhsi-024/")
		assert.ErrorIs(t, err, provider.ErrInvalidURL)
	}
	_, err = fixed(t, 200, strings.Replace(detail, "<dd>HHSI-024</dd>", "<dd></dd>", 1)).GetMovieInfoByURL("https://av-wiki.net/hhsi-024/")
	assert.ErrorIs(t, err, provider.ErrIncompleteMetadata)
}
