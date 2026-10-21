package airavcity

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

func TestIdentity(t *testing.T) {
	p := New()
	require.Equal(t, Name, p.Name())
	require.Equal(t, float64(Priority), p.Priority())
	require.Equal(t, "airavcity4.cc", p.URL().Hostname())
}

// 最小人工样本：只保留解析所需结构，不含真实站点正文。
const detail = `<html><head>
<meta property="og:image" content="https://airavcity4.cc/storage/cover/big/99-21-14639.jpg">
<meta property="og:title" content="STARS-087 永野一夏 AV出道 - airav.io">
<title>STARS-087 永野一夏 AV出道 - airav.io</title></head><body>
<div class="video-title my-3"><h1>STARS-087 永野一夏 AV出道</h1></div>
<div class="video-item"><div class="me-4"><i class="fa fa-clock me-2"></i>2019-06-20 00:00:00</div>
<div><i class="fa fa-eye me-2"></i>704032</div></div>
<div class="video-info"><p class="my-3">低身高，腰圍54公分的纖細身軀。</p>
<div class="info-list my-2"><ul class="list-group">
<li class="my-2">番號：<span>STARS-087</span></li>
<li class="my-2">女優： <a href="/actor?id=1759">永野一夏</a> </li>
<li class="my-2">標籤： <a href="/tag?tid=5">720p</a> <a href="/tag?tid=55">美少女</a> </li>
<li class="my-2">廠商： <a href="/tag?fid=8">SOD</a> </li>
</ul></div></div>
<div class="recommend"><img src="https://airavcity4.cc/storage/cover/big/99-21-71174.jpg"></div>
</body></html>`

func card(hid, title string) string {
	return `<div class="col oneVideo"><div class="card h-100">
<div class="oneVideo-top"><a href="/video?hid=` + hid + `">
<img src="https://airavcity4.cc/storage/cover/big/` + hid + `.jpg?1792267394" class="card-img-top" alt="..."></a></div>
<div class="oneVideo-body"><h5>` + title + `</h5>
<div class="oneVideo-fotter"><p><i class="fa fa-eye"></i>704032</p></div></div>
</div></div>`
}

func searchPage(cards ...string) string {
	return `<html><body><h1>KEY<span>的搜尋結果</span></h1>
<div class="row row-cols-2 row-cols-lg-4 g-2 mt-0">` + strings.Join(cards, "") + `</div></body></html>`
}

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func offline(t *testing.T, f func(*http.Request) (int, string, error)) *AirAVCity {
	t.Helper()
	p := New()
	p.ClonedCollector().WithTransport(transportFunc(func(r *http.Request) (*http.Response, error) {
		require.Equal(t, "airavcity4.cc", r.URL.Host)
		status, body, err := f(r)
		if err != nil {
			return nil, err
		}
		return &http.Response{
			StatusCode: status,
			Status:     fmt.Sprint(status),
			Header:     http.Header{"Content-Type": []string{"text/html; charset=utf-8"}},
			Body:       io.NopCloser(strings.NewReader(body)),
			Request:    r,
		}, nil
	}))
	return p
}

func fixed(t *testing.T, status int, body string) *AirAVCity {
	return offline(t, func(*http.Request) (int, string, error) { return status, body, nil })
}

func TestRegistration(t *testing.T) {
	found := false
	provider.RangeMovieFactory(func(name string, f provider.MovieFactory) bool {
		if name == Name {
			found = true
			assert.Equal(t, float64(Priority), f().Priority())
			_, ok := f().(provider.MovieSearcher)
			assert.True(t, ok)
		}
		return true
	})
	require.True(t, found)
	// 本站不提供演员资料。
	provider.RangeActorFactory(func(name string, _ provider.ActorFactory) bool {
		assert.NotEqual(t, Name, name)
		return true
	})
}

