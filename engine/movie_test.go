package engine

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/metatube-community/metatube-sdk-go/model"
	"github.com/metatube-community/metatube-sdk-go/provider/fanza"
	"github.com/metatube-community/metatube-sdk-go/provider/javbus"
)

func TestSortMovieSearchResults(t *testing.T) {
	e := New(nil)
	e.MustGetMovieProviderByName(fanza.Name).SetPriority(100000)
	e.MustGetMovieProviderByName(javbus.Name).SetPriority(1)
	result := func(id, num, source string) *model.MovieSearchResult {
		return &model.MovieSearchResult{ID: id, Number: num, Provider: source, Title: "title", Homepage: "https://example.com/" + id}
	}
	results := []*model.MovieSearchResult{
		result("fuzzy", "SSIS-002", fanza.Name),
		result("equivalent-high", "ssis00001", fanza.Name),
		result("equivalent-low", "SSIS_001", javbus.Name),
		result("exact-low", "SSIS-001", javbus.Name),
		result("exact-high", "ssis-001", fanza.Name),
		result("invalid", "", fanza.Name),
		result("unknown", "SSIS-001", "unknown"),
	}
	results = e.sortMovieSearchResults("SSIS-001", results)
	var ids []string
	for _, r := range results {
		ids = append(ids, r.ID)
	}
	assert.Equal(t, []string{"exact-high", "exact-low", "equivalent-high", "equivalent-low", "fuzzy"}, ids)
}

func TestSortPaddedMovieNumbers(t *testing.T) {
	for _, tc := range []struct{ query, number, fuzzy string }{
		{"N-960", "n0960", "MAAN-960"},
		{"HEYZO-702", "HEYZO-0702", "HODV-21609"},
		{"HEYZO-679", "HEYZO-0679", "HODV-21609"},
	} {
		e := New(nil)
		e.MustGetMovieProviderByName(fanza.Name).SetPriority(100000)
		e.MustGetMovieProviderByName(javbus.Name).SetPriority(1)
		results := e.sortMovieSearchResults(tc.query, []*model.MovieSearchResult{
			{ID: "fuzzy", Number: tc.fuzzy, Title: "title", Provider: fanza.Name, Homepage: "https://example.com/fuzzy"},
			{ID: "equivalent", Number: tc.number, Title: "title", Provider: javbus.Name, Homepage: "https://example.com/equivalent"},
		})
		if len(results) != 2 || results[0].ID != "equivalent" {
			t.Fatal(tc.query, results)
		}
	}
}

func TestSortMovieSearchResultsStable(t *testing.T) {
	e := New(nil)
	results := []*model.MovieSearchResult{
		{ID: "first", Number: "ABC-001", Title: "title", Provider: fanza.Name, Homepage: "https://example.com/first"},
		{ID: "second", Number: "ABC-001", Title: "title", Provider: fanza.Name, Homepage: "https://example.com/second"},
	}
	results = e.sortMovieSearchResults("ABC-001", results)
	assert.Equal(t, "first", results[0].ID)
	assert.Equal(t, "second", results[1].ID)
}
