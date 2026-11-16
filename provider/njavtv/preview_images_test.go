package njavtv

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

func TestDetailDoesNotUseSeekSpritesAsStills(t *testing.T) {
	p := New()
	p.ClonedCollector().WithTransport(previewTransportFunc(func(r *http.Request) (*http.Response, error) {
		body := `<meta property="og:title" content="Test movie"><meta property="og:image" content="https://fourhoi.com/ssis-001/cover-n.jpg">
<h1>SSIS-001 Test movie</h1><div class="text-secondary"><span>品番:</span><span class="font-medium">SSIS-001</span></div>
<script>const playerSettings = {thumbnail: {enabled: true, pic_num: 4481, width: 300, height: 168, col: 6, row: 6, urls: ["https://surrit.com/movie/seek/_0.jpg","https://surrit.com/movie/seek/_1.jpg"]}};</script>
<div class="thumbnail group"><a href="/dm1/ja/other-001"><img class="lozad" data-src="https://fourhoi.com/other-001/cover-t.jpg" src="data:image/png;base64,AA"></a></div>`
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/html"}}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	}))
	info, err := p.GetMovieInfoByURL("https://njavtv.com/dm44/ja/ssis-001")
	require.NoError(t, err)
	require.True(t, info.IsValid())
	assert.Empty(t, info.PreviewImages)
	assert.Equal(t, "https://fourhoi.com/ssis-001/cover-n.jpg", info.CoverURL)
}
