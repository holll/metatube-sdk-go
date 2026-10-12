package javxxx

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/metatube-community/metatube-sdk-go/provider"
	"github.com/metatube-community/metatube-sdk-go/provider/internal/testkit"
)

func TestJavXXX_Registered(t *testing.T) {
	registered := false
	provider.RangeMovieFactory(func(name string, _ provider.MovieFactory) bool {
		if registered = name == Name; registered {
			return false
		}
		return true
	})
	assert.True(t, registered, "provider %q 未注册", Name)
}

func TestJavXXX_GetMovieInfoByID(t *testing.T) {
	testkit.Test(t, New, []string{
		"SSIS-001", // 有演员、多类型
		"SVDVD-907",
		"MIDV-001", // 番号直出会 301 到带 /v{n}/ 前缀的规范路径
	})
}

func TestJavXXX_GetMovieInfoByURL(t *testing.T) {
	testkit.Test(t, New, []string{
		"https://www.javxxx.me/ja/video/ssis-001/",
		"https://www.javxxx.me/ja/v3/video/midv-001/", // 带 /v{n}/ 前缀的规范路径
	})
}

func TestJavXXX_SearchMovie(t *testing.T) {
	testkit.Test(t, New, []string{
		"SSIS-001",
		"SVDVD-907",
	})
}

func TestJavXXX_ParseMovieIDFromURL(t *testing.T) {
	jx := New()
	for _, unit := range []struct {
		url, want string
		wantErr   bool
	}{
		{"https://www.javxxx.me/ja/video/ssis-001/", "SSIS-001", false},
		{"https://www.javxxx.me/ja/v3/video/midv-001/", "MIDV-001", false},
		{"https://www.javxxx.me/ja/", "", true},
	} {
		got, err := jx.ParseMovieIDFromURL(unit.url)
		if unit.wantErr {
			assert.Error(t, err, unit.url)
			continue
		}
		require.NoError(t, err, unit.url)
		assert.Equal(t, unit.want, got, unit.url)
	}
}

func TestBuildMediaURLs(t *testing.T) {
	const (
		thumb = "https://pics.pornfhd.com/s/digital/video/ssis00001/ssis00001pl.jpg"
		cover = "https://pics.pornfhd.com/digital/video/ssis00001/ssis00001pl.jpg"
	)
	assert.Equal(t, thumb, buildThumbURL(cover))
	assert.Equal(t, cover, buildCoverURL(thumb))
	// 路径不符预期时原样返回
	assert.Equal(t, "https://example.com/a.jpg", buildThumbURL("https://example.com/a.jpg"))
}

func TestTrimNumberPrefix(t *testing.T) {
	for _, unit := range []struct {
		title, number, want string
	}{
		{"SSIS-001 [英語字幕] 一ヶ月間の…", "SSIS-001", "[英語字幕] 一ヶ月間の…"},
		// 无分隔符时不应误伤
		{"SSIS-0019 別のタイトル", "SSIS-001", "SSIS-0019 別のタイトル"},
		{"タイトルのみ", "", "タイトルのみ"},
	} {
		assert.Equal(t, unit.want, trimNumberPrefix(unit.title, unit.number), unit.title)
	}
}
