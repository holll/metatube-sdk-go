package getchu

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

func offline(t *testing.T, status int, body string) *Getchu {
	t.Helper()
	p := New()
	p.ClonedCollector().WithTransport(transportFunc(func(r *http.Request) (*http.Response, error) {
		require.Equal(t, "dl.getchu.com", r.URL.Host)
		return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": {"text/html; charset=utf-8"}}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	}))
	return p
}

// 最小人工样本，不复制完整作品介绍。
const detail = `<meta property="og:title" content="テスト作品"><meta property="og:image" content="/data/item_img/40581/4058126/4058126top.jpg"><meta property="og:url" content="https://dl.getchu.com/i/item4058126"><table><tr><td class="bluetext">サークル</td><td><a>ぷにもえ！</a></td></tr><tr><td class="bluetext">作者</td><td>作者名</td></tr><tr><td class="bluetext">画像数&amp;ページ数</td><td>動画：20分　写真集：60枚</td></tr><tr><td class="bluetext">配信開始日</td><td>2024/06/28</td></tr><tr><td class="bluetext">趣向</td><td><a>タグ</a><a>タグ</a><a>分類</a></td></tr><tr><td class="bluetext">作品内容</td><td>一行目<br>二行目<p>三行目</p></td></tr></table><table><tr><td style="background-color: #444444;"><a href="/data/item_img/40581/4058126/4058126_2977.jpg"><img src="/thumb.jpg"></a></td><td style="background-color: #444444;"><a href="/data/item_img/40581/4058126/4058126_2977.jpg">重複</a></td></tr></table><aside><table><tr><td>サークル</td><td>汚染</td></tr></table><td bgcolor="#ffffff"><img src="https://img.example/pollution.jpg"></td></aside>`

func TestIdentityAndIDs(t *testing.T) {
	p := New()
	assert.Equal(t, "Getchu", p.Name())
	assert.Equal(t, float64(1000), p.Priority())
	for _, id := range []string{"4058126", "GETCHU-4058126", "getchu_4058126", " 4058126 "} {
		assert.Equal(t, "4058126", p.NormalizeMovieID(id))
	}
	p = offline(t, 200, detail)
	for _, id := range []string{"", "../4058126", "PNME-224", "0", "https://dl.getchu.com/i/item4058126"} {
		assert.Empty(t, p.NormalizeMovieID(id), id)
		_, err := p.GetMovieInfoByID(id)
		assert.ErrorIs(t, err, provider.ErrInvalidID, id)
	}
}

func TestURLBoundaries(t *testing.T) {
	p := New()
	for _, raw := range []string{"https://dl.getchu.com/i/item4058126", "http://DL.GETCHU.COM/i/item4058126/"} {
		id, err := p.ParseMovieIDFromURL(raw)
		require.NoError(t, err)
		assert.Equal(t, "4058126", id)
	}
	p.ClonedCollector().WithTransport(transportFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("无效输入不得发送请求")
		return nil, nil
	}))
	for _, raw := range []string{"https://evil/i/item4058126", "https://dl.getchu.com.evil/i/item4058126", "ftp://dl.getchu.com/i/item4058126", "https://u@dl.getchu.com/i/item4058126", "https://dl.getchu.com:443/i/item4058126", "https://dl.getchu.com/i/item4058126?x=1", "https://dl.getchu.com/i/item4058126#x", "https://dl.getchu.com/i/item%34%30%35%38%31%32%36", "https://dl.getchu.com/i/item../4058126", "https://dl.getchu.com/", "https://dl.getchu.com/i/item0"} {
		_, err := p.GetMovieInfoByURL(raw)
		assert.ErrorIs(t, err, provider.ErrInvalidURL, raw)
	}
}

func TestDetail(t *testing.T) {
	info, err := offline(t, 200, detail).GetMovieInfoByID("GETCHU-4058126")
	require.NoError(t, err)
	require.True(t, info.IsValid())
	assert.Equal(t, "4058126", info.ID)
	assert.Equal(t, "GETCHU-4058126", info.Number)
	assert.Equal(t, "https://dl.getchu.com/i/item4058126", info.Homepage)
	assert.Equal(t, "テスト作品", info.Title)
	assert.Equal(t, "ぷにもえ！", info.Label)
	assert.Equal(t, 20, info.Runtime)
	assert.Equal(t, "2024-06-28", time.Time(info.ReleaseDate).Format("2006-01-02"))
	assert.Equal(t, "https://dl.getchu.com/data/item_img/40581/4058126/4058126top.jpg", info.CoverURL)
	assert.Equal(t, info.CoverURL, info.ThumbURL)
	assert.Equal(t, []string{"タグ", "分類"}, []string(info.Genres))
	assert.Equal(t, []string{"https://dl.getchu.com/data/item_img/40581/4058126/4058126_2977.jpg"}, []string(info.PreviewImages))
	assert.Equal(t, "一行目\n二行目\n三行目", info.Summary)
	assert.Empty(t, info.Actors)
	info, err = offline(t, 200, strings.Replace(detail, "動画：20分　写真集：60枚", "写真集：60枚", 1)).GetMovieInfoByID("4058126")
	require.NoError(t, err)
	assert.Zero(t, info.Runtime)
}

func TestErrors(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
		want   error
	}{
		{"404", 404, "", provider.ErrInfoNotFound},
		{"empty", 200, "<html>unknown</html>", provider.ErrIncompleteMetadata},
		{"og_only", 200, `<meta property="og:title" content="テスト作品"><meta property="og:image" content="https://img.example/cover.jpg">`, provider.ErrIncompleteMetadata},
		{"cover", 200, strings.Replace(detail, `<meta property="og:image" content="/data/item_img/40581/4058126/4058126top.jpg">`, "", 1), provider.ErrIncompleteMetadata},
		{"title", 200, strings.Replace(detail, `content="テスト作品"`, `content=""`, 1), provider.ErrIncompleteMetadata},
		{"challenge", 403, `<title>Just a moment...</title>`, ErrChallenge},
		{"age", 200, `<title>年齢認証</title>`, ErrChallenge},
		{"canonical", 200, strings.Replace(detail, "https://dl.getchu.com/i/item4058126", "https://evil/i/item4058126", 1), provider.ErrInvalidURL},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := offline(t, tc.status, tc.body).GetMovieInfoByID("4058126")
			assert.ErrorIs(t, err, tc.want)
		})
	}
	p := New()
	calls := 0
	sentinel := errors.New("网络失败")
	p.ClonedCollector().WithTransport(transportFunc(func(*http.Request) (*http.Response, error) { calls++; return nil, sentinel }))
	_, err := p.GetMovieInfoByID("4058126")
	assert.ErrorIs(t, err, sentinel)
	assert.Equal(t, 1, calls)
	_, err = offline(t, 503, "error").GetMovieInfoByID("4058126")
	assert.Error(t, err)
}

