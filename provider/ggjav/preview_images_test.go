package ggjav

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type previewTransportFunc func(*http.Request) (*http.Response, error)

func (f previewTransportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestDetailCurrentTemplate(t *testing.T) {
	p := New()
	p.ClonedCollector().WithTransport(previewTransportFunc(func(r *http.Request) (*http.Response, error) {
		body := `<h2 class="columns small-12 title_text">KTRA-746 Test movie</h2>
<div class="video_cover"><img src="https://cdn-1.ggjav.com/media/video/large_301794.jpg"></div>
<div class="meta_label">番号：</div><div class="video_number">KTRA-746</div>
<div class="previews"><h2 class="title_text">プレビュー</h2><img src="https://cdn-1.ggjav.com/media/preview/301794_0.jpg"></div>
<h2 class="title_text">その他の関連動画</h2>`
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/html"}}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	}))
	info, err := p.GetMovieInfoByID("301794")
	require.NoError(t, err)
	require.True(t, info.IsValid())
	assert.Equal(t, "KTRA-746 Test movie", info.Title)
	assert.Equal(t, "KTRA-746", info.Number)
	assert.Equal(t, []string{"https://cdn-1.ggjav.com/media/preview/301794_0.jpg"}, []string(info.PreviewImages))
}

func TestDetailPreviewImages(t *testing.T) {
	for _, tt := range []struct {
		name, gallery string
		want          []string
	}{
		{"gallery", `<div class="columns chunk previews">
<img src="https://cdn-1.ggjav.com/media/preview/301794_0.jpg" loading="lazy">
<img src="/resources/icons/no_image.png" data-src="//cdn-1.ggjav.com/media/preview/301794_1.jpg">
<img src="data:image/gif;base64,AA" data-original="/media/preview/301794_2.jpg">
<img data-lazy-src="https://cdn-1.ggjav.com/media/preview/301794_3.jpg?token=x&amp;size=full">
<img src="https://cdn-1.ggjav.com/media/preview/301794_0.jpg#zoom">
<img src="https://cdn-1.ggjav.com/media/preview/301794_4.jpg" data-src="https://cdn-1.ggjav.com/media/preview/301794_1.jpg">
<img src="https://cdn-1.ggjav.com/media/preview/301794_5.jpg" data-src="javascript:alert(1)">
<img data-src="data:image/jpeg;/media/preview/301794_6.jpg"><img src="javascript:/media/preview/301794_7.jpg">
<img src="https://cdn-1.ggjav.com/media/preview/999999_0.jpg"><img src="https://cdn-1.ggjav.com/media/video/large_301794.jpg">
<img src="/resources/icons/no_image.png"><img src="/media/preview/301794_no_image.jpg">
<img src="https://cdn-1.ggjav.com/ad.jpg?next=/media/preview/301794_8.jpg">
<img src="/media/preview/301794_9.mp4"><img src="/media/preview/301794_10.jpg/other">
</div><div class="other-previews"><img src="/media/preview/301794_11.jpg"></div>
<div class="recommend"><img src="/media/preview/301794_12.jpg"></div>`, []string{
			"https://cdn-1.ggjav.com/media/preview/301794_0.jpg",
			"https://cdn-1.ggjav.com/media/preview/301794_1.jpg",
			"https://ggjav.com/media/preview/301794_2.jpg",
			"https://cdn-1.ggjav.com/media/preview/301794_3.jpg?token=x&size=full",
			"https://cdn-1.ggjav.com/media/preview/301794_5.jpg",
		}},
		{"no gallery", `<div class="previews"><img src="/resources/icons/no_image.png"></div>`, []string{}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			p := New()
			p.ClonedCollector().WithTransport(previewTransportFunc(func(r *http.Request) (*http.Response, error) {
				body := `<div class="title_text">Test movie</div><div>番号：KTRA-746</div><div class="info"><div class="large-6"><img src="https://cdn-1.ggjav.com/media/video/large_301794.jpg"></div></div>` + tt.gallery
				return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/html"}}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
			}))
			info, err := p.GetMovieInfoByID("301794")
			require.NoError(t, err)
			require.True(t, info.IsValid())
			assert.Equal(t, tt.want, []string(info.PreviewImages))
			assert.Equal(t, "https://cdn-1.ggjav.com/media/video/large_301794.jpg", info.CoverURL)
		})
	}
}
