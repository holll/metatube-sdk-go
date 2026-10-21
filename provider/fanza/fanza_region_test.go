package fanza

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestURLAliases(t *testing.T) {
	p := New()
	aliases := p.URLAliases()
	require.Len(t, aliases, 1)
	assert.Equal(t, "video.dmm.co.jp", aliases[0].Hostname())
	assert.NotEqual(t, p.URL().Hostname(), aliases[0].Hostname())
}

// 地域限制页返回 HTTP 200 且不含 next_f 数据，搜索必须返回
// ErrRegionNotAvailable，而不是晦涩的解析错误。
// 真实站点会 302 到 /not-available-in-your-region/，这里用同一个
// RoundTripper 复刻该跳转。
func TestSearchMovieRegionNotAvailable(t *testing.T) {
	p := New()
	p.ClonedCollector().WithTransport(roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if strings.Contains(r.URL.Path, regionNotAvailable) {
			body := `<html><head><title>このページはお住まいの地域からご利用になれません。</title></head></html>`
			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Header:     http.Header{"Content-Type": []string{"text/html; charset=utf-8"}},
				Body:       io.NopCloser(strings.NewReader(body)),
				Request:    r,
			}, nil
		}
		return &http.Response{
			StatusCode: http.StatusFound,
			Status:     "302 Found",
			Header: http.Header{
				"Location": []string{"https://special.fanza.jp/" + regionNotAvailable + "/allservice/"},
			},
			Body:    io.NopCloser(strings.NewReader("")),
			Request: r,
		}, nil
	}))
	_, err := p.SearchMovie("HHSI-024")
	require.ErrorIs(t, err, ErrRegionNotAvailable)
}
