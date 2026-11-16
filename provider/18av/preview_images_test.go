package eighteenav

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

// Mirrors the observed detail template: the player poster, actor photos, ads and
// recommended movies are not stills of the current movie.
func TestDetailDoesNotInventPreviewImages(t *testing.T) {
	p := New()
	p.ClonedCollector().WithTransport(previewTransportFunc(func(r *http.Request) (*http.Response, error) {
		body := `<div class="archive-title"><h1>NHDTB-817 Test movie</h1></div>
<div id="dz_video"><div id="player-wrap"><img src="https://fchost1.imgstream3.com/b/censored/334998_NHDTB-817.jpg"></div></div>
<div class="posts-inner-details-text"><ul><li class="posts-headline">品番:</li><li class="posts-message">NHDTB-817</li></ul></div>
<div class="actor-right-images-part"><img src="https://example.com/actor.jpg"><p>Actor</p></div>
<div class="ut1_img_content_js"><a href="https://example.com/ad"><img src="https://example.com/ad.jpg"></a></div>
<div class="post video_9s"><figure class="video-preview thumb"><a href="/ja/censored_content/123/OTHER-001.html"><img src="https://example.com/other.jpg"><video data-src="https://example.com/other.mp4"></video></a></figure></div>`
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/html; charset=utf-8"}}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	}))
	info, err := p.GetMovieInfoByURL("https://18av.mm-cg.com/ja/censored_content/202186/NHDTB-817.html")
	require.NoError(t, err)
	require.True(t, info.IsValid())
	assert.Empty(t, info.PreviewImages)
	assert.Equal(t, "https://fchost1.imgstream3.com/b/censored/334998_NHDTB-817.jpg", info.CoverURL)
	assert.Equal(t, "https://gifb.imgstream3.com/hc/9smv_censored_334998_NHDTB-817.mp4", info.PreviewVideoURL)
}
