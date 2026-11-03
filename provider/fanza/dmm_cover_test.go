package fanza

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/metatube-community/metatube-sdk-go/internal/envconfig"
	"github.com/metatube-community/metatube-sdk-go/provider/internal/dmmcover"
)

type dmmCoverTransport func(*http.Request) (*http.Response, error)

func (f dmmCoverTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestDMMCoverDetail(t *testing.T) {
	const (
		cid         = "test00001"
		oldCover    = "https://images.example.invalid/poster.jpg"
		sample      = "https://pics.dmm.co.jp/digital/video/test00001/test00001sample.jpg"
		otherSample = "https://pics.dmm.co.jp/digital/video/test00002/test00002sample.jpg"
		candidate   = "https://awsimgsrc.dmm.co.jp/pics_dig/digital/video/test00001/test00001pl.jpg"
	)
	var jpegData bytes.Buffer
	require.NoError(t, jpeg.Encode(&jpegData, image.NewRGBA(image.Rect(0, 0, 2, 3)), nil))

	// mono 的 imcmp 使用独立 fetcher 比较缩略图和首张预览图。
	// 两者均指向本地服务，通过 404 使其判为不同图片，避免访问外部图床。
	images := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/thumb.jpg" && r.URL.Path != "/sample.jpg" {
			t.Errorf("unexpected local image request: %s", r.URL)
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(images.Close)
	thumb := images.URL + "/thumb.jpg"

	for _, route := range []string{"digital", "mono"} {
		t.Run(route, func(t *testing.T) {
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
					probes, details, thumbChecks := 0, 0, 0
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

					previews := append([]string(nil), tc.previews...)
					homepage := fmt.Sprintf(movieDigitalAVURL, cid)
					if route == "mono" {
						homepage = fmt.Sprintf(movieMonoDVDURL, cid)
						previews = append([]string{images.URL + "/sample.jpg"}, previews...)
					}
					p.httpClient.Transport = dmmCoverTransport(func(r *http.Request) (*http.Response, error) {
						require.Equal(t, "digital", route, "mono must not call GraphQL")
						details++
						require.Equal(t, "https://api.video.dmm.co.jp/graphql", r.URL.String())
						require.Equal(t, http.MethodPost, r.Method)
						var request struct {
							Variables map[string]interface{} `json:"variables"`
						}
						require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
						require.Equal(t, cid, request.Variables["id"])
						samples := make([]map[string]string, 0, len(previews))
						for _, preview := range previews {
							samples = append(samples, map[string]string{"largeImageUrl": preview})
						}
						// machinebox/graphql 将 data 解包到 ContentPageDataResponse。
						body, err := json.Marshal(map[string]interface{}{"data": map[string]interface{}{
							"ppvContent": map[string]interface{}{
								"id": cid, "floor": "AV", "title": "Test metadata", "makerContentId": "TEST-001",
								"packageImage": map[string]string{"largeUrl": tc.cover, "mediumUrl": thumb},
								"sampleImages": samples,
							},
						}})
						require.NoError(t, err)
						return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(bytes.NewReader(body)), Request: r}, nil
					})
					p.ClonedCollector().WithTransport(dmmCoverTransport(func(r *http.Request) (*http.Response, error) {
						require.Equal(t, http.MethodGet, r.Method)
						if r.URL.String() == thumb && route == "digital" {
							// 满足现有尺寸探测，避免 digital 的 imcmp 访问外部预览图。
							thumbChecks++
							return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"image/jpeg"}, "Content-Length": {"204800"}}, Body: io.NopCloser(bytes.NewReader(jpegData.Bytes())), Request: r}, nil
						}
						require.Equal(t, "mono", route)
						require.Equal(t, homepage, r.URL.String())
						details++
						html := fmt.Sprintf(`<html><body><h1 id="title">Test metadata</h1><img id="package-src-%s" src="%s"><a id="%s" href="%s">Package</a><table><tr><td>品番：</td><td>%s</td></tr></table><div id="sample-image-block">`, cid, thumb, cid, tc.cover, cid)
						for _, preview := range previews {
							html += fmt.Sprintf(`<a name="sample-image"><img src="%s"></a>`, preview)
						}
						html += `</div></body></html>`
						return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/html; charset=utf-8"}}, Body: io.NopCloser(bytes.NewBufferString(html)), Request: r}, nil
					}))

					info, err := p.GetMovieInfoByURL(homepage)
					require.NoError(t, err)
					require.True(t, info.IsValid(), "detail metadata must actually be parsed")
					require.Equal(t, cid, info.ID)
					require.Equal(t, "TEST-001", info.Number)
					require.Equal(t, 1, details)
					wantThumbChecks := 0
					if route == "digital" {
						wantThumbChecks = 1
					}
					require.Equal(t, wantThumbChecks, thumbChecks)
					require.Equal(t, tc.probes, probes)
					wantCover, wantBig := tc.cover, ""
					if tc.probes == 1 && tc.status == http.StatusOK {
						wantCover, wantBig = candidate, candidate
					}
					require.Equal(t, wantCover, info.CoverURL)
					require.Equal(t, wantBig, info.BigCoverURL)
					require.Equal(t, thumb, info.ThumbURL)
					wantBigThumb := ""
					if route == "digital" {
						wantBigThumb = thumb
					}
					require.Equal(t, wantBigThumb, info.BigThumbURL)
					require.Equal(t, previews, []string(info.PreviewImages))
				})
			}
		})
	}
}

func TestDMMCoverInvalidConfig(t *testing.T) {
	p := New()
	config := envconfig.NewConfig()
	config.Set("use_dmm_hd_cover", "invalid")
	require.ErrorContains(t, p.SetConfig(config), "use_dmm_hd_cover")
}
