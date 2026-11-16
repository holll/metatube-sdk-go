package javtrailers

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type galleryTransport func(*http.Request) (*http.Response, error)

func (f galleryTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// Reduced from the live Nuxt table: indices are references, not literal values.
const galleryData = `[["Reactive",1],{"data":2},{"video-ssis00001":3,"video-suggestions-ssis00001":12},{"video":4},{"contentId":5,"gallery":6},"ssis00001",[7,8,7,9,10,11,-1,999],"https://pics.dmm.co.jp/digital/video/ssis00001/ssis00001-1.jpg","//pics.dmm.co.jp/digital/video/ssis00001/ssis00001-2.jpg","data:image/gif;base64,AA","javascript:alert(1)","https://example.com/trailer.mp4",{"video":13},{"contentId":14,"gallery":15},"other00001",[16],"https://example.com/other.jpg"]`

func TestGalleryFromNuxtData(t *testing.T) {
	for _, tt := range []struct {
		name string
		data string
		want []string
	}{
		{"gallery", galleryData, []string{"https://pics.dmm.co.jp/digital/video/ssis00001/ssis00001-1.jpg", "https://pics.dmm.co.jp/digital/video/ssis00001/ssis00001-2.jpg"}},
		{"missing", "", []string{}},
		{"malformed", "{bad", []string{}},
		{"gallery wrong type", strings.Replace(galleryData, `[7,8,7,9,10,11,-1,999]`, `{"unexpected":7}`, 1), []string{}},
		{"empty gallery", strings.Replace(galleryData, `[7,8,7,9,10,11,-1,999]`, `[]`, 1), []string{}},
		{"different movie", strings.Replace(galleryData, `"contentId":5`, `"contentId":14`, 1), []string{}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			p := New()
			p.ClonedCollector().WithTransport(galleryTransport(func(r *http.Request) (*http.Response, error) {
				body := `<h1>SSIS-001 Test title</h1><p class="mb-1"><span class="font-weight-bold mr-3">DVD ID:</span>SSIS-001</p><script type="application/ld+json">{"@type":"VideoObject","thumbnailUrl":"https://example.com/cover.webp"}</script><img src="https://example.com/ad.jpg"><script id="__NUXT_DATA__" type="application/json">` + tt.data + `</script>`
				return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/html"}}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
			}))
			info, err := p.GetMovieInfoByURL("https://javtrailers.com/ja/video/ssis00001")
			require.NoError(t, err)
			require.True(t, info.IsValid())
			assert.Equal(t, tt.want, []string(info.PreviewImages))
		})
	}
}

func TestNuxtGalleryNestedWrappers(t *testing.T) {
	raw := `[["ShallowReactive",1],{"data":2},["Ref",3],{"video-ssis00001":4},["ShallowRef",5],{"video":6},["Reactive",7],{"contentId":8,"gallery":9},"ssis00001",[10,11],"https://example.com/still.jpg",{"not":"a string"}]`
	assert.Equal(t, []string{"https://example.com/still.jpg"}, nuxtGallery([]byte(raw), "ssis00001"))
}

func TestNuxtGalleryRejectsInvalidGraphs(t *testing.T) {
	for _, raw := range []string{`[]`, `null`, `[null]`, `[["Reactive",0]]`, `[["Reactive",-1]]`, `[["Reactive",99]]`, `[["Unknown",1],{}]`, `[{"data":0}]`, `[{"data":1},null]`} {
		assert.Empty(t, nuxtGallery([]byte(raw), "ssis00001"), raw)
	}
}
