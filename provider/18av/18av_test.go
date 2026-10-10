package eighteenav

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/metatube-community/metatube-sdk-go/provider"
	"github.com/metatube-community/metatube-sdk-go/provider/internal/testkit"
)

func TestEighteenAV_Registered(t *testing.T) {
	registered := false
	provider.RangeMovieFactory(func(name string, _ provider.MovieFactory) bool {
		if registered = name == Name; registered {
			return false
		}
		return true
	})
	assert.True(t, registered, "provider %q 未注册", Name)
}

func TestEighteenAV_GetMovieInfoByID(t *testing.T) {
	testkit.Test(t, New, []string{
		"NHDTB-817",
		"START-343",
		"ROE-471",
	})
}

func TestEighteenAV_GetMovieInfoByURL(t *testing.T) {
	testkit.Test(t, New, []string{
		"https://18av.mm-cg.com/ja/censored_content/202186/NHDTB-817.html",
		"https://18av.mm-cg.com/ja/amateurjav_content/331742/MAAN-1160.html",
	})
}

func TestEighteenAV_SearchMovie(t *testing.T) {
	testkit.Test(t, New, []string{
		"NHDTB-817",
		"START-343",
	})
}

func TestEighteenAV_ParseMovieIDFromURL(t *testing.T) {
	av := New()
	for _, unit := range []struct {
		url, want string
		wantErr   bool
	}{
		// 常规分类：末段即品番
		{"https://18av.mm-cg.com/ja/censored_content/202186/NHDTB-817.html", "NHDTB-817", false},
		{"https://18av.mm-cg.com/ja/reducing-mosaic_content/229862/START-343.html", "START-343", false},
		{"https://18av.mm-cg.com/ja/uncensored_content/320679/fc2-ppv-4878910.html", "fc2-ppv-4878910", false},
		// dt_content 的末段固定为 "content"，回退到内部数字 ID
		{"https://18av.mm-cg.com/ja/dt_content/91079/content.html", "91079", false},
		{"https://18av.mm-cg.com/ja/", "", true},
	} {
		got, err := av.ParseMovieIDFromURL(unit.url)
		if unit.wantErr {
			assert.Error(t, err, unit.url)
			continue
		}
		require.NoError(t, err, unit.url)
		assert.Equal(t, unit.want, got, unit.url)
	}
}

func TestBuildMediaURLs(t *testing.T) {
	for _, unit := range []struct {
		cover, thumb, preview string
	}{
		{
			"https://fchost1.imgstream2.com/b/censored/334998_NHDTB-817.jpg",
			"https://fchost1.imgstream2.com/s/censored/334998_NHDTB-817.jpg",
			"https://gifb.imgstream2.com/hc/9smv_censored_334998_NHDTB-817.mp4",
		},
		{
			// 图床域名会轮换，须按封面 URL 现场推导
			"https://fbhost1.imgstream3.com/b/amateur/234374_SIRO-5204.jpg",
			"https://fbhost1.imgstream3.com/s/amateur/234374_SIRO-5204.jpg",
			"https://gifb.imgstream3.com/hc/9smv_amateur_234374_SIRO-5204.mp4",
		},
		// 路径不符合预期时留空
		{"https://18av.mm-cg.com/js/player/play.php", "", ""},
		{"", "", ""},
	} {
		thumb, preview := buildMediaURLs(unit.cover)
		assert.Equal(t, unit.thumb, thumb, unit.cover)
		assert.Equal(t, unit.preview, preview, unit.cover)
	}
}

func TestCleanValue(t *testing.T) {
	for _, unit := range []struct{ in, want string }{
		{"  ナチュラルハイ ", "ナチュラルハイ"},
		{"----", ""},
		{"N/A", ""},
		{"n/a", ""},
		{"-", ""},
		{"素人", ""},
		{"", ""},
	} {
		assert.Equal(t, unit.want, cleanValue(unit.in), unit.in)
	}
}

func TestTrimNumberPrefix(t *testing.T) {
	for _, unit := range []struct {
		title, number, want string
	}{
		{"NHDTB-817 ロングスカート女学生痴●", "NHDTB-817", "ロングスカート女学生痴●"},
		// 无空格分隔时不应误伤标题
		{"NHDTB-8179 別のタイトル", "NHDTB-817", "NHDTB-8179 別のタイトル"},
		{"タイトルのみ", "", "タイトルのみ"},
		{"[モザイク破壊]START-343 タイトル", "START-343", "[モザイク破壊]START-343 タイトル"},
	} {
		assert.Equal(t, unit.want, trimNumberPrefix(unit.title, unit.number), unit.title)
	}
}
