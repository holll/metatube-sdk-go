package javtrailers

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/metatube-community/metatube-sdk-go/provider"
	"github.com/metatube-community/metatube-sdk-go/provider/internal/testkit"
)

func TestJavTrailers_Registered(t *testing.T) {
	registered := false
	provider.RangeMovieFactory(func(name string, _ provider.MovieFactory) bool {
		if registered = name == Name; registered {
			return false
		}
		return true
	})
	assert.True(t, registered, "provider %q 未注册", Name)
}

func TestJavTrailers_GetMovieInfoByID(t *testing.T) {
	testkit.Test(t, New, []string{
		"SSIS-001",  // 有演员、多类型
		"SVDVD-907", // content id 带 "1" 前缀
		"NHDTB-817",
	})
}

func TestJavTrailers_GetMovieInfoByURL(t *testing.T) {
	testkit.Test(t, New, []string{
		"https://javtrailers.com/ja/video/ssis00001",
		"https://javtrailers.com/ja/video/1svdvd00907",
	})
}

func TestJavTrailers_SearchMovie(t *testing.T) {
	testkit.Test(t, New, []string{
		"SSIS-001",
		"SVDVD-907",
	})
}

func TestJavTrailers_ParseMovieIDFromURL(t *testing.T) {
	jt := New()
	for _, unit := range []struct {
		url, want string
		wantErr   bool
	}{
		{"https://javtrailers.com/ja/video/ssis00001", "ssis00001", false},
		{"https://javtrailers.com/ja/video/1svdvd00907", "1svdvd00907", false},
		{"https://javtrailers.com/ja/", "", true},
	} {
		got, err := jt.ParseMovieIDFromURL(unit.url)
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
		thumb = "https://images.javtrailers.com/digital/video/ssis00001/ssis00001ps.w360.webp"
		cover = "https://images.javtrailers.com/digital/video/ssis00001/ssis00001pl.w800.webp"
	)
	assert.Equal(t, thumb, buildThumbURL(cover))
	assert.Equal(t, cover, buildCoverURL(thumb))
	// 路径不符预期时原样返回
	assert.Equal(t, "", buildThumbURL(""))
}

func TestTrimNumberPrefix(t *testing.T) {
	for _, unit := range []struct {
		title, number, want string
	}{
		{"SSIS-001 一ヶ月間の禁欲の果てに…", "SSIS-001", "一ヶ月間の禁欲の果てに…"},
		// 无分隔符时不应误伤
		{"SSIS-0019 別のタイトル", "SSIS-001", "SSIS-0019 別のタイトル"},
		{"タイトルのみ", "", "タイトルのみ"},
	} {
		assert.Equal(t, unit.want, trimNumberPrefix(unit.title, unit.number), unit.title)
	}
}