func TestURLAndIDValidation(t *testing.T) {
	p := offline(t, func(*http.Request) (int, string, error) {
		t.Fatal("invalid input must not request")
		return 0, "", nil
	})
	for _, raw := range []string{
		"https://airavcity4.cc/",
		"https://airavcity4.cc/search_result?kw=STARS-087",
		"https://airavcity4.cc/video",
		"https://airavcity4.cc/video?hid=",
		"https://airavcity4.cc/video?hid=99-21-14639&x=1",
		"https://airavcity4.cc/video/99-21-14639",
		"https://airavcity4.cc/video?id=99-21-14639",
		"https://evil-airavcity4.cc/video?hid=99-21-14639",
		"https://airavcity4.cc.evil/video?hid=99-21-14639",
		"https://airavcity4.cc@evil/video?hid=99-21-14639",
		"https://user@airavcity4.cc/video?hid=99-21-14639",
		"http://airavcity4.cc:443/video?hid=99-21-14639",
		"ftp://airavcity4.cc/video?hid=99-21-14639",
		"https://airavcity4.cc/video?hid=../etc/passwd",
		"https://airavcity4.cc/video?hid=99%2F21",
		"https://airavcity4.cc/video?hid=99-21-14639#x",
	} {
		_, err := p.GetMovieInfoByURL(raw)
		require.ErrorIs(t, err, provider.ErrInvalidURL, raw)
	}
	for _, id := range []string{"", "  ", "../x", "x/y", "x?y=1", "https://airavcity4.cc/video?hid=1", "x%2fy"} {
		assert.Empty(t, p.NormalizeMovieID(id), id)
		_, err := p.GetMovieInfoByID(id)
		require.ErrorIs(t, err, provider.ErrInvalidID, id)
	}
	for _, id := range []string{"99-21-14639", "QC-BT-4734904", "127999", "FNS-266"} {
		assert.NotEmpty(t, p.NormalizeMovieID(id), id)
	}
}

func TestDetail(t *testing.T) {
	info, err := fixed(t, 200, detail).GetMovieInfoByURL("https://airavcity4.cc/video?hid=99-21-14639")
	require.NoError(t, err)
	assert.Equal(t, "99-21-14639", info.ID)
	assert.Equal(t, "STARS-087", info.Number)
	// 站点标题带番号前缀，与番号一致时去掉，便于跨源比较。
	assert.Equal(t, "永野一夏 AV出道", info.Title)
	assert.Equal(t, Name, info.Provider)
	assert.Equal(t, "https://airavcity4.cc/video?hid=99-21-14639", info.Homepage)
	assert.Equal(t, "低身高，腰圍54公分的纖細身軀。", info.Summary)
	assert.Equal(t, []string{"永野一夏"}, []string(info.Actors))
	assert.Equal(t, []string{"720p", "美少女"}, []string(info.Genres))
	assert.Equal(t, "SOD", info.Maker)
	assert.Equal(t, "2019-06-20", time.Time(info.ReleaseDate).Format("2006-01-02"))
	// 必须取 og:image，推荐作品同样使用 cover/big 路径。
	assert.Equal(t, "https://airavcity4.cc/storage/cover/big/99-21-14639.jpg", info.CoverURL)
	assert.Equal(t, info.CoverURL, info.ThumbURL)
	assert.Zero(t, info.Runtime)
	assert.Empty(t, info.PreviewVideoURL)
	assert.Empty(t, info.PreviewVideoHLSURL)
}

func TestDetailMultipleActorsAndOptionalFields(t *testing.T) {
	body := strings.Replace(detail, `<a href="/actor?id=1759">永野一夏</a>`,
		`<a href="/actor?id=463">葵司</a> <a href="/actor?id=4661">乙白沙也加</a>`, 1)
	body = strings.Replace(body, `<a href="/tag?tid=5">720p</a> <a href="/tag?tid=55">美少女</a>`, ``, 1)
	body = strings.Replace(body, `<a href="/tag?fid=8">SOD</a>`, ``, 1)
	body = strings.Replace(body, `2019-06-20 00:00:00`, ``, 1)
	info, err := fixed(t, 200, body).GetMovieInfoByURL("https://airavcity4.cc/video?hid=127999")
	require.NoError(t, err)
	assert.Equal(t, []string{"葵司", "乙白沙也加"}, []string(info.Actors))
	assert.Empty(t, info.Genres)
	assert.Empty(t, info.Maker)
	assert.True(t, time.Time(info.ReleaseDate).IsZero())
}

