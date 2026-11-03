package tokyohot

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/metatube-community/metatube-sdk-go/provider/internal/testkit"
)

func TestTokyoHot_GetMovieInfoByID(t *testing.T) {
	testkit.Test(t, New, []string{
		"s2mbd-002",
		"n1633",
		"n1624",
		"5656",
		"kb1624",
	})
}

func TestNormalizeMovieKeyword(t *testing.T) {
	for _, tc := range []struct{ input, want string }{
		{"N-960", "n0960"},
		{"n960", "n0960"},
		{"N_0960", "n0960"},
		{"n0238", "n0238"},
		{"n1633", "n1633"},
		{"kb1624", "kb1624"},
		{"5782", "5782"},
		{"NHDTC-1094", ""},
		{"N-960-C", ""},
	} {
		if got := New().NormalizeMovieKeyword(tc.input); got != tc.want {
			t.Errorf("%q: got %q, want %q", tc.input, got, tc.want)
		}
	}
}

type searchTransport func(*http.Request) (*http.Response, error)

func (f searchTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestSearchMovieNumberVariant(t *testing.T) {
	p := New()
	calls := 0
	p.ClonedCollector().WithTransport(searchTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Query().Get("q") != "n0960" {
			t.Fatalf("错误的搜索词: %s", r.URL)
		}
		body := `<div id="main"><ul><li><a href="/product/5782/"><img src="/cover.jpg" alt="n0960"></a><div class="title">テスト作品</div>(作品番号: n0960)</li></ul></div>`
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/html"}}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	}))
	results, err := p.SearchMovie("N-960")
	if err != nil || len(results) != 1 {
		t.Fatal(results, err)
	}
	if calls != 1 || results[0].ID != "5782" || results[0].Number != "n0960" || !results[0].IsValid() {
		t.Fatal(calls, results[0])
	}
}

func TestTokyoHot_SearchMovie(t *testing.T) {
	testkit.Test(t, New, []string{
		"1624",
		"n0238",
	})
}