func TestCrossIDRedirect(t *testing.T) {
	p := New()
	calls := 0
	p.ClonedCollector().WithTransport(transportFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		header := http.Header{"Content-Type": {"text/html; charset=utf-8"}}
		code, body := http.StatusOK, strings.ReplaceAll(detail, "4058126", "4058127")
		if calls == 1 {
			code, body = http.StatusFound, ""
			header.Set("Location", "https://dl.getchu.com/i/item4058127")
		}
		return &http.Response{StatusCode: code, Header: header, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	}))
	m, err := p.GetMovieInfoByID("4058126")
	assert.ErrorIs(t, err, provider.ErrInfoNotFound)
	assert.Nil(t, m)
	assert.Equal(t, 2, calls)
}

func TestCharset(t *testing.T) {
	body, err := japanese.EUCJP.NewEncoder().String(detail)
	require.NoError(t, err)
	p := New()
	p.ClonedCollector().WithTransport(transportFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/html; charset=EUC-JP"}}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	}))
	m, err := p.GetMovieInfoByID("4058126")
	require.NoError(t, err)
	assert.Equal(t, "テスト作品", m.Title)
}

func TestRedirects(t *testing.T) {
	for _, target := range []string{"https://evil/i/item4058126", "https://dl.getchu.com/", "https://dl.getchu.com/i/item4058126/"} {
		t.Run(target, func(t *testing.T) {
			p := New()
			calls := 0
			p.ClonedCollector().WithTransport(transportFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				header := http.Header{"Content-Type": {"text/html; charset=utf-8"}}
				code, body := 200, detail
				if calls == 1 {
					code, body = 302, ""
					header.Set("Location", target)
				}
				return &http.Response{StatusCode: code, Header: header, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
			}))
			_, err := p.GetMovieInfoByID("4058126")
			if strings.HasSuffix(target, "item4058126/") {
				require.NoError(t, err)
				assert.Equal(t, 2, calls)
			} else {
				assert.ErrorIs(t, err, provider.ErrInvalidURL)
				assert.Equal(t, 1, calls)
			}
		})
	}
}