func TestDetailIncompleteMetadata(t *testing.T) {
	for name, body := range map[string]string{
		"no-number":  strings.Replace(detail, `番號：<span>STARS-087</span>`, `番號：<span></span>`, 1),
		"no-title":   strings.Replace(detail, `<div class="video-title my-3"><h1>STARS-087 永野一夏 AV出道</h1></div>`, ``, 1),
		"no-cover":   strings.Replace(detail, `<meta property="og:image" content="https://airavcity4.cc/storage/cover/big/99-21-14639.jpg">`, ``, 1),
		"placeholder": `<html><head><title>免費A片</title></head><body><div class="video-title"><h1></h1></div></body></html>`,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := fixed(t, 200, body).GetMovieInfoByURL("https://airavcity4.cc/video?hid=99-21-14639")
			assert.ErrorIs(t, err, provider.ErrIncompleteMetadata)
		})
	}
}

func TestSearch(t *testing.T) {
	body := searchPage(
		card("99-21-14639", "STARS-087 永野一夏 AV出道"),
		card("127999", "STARS-087 永野一夏 AV出道"),
		// 系列合集标题没有番号前缀，应跳过。
		card("99-18-5202", "肛交巨星 4"),
		card("99-21-14639", "STARS-087 永野一夏 AV出道"),
	)
	results, err := fixed(t, 200, body).SearchMovie("STARS-087")
	require.NoError(t, err)
	require.Len(t, results, 2)
	assert.Equal(t, "99-21-14639", results[0].ID)
	assert.Equal(t, "STARS-087", results[0].Number)
	assert.Equal(t, "永野一夏 AV出道", results[0].Title)
	assert.Equal(t, "https://airavcity4.cc/video?hid=99-21-14639", results[0].Homepage)
	assert.Equal(t, "https://airavcity4.cc/storage/cover/big/99-21-14639.jpg?1792267394", results[0].CoverURL)
	assert.Equal(t, "127999", results[1].ID)
	for _, r := range results {
		assert.True(t, r.IsValid(), r)
	}
}

func TestSearchEmptyAndBrokenTemplates(t *testing.T) {
	// 结构完整但没有卡片：真实空结果。
	results, err := fixed(t, 200, searchPage()).SearchMovie("ZZZZ-999")
	require.NoError(t, err)
	assert.Empty(t, results)

	for name, body := range map[string]string{
		"unknown":  `<html><body><div>未知模板</div></body></html>`,
		"no-row":   `<html><body><h1>的搜尋結果</h1></body></html>`,
		"bad-hid":  searchPage(card("../etc/passwd", "STARS-087 x")),
		"no-title": searchPage(`<div class="col oneVideo"><a href="/video?hid=99-21-14639"></a></div>`),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := fixed(t, 200, body).SearchMovie("STARS-087")
			assert.ErrorIs(t, err, provider.ErrIncompleteMetadata)
		})
	}

	_, err = fixed(t, 200, searchPage()).SearchMovie("  ")
	assert.ErrorIs(t, err, provider.ErrInvalidKeyword)
}

func TestSearchPaginationParam(t *testing.T) {
	calls := []string{}
	p := offline(t, func(r *http.Request) (int, string, error) {
		calls = append(calls, r.URL.RequestURI())
		assert.Equal(t, "STARS-087", r.URL.Query().Get("kw"))
		return 200, searchPage(card("99-21-14639", "STARS-087 永野一夏 AV出道")), nil
	})
	_, err := p.SearchMovie("STARS-087")
	require.NoError(t, err)
	require.Len(t, calls, 1)
	assert.Equal(t, "/search_result?kw=STARS-087", calls[0])
}

func TestGetMovieInfoByIDByNumber(t *testing.T) {
	calls := []string{}
	p := offline(t, func(r *http.Request) (int, string, error) {
		calls = append(calls, r.URL.RequestURI())
		if r.URL.Path == "/search_result" {
			return 200, searchPage(
				card("99-21-70529", "FNS-265 別的作品"),
				card("QC-BT-4734904", "FNS-266 工作合不來的同事"),
			), nil
		}
		require.Equal(t, "qc-bt-4734904", r.URL.Query().Get("hid"))
		return 200, strings.Replace(strings.Replace(detail, "STARS-087", "FNS-266", -1), "99-21-14639", "QC-BT-4734904", -1), nil
	})
	info, err := p.GetMovieInfoByID("FNS-266")
	require.NoError(t, err)
	assert.Equal(t, "qc-bt-4734904", info.ID)
	assert.Equal(t, "FNS-266", info.Number)
	assert.Equal(t, []string{"/search_result?kw=fns-266", "/video?hid=qc-bt-4734904"}, calls)
}

