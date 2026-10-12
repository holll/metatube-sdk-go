package njavtv

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/metatube-community/metatube-sdk-go/provider"
	"github.com/metatube-community/metatube-sdk-go/provider/internal/testkit"
)

func TestNjavTV_Registered(t *testing.T) {
	registered := false
	provider.RangeMovieFactory(func(name string, _ provider.MovieFactory) bool {
		if registered = name == Name; registered {
			return false
		}
		return true
	})
	assert.True(t, registered, "provider %q 未注册", Name)
}

func TestNjavTV_GetMovieInfoByID(t *testing.T) {
	testkit.Test(t, New, []string{
		"SSIS-001",  // 有女优、男优、监督、系列
		"SVDVD-907", // 常规
		"NHDTB-817",
	})
}

func TestNjavTV_GetMovieInfoByURL(t *testing.T) {
	testkit.Test(t, New, []string{
		// 短链（会 301 到带 dm{n} 前缀的规范路径）
		"https://njavtv.com/ja/ssis-001",
		// 规范路径
		"https://njavtv.com/dm26/ja/svdvd-907",
	})
}

func TestNjavTV_SearchMovie(t *testing.T) {
	testkit.Test(t, New, []string{
		"SSIS-001",
		"SVDVD-907",
	})
}

func TestNjavTV_ParseMovieIDFromURL(t *testing.T) {
	nt := New()
	for _, unit := range []struct {
		url, want string
		wantErr   bool
	}{
		{"https://njavtv.com/dm26/ja/svdvd-907", "SVDVD-907", false},
		{"https://njavtv.com/ja/ssis-001", "SSIS-001", false},
		{"https://njavtv.com/ja/", "", true},
	} {
		got, err := nt.ParseMovieIDFromURL(unit.url)
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
		thumb = "https://fourhoi.com/ssis-001/cover-t.jpg"
		cover = "https://fourhoi.com/ssis-001/cover-n.jpg"
	)
	assert.Equal(t, thumb, buildThumbURL(cover))
	assert.Equal(t, cover, buildCoverURL(thumb))
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
