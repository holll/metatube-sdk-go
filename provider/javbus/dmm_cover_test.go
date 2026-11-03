package javbus

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/metatube-community/metatube-sdk-go/internal/envconfig"
	"github.com/metatube-community/metatube-sdk-go/provider/internal/dmmcover"
)

type dmmCoverTransport func(*http.Request) (*http.Response, error)

func (f dmmCoverTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestDMMCoverDetail(t *testing.T) {
	const (
		homepage    = "https://www.javbus.com/ja/TEST-001"
		oldCover    = "https://images.example.invalid/poster.jpg" // 避免触发现有 /cover/ 缩略图 HEAD 探测。
		sample      = "https://pics.dmm.co.jp/digital/video/test00001/test00001jp-1.jpg"
		otherSample = "https://pics.dmm.co.jp/digital/video/test00002/test00002jp-1.jpg"
		candidate   = "https://awsimgsrc.dmm.co.jp/pics_dig/digital/video/test00001/test00001pl.jpg"
	)
	var jpegData bytes.Buffer
	require.NoError(t, jpeg.Encode(&jpegData, image.NewRGBA(image.Rect(0, 0, 2, 3)), nil))

	for _, tc := range []struct {
		name, config, cover string
		previews            []string
		status, probes      int
	}{
		{"default", "", oldCover, []string{sample}, http.StatusOK, 0},
		{"false", "false", oldCover, []string{sample}, http.StatusOK, 0},
		{"jpeg", "true", oldCover, []string{sample}, http.StatusOK, 1},
		{"not_found", "true", oldCover, []string{sample}, http.StatusNotFound, 1},
		{"no_cid", "true", oldCover, []string{"https://images.example.invalid/sample.jpg"}, http.StatusOK, 0},
		{"conflicting_previews", "true", oldCover, []string{sample, otherSample}, http.StatusOK, 0},
		{"conflicting_cover", "true", "https://pics.dmm.co.jp/digital/video/test00002/test00002pl.jpg", []string{sample}, http.StatusOK, 0},
		{"already_hd", "true", candidate, []string{sample}, http.StatusOK, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := New()
			probes, details := 0, 0
			p.hdCover = dmmcover.New(&http.Client{Transport: dmmCoverTransport(func(r *http.Request) (*http.Response, error) {
				probes++
				require.Equal(t, candidate, r.URL.String())
				require.Equal(t, http.MethodGet, r.Method)
				return &http.Response{StatusCode: tc.status, Header: http.Header{"Content-Type": {"image/jpeg"}}, Body: io.NopCloser(bytes.NewReader(jpegData.Bytes())), Request: r}, nil
			})})
			if tc.config != "" {
				config := envconfig.NewConfig()
				config.Set("use_dmm_hd_cover", tc.config)
				require.NoError(t, p.SetConfig(config))
			}
			html := fmt.Sprintf(`<html><body><a class="bigImage"><img title="Test metadata" src="%s"></a><div class="col-md-3 info"><p><span>品番:</span><span>TEST-001</span></p></div><div id="sample-waterfall">`, tc.cover)
			for _, preview := range tc.previews {
				html += fmt.Sprintf(`<a href="%s">Sample</a>`, preview)
			}
			html += `</div></body></html>`
			p.ClonedCollector().WithTransport(dmmCoverTransport(func(r *http.Request) (*http.Response, error) {
				details++
				require.Equal(t, homepage, r.URL.String())
				require.Equal(t, http.MethodGet, r.Method)
				return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/html; charset=utf-8"}}, Body: io.NopCloser(bytes.NewBufferString(html)), Request: r}, nil
			}))

			info, err := p.GetMovieInfoByURL(homepage)
			require.NoError(t, err)
			require.True(t, info.IsValid(), "detail metadata must actually be parsed")
			require.Equal(t, "TEST-001", info.Number)
			require.Equal(t, 1, details)
			require.Equal(t, tc.probes, probes)
			wantCover, wantBig := tc.cover, ""
			if tc.probes == 1 && tc.status == http.StatusOK {
				wantCover, wantBig = candidate, candidate
			}
			require.Equal(t, wantCover, info.CoverURL)
			require.Equal(t, wantBig, info.BigCoverURL)
			require.Empty(t, info.ThumbURL)
			require.Empty(t, info.BigThumbURL)
			require.Equal(t, tc.previews, []string(info.PreviewImages))
		})
	}
}

func TestDMMCoverInvalidConfig(t *testing.T) {
	p := New()
	config := envconfig.NewConfig()
	config.Set("use_dmm_hd_cover", "invalid")
	require.ErrorContains(t, p.SetConfig(config), "use_dmm_hd_cover")
}