func TestGetMovieInfoByIDByHID(t *testing.T) {
	calls := []string{}
	p := offline(t, func(r *http.Request) (int, string, error) {
		calls = append(calls, r.URL.RequestURI())
		return 200, detail, nil
	})
	info, err := p.GetMovieInfoByID("99-21-14639")
	require.NoError(t, err)
	assert.Equal(t, "99-21-14639", info.ID)
	// hid 直接取详情，不经过搜索。
	assert.Equal(t, []string{"/video?hid=99-21-14639"}, calls)
}

func TestGetMovieInfoByIDNumberMismatch(t *testing.T) {
	p := offline(t, func(r *http.Request) (int, string, error) {
		if r.URL.Path == "/search_result" {
			return 200, searchPage(card("99-21-70529", "FNS-265 別的作品")), nil
		}
		return 200, strings.Replace(detail, "STARS-087", "FNS-265", -1), nil
	})
	_, err := p.GetMovieInfoByID("FNS-266")
	assert.ErrorIs(t, err, provider.ErrInfoNotFound)
}

func TestErrorsAndChallenge(t *testing.T) {
	sentinel := errors.New("transport failed")
	for name, tc := range map[string]struct {
		status int
		body   string
		neterr error
		want   error
	}{
		"challenge":  {200, `<html><head><title>Just a moment...</title></head></html>`, nil, ErrChallenge},
		"not-found":  {404, `<html><body>404</body></html>`, nil, provider.ErrInfoNotFound},
		"network":    {0, "", sentinel, sentinel},
		"server":     {503, "error", nil, nil},
	} {
		t.Run(name, func(t *testing.T) {
			p := offline(t, func(*http.Request) (int, string, error) { return tc.status, tc.body, tc.neterr })
			_, err := p.GetMovieInfoByURL("https://airavcity4.cc/video?hid=99-21-14639")
			require.Error(t, err)
			if tc.want != nil {
				assert.ErrorIs(t, err, tc.want)
			}
			if tc.neterr != nil {
				assert.ErrorIs(t, err, sentinel)
			}
		})
	}
	// 搜索页同样不能把挑战页当成空结果。
	_, err := fixed(t, 200, `<html><head><title>Just a moment...</title></head></html>`).SearchMovie("STARS-087")
	assert.ErrorIs(t, err, ErrChallenge)
}

func TestRedirectValidation(t *testing.T) {
	for destination, allowed := range map[string]bool{
		"https://airavcity4.cc/video?hid=99-21-14639":     true,
		"https://airavcity4.cc/video?hid=qc-bt-4734904":   true,
		"https://evil.example/video?hid=99-21-14639":      false,
		"https://airavcity4.cc/search_result?kw=STARS-087": false,
		"https://airavcity4.cc/":                          false,
	} {
		t.Run(destination, func(t *testing.T) {
			p := New()
			calls := 0
			p.ClonedCollector().WithTransport(transportFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				require.Equal(t, "airavcity4.cc", r.URL.Host)
				status, body := 200, detail
				headers := http.Header{"Content-Type": []string{"text/html; charset=utf-8"}}
				// 只在首次请求时重定向，避免同一目标再次 302 造成循环。
				if calls == 1 && r.URL.Path == "/video" && r.URL.Query().Get("hid") == "99-21-14639" {
					status, body = 302, ""
					headers.Set("Location", destination)
				}
				return &http.Response{StatusCode: status, Status: fmt.Sprint(status), Header: headers, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
			}))
			_, err := p.GetMovieInfoByURL("https://airavcity4.cc/video?hid=99-21-14639")
			if allowed {
				require.NoError(t, err)
				assert.Equal(t, 2, calls)
				return
			}
			assert.ErrorIs(t, err, provider.ErrInvalidURL)
			assert.Equal(t, 1, calls)
		})
	}
}
