package gyutto

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/text/encoding/japanese"

	"github.com/metatube-community/metatube-sdk-go/provider"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func offline(t *testing.T, f func(*http.Request) (int, string)) *Gyutto {
	t.Helper()
	p := New()
	p.ClonedCollector().WithTransport(transportFunc(func(r *http.Request) (*http.Response, error) {
		require.Equal(t, "gyutto.com", r.URL.Host)
		code, body := f(r)
		return &http.Response{StatusCode: code, Header: http.Header{"Content-Type": {"text/html; charset=utf-8"}}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	}))
	return p
}

const detail = `<meta property="og:url" content="https://gyutto.com/i/item271421"><meta property="og:image" content="http://Gyutto.com/data/item_img/2714/271421/271421.jpg"><meta property="og:description" content="動画：20分　写真集：60枚"><div id="RightSide"><div class="parts_Mds01"><h1>テスト作品</h1></div><div id="noMovie"><div class="unit_DojinMainPh"><div class="ItemPh"><img src="/data/item_img/2714/271421/271421.jpg"></div></div><div class="unit_DetailBasicInfo"><dl class="BasicInfo"><dt>サークル</dt><dd><a>ぷにもえ！</a><span><a>サークル情報</a></span></dd></dl><dl class="BasicInfo"><dt>配信開始日</dt><dd>2024年06月28日</dd></dl><dl class="BasicInfo"><dt>ジャンル</dt><dd><a>分類</a><a>分類</a></dd></dl></div></div><div class="unit_DetailBasicInfo"><dl class="BasicInfo"><dt>サークル</dt><dd>別表示</dd></dl></div><div class="parts_Mds02"><h2>スタッフ情報</h2></div><div class="unit_DetailSummary">関口赤口</div><div class="parts_Mds02"><h2>作品概要</h2></div><div class="unit_DetailSummary"><p>一行目<br>二行目</p><p>三行目</p></div><div class="unit_SamplePhSmall"><a class="highslide" href="/data/item_img/2714/271421/271421_430.jpg"><img src="/thumb.jpg"></a><a class="highslide" href="/data/item_img/2714/271421/271421_430.jpg">重複</a></div></div><aside><div class="parts_Mds01"><h1>汚染</h1></div><div class="unit_SamplePhSmall"><a class="highslide" href="/pollution.jpg"></a></div></aside>`

const search = `<div id="struct_2ColRightIn"><div class="unit_ItemList"><div class="unit_SeachSmall"><ul class="ListBox"><li><dl class="ItemBox"><dd class="DefiPhotoName"><a href="/i/item271421"><span><img alt="テスト作品 完全なタイトル" src="/data/item_img/2714/271421/271421_p_s2.jpg"></span>テスト作品...</a></dd><dd class="DefiAuthor"><a>ぷにもえ！</a></dd></dl></li></ul></div></div></div>`

func TestIdentityAndID(t *testing.T) {
	p := New()
	assert.Equal(t, "Gyutto", p.Name())
	assert.Equal(t, float64(1000), p.Priority())
	for _, id := range []string{"271421", "GYUTTO-271421", "gyutto_271421", " 271421 "} {
		assert.Equal(t, "271421", p.NormalizeMovieID(id))
	}
	p.ClonedCollector().WithTransport(transportFunc(func(*http.Request) (*http.Response, error) { t.Fatal("无效 ID 不得请求"); return nil, nil }))
	for _, id := range []string{"", "0", "../271421", "PNME-224", "271421?x=1"} {
		_, err := p.GetMovieInfoByID(id)
		assert.ErrorIs(t, err, provider.ErrInvalidID)
	}
}

func TestURLs(t *testing.T) {
	p := New()
	for _, raw := range []string{"https://gyutto.com/i/item271421", "https://gyutto.com/i/item271421/", "https://gyutto.com/i/item271421/af-13475?select_uaflag=1", "http://GYUTTO.COM/i/item271421"} {
		id, err := p.ParseMovieIDFromURL(raw)
		require.NoError(t, err)
		assert.Equal(t, "271421", id)
	}
	p.ClonedCollector().WithTransport(transportFunc(func(*http.Request) (*http.Response, error) { t.Fatal("无效 URL 不得请求"); return nil, nil }))
	for _, raw := range []string{"https://gyutto.com/", "https://gyutto.com.evil/i/item271421", "https://evilgyutto.com/i/item271421", "ftp://gyutto.com/i/item271421", "https://u@gyutto.com/i/item271421", "https://gyutto.com:443/i/item271421", "https://gyutto.com/i/item271421?evil=1", "https://gyutto.com/i/item271421?select_uaflag=1&select_uaflag=3", "https://gyutto.com/i/item271421?x=%ZZ", "https://gyutto.com/i/item271421#x", "https://gyutto.com/i/item%32%37%31%34%32%31", "https://gyutto.com/i/item271421/../item1", "https://gyutto.com/i/item271421/af-1/evil"} {
		_, err := p.GetMovieInfoByURL(raw)
		assert.ErrorIs(t, err, provider.ErrInvalidURL, raw)
	}
}

func TestDetail(t *testing.T) {
	p := offline(t, func(r *http.Request) (int, string) {
		assert.Equal(t, "/i/item271421", r.URL.Path)
		assert.Equal(t, "1", r.URL.Query().Get("select_uaflag"))
		return 200, detail
	})
	m, err := p.GetMovieInfoByURL("https://gyutto.com/i/item271421/af-13475?select_uaflag=1")
	require.NoError(t, err)
	assert.True(t, m.IsValid())
	assert.Equal(t, "271421", m.ID)
	assert.Equal(t, "GYUTTO-271421", m.Number)
	assert.Equal(t, "https://gyutto.com/i/item271421", m.Homepage)
	assert.Equal(t, "テスト作品", m.Title)
	assert.Equal(t, "ぷにもえ！", m.Label)
	assert.Equal(t, "2024-06-28", time.Time(m.ReleaseDate).Format("2006-01-02"))
	assert.Equal(t, 20, m.Runtime)
	assert.Equal(t, "https://gyutto.com/data/item_img/2714/271421/271421.jpg", m.CoverURL)
	assert.Equal(t, []string{"分類"}, []string(m.Genres))
	assert.Equal(t, []string{"https://gyutto.com/data/item_img/2714/271421/271421_430.jpg"}, []string(m.PreviewImages))
	assert.Equal(t, "一行目\n二行目\n三行目", m.Summary)
	assert.Empty(t, m.Actors)
	assert.Empty(t, m.PreviewVideoURL)
	p = offline(t, func(*http.Request) (int, string) {
		return 200, strings.Replace(detail, "動画：20分　写真集：60枚", "写真集：60枚", 1)
	})
	m, err = p.GetMovieInfoByID("GYUTTO-271421")
	require.NoError(t, err)
	assert.Zero(t, m.Runtime)
}

func TestSearchEncodingAndDedup(t *testing.T) {
	keyword := "ぬるテカ天国 & +/?=#"
	p := offline(t, func(r *http.Request) (int, string) {
		assert.Equal(t, "/search/search_list.php", r.URL.Path)
		decoded, err := japanese.EUCJP.NewDecoder().String(r.URL.Query().Get("search_keyword"))
		require.NoError(t, err)
		assert.Equal(t, keyword, decoded)
		assert.Equal(t, "10", r.URL.Query().Get("category_id"))
		return 200, search + search
	})
	rs, err := p.SearchMovie(keyword)
	require.NoError(t, err)
	require.Len(t, rs, 1)
	assert.Equal(t, "271421", rs[0].ID)
	assert.Equal(t, "GYUTTO-271421", rs[0].Number)
	assert.Equal(t, "テスト作品 完全なタイトル", rs[0].Title)
	assert.Equal(t, "https://gyutto.com/i/item271421", rs[0].Homepage)
	assert.True(t, rs[0].IsValid())
	for _, k := range []string{" ", "\U00020000"} {
		_, err = p.SearchMovie(k)
		assert.ErrorIs(t, err, provider.ErrInvalidKeyword)
	}
}

func TestEmptyAndBrokenSearch(t *testing.T) {
	const empty = `<div id="struct_2ColRightIn"><div class="parts_Lead"><p><span class="Alert">アダルト作品では、「PNME-224」に一致する作品はありませんでした。</span></p></div></div>`
	for _, tc := range []struct {
		body  string
		empty bool
	}{
		{empty, true},
		{strings.Replace(empty, "struct_2ColRightIn", "sidebar", 1), false},
		{strings.Replace(empty, "PNME-224", "OTHER", 1), false},
		{`<html>unknown</html>`, false},
		{`<div class="unit_ItemList"></div>`, false},
		{strings.Replace(search, `href="/i/item271421"`, `href="https://evil/i/item271421"`, 1), false},
		{strings.Replace(search, `href="/i/item271421"`, ``, 1), false},
		{strings.Replace(strings.Replace(search, `alt="テスト作品 完全なタイトル"`, `alt=""`, 1), `テスト作品...`, "", 1), false},
		{empty + `<form id="challenge-form"></form>`, false},
	} {
		p := offline(t, func(*http.Request) (int, string) { return 200, tc.body })
		rs, err := p.SearchMovie("PNME-224")
		if tc.empty {
			require.NoError(t, err)
			assert.Empty(t, rs)
		} else {
			assert.Error(t, err)
		}
	}
}

func TestErrorsAndCharset(t *testing.T) {
	for _, tc := range []struct {
		status int
		body   string
		want   error
	}{
		{404, "", provider.ErrInfoNotFound},
		{200, `<title>年齢認証</title>`, ErrChallenge},
		{403, `<title>Just a moment...</title>`, ErrChallenge},
		{200, `<html>unknown</html>`, provider.ErrIncompleteMetadata},
		{200, strings.Replace(detail, "https://gyutto.com/i/item271421", "https://evil/i/item271421", 1), provider.ErrInvalidURL},
	} {
		p := offline(t, func(*http.Request) (int, string) { return tc.status, tc.body })
		_, err := p.GetMovieInfoByID("271421")
		assert.ErrorIs(t, err, tc.want)
	}
	p := New()
	calls := 0
	sentinel := errors.New("网络失败")
	p.ClonedCollector().WithTransport(transportFunc(func(*http.Request) (*http.Response, error) { calls++; return nil, sentinel }))
	_, err := p.SearchMovie("title")
	assert.ErrorIs(t, err, sentinel)
	assert.Equal(t, 1, calls)
	body, err := japanese.EUCJP.NewEncoder().String(detail)
	require.NoError(t, err)
	p = New()
	p.ClonedCollector().WithTransport(transportFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/html; charset=EUC-JP"}}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	}))
	m, err := p.GetMovieInfoByID("271421")
	require.NoError(t, err)
	assert.Equal(t, "テスト作品", m.Title)
}

func TestCrossIDRedirect(t *testing.T) {
	p := New()
	calls := 0
	p.ClonedCollector().WithTransport(transportFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		header := http.Header{"Content-Type": {"text/html; charset=utf-8"}}
		code, body := http.StatusOK, strings.ReplaceAll(detail, "271421", "271422")
		if calls == 1 {
			code, body = http.StatusFound, ""
			header.Set("Location", "https://gyutto.com/i/item271422?select_uaflag=1")
		}
		return &http.Response{StatusCode: code, Header: header, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	}))
	m, err := p.GetMovieInfoByID("271421")
	assert.ErrorIs(t, err, provider.ErrInfoNotFound)
	assert.Nil(t, m)
	assert.Equal(t, 2, calls)
}

func TestMissingSummaryDoesNotUseStaff(t *testing.T) {
	p := offline(t, func(*http.Request) (int, string) {
		return http.StatusOK, strings.Replace(detail, `<div class="parts_Mds02"><h2>作品概要</h2></div><div class="unit_DetailSummary"><p>一行目<br>二行目</p><p>三行目</p></div>`, "", 1)
	})
	m, err := p.GetMovieInfoByID("271421")
	require.NoError(t, err)
	assert.Empty(t, m.Summary)
}

func TestRedirectBoundaries(t *testing.T) {
	for _, searching := range []bool{false, true} {
		p := New()
		calls := 0
		p.ClonedCollector().WithTransport(transportFunc(func(r *http.Request) (*http.Response, error) {
			calls++
			return &http.Response{StatusCode: http.StatusFound, Header: http.Header{"Location": {"https://evil/i/item271421"}}, Body: io.NopCloser(strings.NewReader("")), Request: r}, nil
		}))
		var err error
		if searching {
			_, err = p.SearchMovie("title")
		} else {
			_, err = p.GetMovieInfoByID("271421")
		}
		assert.ErrorIs(t, err, provider.ErrInvalidURL)
		assert.Equal(t, 1, calls)
	}
}
