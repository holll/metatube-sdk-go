package fanza

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/metatube-community/metatube-sdk-go/provider/fanza/internal/searchparse"
)

func TestMovieSearchKeywords(t *testing.T) {
	for _, tt := range []struct {
		keyword string
		want    []string
	}{
		{"SSIS-001", []string{"ssis00001#", "ssis001", "ssis1"}},
		{"ssis_001", []string{"ssis00001#", "ssis001", "ssis1"}},
		{"ssis001", []string{"ssis001", "ssis00001#", "ssis1"}},
		{"ssis1", []string{"ssis1", "ssis00001#"}},
		{"ssis00001", []string{"ssis00001", "ssis00001#", "ssis1"}},
		{"ssis-123456", []string{"ssis123456#", "ssis123456"}},
		{"h_068mxgs1184bod", []string{"h_068mxgs1184bod"}},
		{"作品名", []string{"作品名"}},
	} {
		t.Run(tt.keyword, func(t *testing.T) {
			assert.Equal(t, tt.want, movieSearchKeywords(tt.keyword))
		})
	}
}

func TestSearchMovieVariants(t *testing.T) {
	for _, tt := range []struct {
		name    string
		keyword string
		pages   [][]string
		queries []string
		wantIDs []string
	}{
		{"exact-first", "SSIS-001", [][]string{{"ssis00001"}}, []string{"ssis00001#"}, []string{"ssis00001"}},
		{"ignore-near-match", "SSIS-001", [][]string{{"ssis00002"}, {"ssis00001", "ssis00003"}}, []string{"ssis00001#", "ssis001"}, []string{"ssis00001"}},
		{"third-variant", "SSIS-001", [][]string{nil, nil, {"ssis00001"}}, []string{"ssis00001#", "ssis001", "ssis1"}, []string{"ssis00001"}},
		{"bounded-miss", "SSIS-001", [][]string{{"ssis00002"}, nil, {"ssis00003"}}, []string{"ssis00001#", "ssis001", "ssis1"}, nil},
		{"compact-original-first", "ssis001", [][]string{{"ssis00001"}}, []string{"ssis001"}, []string{"ssis00001"}},
		{"title-search-unchanged", "作品名", [][]string{{"ssis00001", "ssis00002"}}, []string{"作品名"}, []string{"ssis00001", "ssis00002"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			p := New()
			var queries []string
			p.ClonedCollector().WithTransport(roundTripFunc(func(r *http.Request) (*http.Response, error) {
				parts := strings.SplitN(r.URL.Path, "searchstr=", 2)
				require.Len(t, parts, 2)
				queries = append(queries, strings.SplitN(parts[1], "/", 2)[0])
				index := len(queries) - 1
				require.Less(t, index, len(tt.pages))
				var resp searchparse.ResponseWrapper
				for _, id := range tt.pages[index] {
					resp.BackendResponse.Contents.Data = append(resp.BackendResponse.Contents.Data, searchparse.Product{
						ContentID: id, Title: "title", DetailURL: "https://video.dmm.co.jp/av/content/?id=" + id,
					})
				}
				data, err := json.Marshal([]any{resp})
				require.NoError(t, err)
				line, err := json.Marshal("0:" + string(data) + "\n")
				require.NoError(t, err)
				body := `<html><script>self.__next_f = [[0], [1,` + string(line) + `]];</script></html>`
				return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/html; charset=utf-8"}}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
			}))
			results, err := p.SearchMovie(tt.keyword)
			require.NoError(t, err)
			assert.Equal(t, tt.queries, queries)
			var ids []string
			for _, r := range results {
				ids = append(ids, r.ID)
			}
			assert.ElementsMatch(t, tt.wantIDs, ids)
		})
	}
}
