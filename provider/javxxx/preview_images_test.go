package javxxx

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

func TestDetailWithoutScenesDoesNotCollectRecommendations(t *testing.T) {
	p := New()
	p.ClonedCollector().WithTransport(previewTransportFunc(func(r *http.Request) (*http.Response, error) {
		body := `<meta property="og:image" content="https://pics.pornfhd.com/digital/video/ssis00001/ssis00001pl.jpg">
<div id="player-wrapper" data-ts-id="45150"></div><div id="scenes" class="text-center"></div>
<div id="player-tool"><h1>SSIS-001 Test movie</h1></div><div class="movie-description"><div><span>Code:</span> SSIS-001</div></div>
<div class="item thumb-block"><img class="video-with-trailer" src="https://example.com/other-cover.jpg" data-mediabook="https://video.pornfhd.com/p/other/preview.png" data-lazy-src="https://example.com/other.jpg"></div>`
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/html"}}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	}))
	info, err := p.GetMovieInfoByURL("https://www.javxxx.me/ja/v1/video/ssis-001/")
	require.NoError(t, err)
	require.True(t, info.IsValid())
	assert.Empty(t, info.PreviewImages)
	assert.Equal(t, "https://pics.pornfhd.com/digital/video/ssis00001/ssis00001pl.jpg", info.CoverURL)
}
