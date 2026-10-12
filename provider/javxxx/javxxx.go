package javxxx

import (
	"fmt"
	"net/url"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/gocolly/colly/v2"
	"golang.org/x/text/language"

	"github.com/metatube-community/metatube-sdk-go/common/parser"
	"github.com/metatube-community/metatube-sdk-go/model"
	"github.com/metatube-community/metatube-sdk-go/provider"
	"github.com/metatube-community/metatube-sdk-go/provider/internal/scraper"
)

var (
	_ provider.MovieProvider = (*JavXXX)(nil)
	_ provider.MovieSearcher = (*JavXXX)(nil)
)

const (
	Name     = "JAVXXX"
	Priority = 450
)

const (
	baseURL   = "https://www.javxxx.me/ja/"
	searchURL = "https://www.javxxx.me/search/%s/" // 重定向后不带语言前缀，实测可直接访问
)

type JavXXX struct {
	*scraper.Scraper
}

func New() *JavXXX {
	return &JavXXX{
		scraper.NewDefaultScraper(Name, baseURL, Priority, language.Japanese,
			scraper.WithHeaders(map[string]string{
				"Referer": baseURL,
			}),
		),
	}
}

// NormalizeMovieID 详情页 slug 就是品番的小写形式（ssis-001），统一转大写即品番。
func (jx *JavXXX) NormalizeMovieID(id string) string {
	return strings.ToUpper(strings.TrimSpace(id))
}

func (jx *JavXXX) ParseMovieIDFromURL(rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	segs := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(segs) < 2 {
		return "", provider.ErrInvalidURL
	}
	// 部分番号存在变体条目（如 ssis-001c），此处按 URL 原样返回；
	// 真实品番以详情页 Code: 字段为准，见 GetMovieInfoByURL。
	return jx.NormalizeMovieID(segs[len(segs)-1]), nil
}

func (jx *JavXXX) GetMovieInfoByID(id string) (*model.MovieInfo, error) {
	// 部分番号会 301 到带 /v{n}/ 前缀的规范路径（如 midv-001 → /ja/v3/video/midv-001/）。
	return jx.GetMovieInfoByURL(fmt.Sprintf("%svideo/%s/", baseURL, strings.ToLower(id)))
}

func (jx *JavXXX) GetMovieInfoByURL(rawURL string) (info *model.MovieInfo, err error) {
	id, err := jx.ParseMovieIDFromURL(rawURL)
	if err != nil {
		return
	}

	info = &model.MovieInfo{
		ID:            id,
		Provider:      jx.Name(),
		Homepage:      rawURL,
		Actors:        []string{},
		PreviewImages: []string{},
		Genres:        []string{},
	}

	var h1Title, coverURL string
	var runtime, releaseDate string
	var genreTexts []string

	c := jx.ClonedCollector()

	// 标题：<div id="player-tool"><h1 class="py-2 text-white">…</h1></div>
	c.OnXML(`//div[@id='player-tool']//h1`, func(e *colly.XMLElement) {
		if h1Title == "" {
			h1Title = strings.TrimSpace(e.Text)
		}
	})

	c.OnXML(`//meta[@property='og:image']/@content`, func(e *colly.XMLElement) {
		if coverURL == "" {
			coverURL = strings.TrimSpace(e.Text)
		}
	})

	// 信息块行有两种写法（class 可能在 div 也可能在 span 上），
	// 统一用「后代 span 文本以标签开头」定位，避免依赖 class 位置：
	//   <div class="text-secondary1"><span>Code:</span> SSIS-001</div>
	//   <div><span class="text-secondary1">メーカー:</span> <a>…</a></div>
	row := func(label string) string {
		return fmt.Sprintf(`//div[contains(@class,'movie-description')]`+
			`//div[span[starts-with(normalize-space(.), '%s')]]`, label)
	}
	// 纯文本行：e.Text 形如「Code: SSIS-001」，去掉标签前缀。
	textRow := func(label string, dst *string) {
		c.OnXML(row(label), func(e *colly.XMLElement) {
			if *dst == "" {
				*dst = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(e.Text), label))
			}
		})
	}

	// 番号取 Code: 字段而非 slug——slug 对变体条目会带 "c" 之类后缀。
	textRow("Code:", &info.Number)
	textRow("配信開始日:", &releaseDate)
	textRow("収録時間:", &runtime)

	c.OnXML(row("AV女優:"), func(e *colly.XMLElement) {
		for _, s := range e.ChildTexts(`.//a`) {
			if s = strings.TrimSpace(s); s != "" {
				info.Actors = append(info.Actors, s)
			}
		}
	})

	c.OnXML(row("メーカー:"), func(e *colly.XMLElement) {
		if info.Maker == "" {
			info.Maker = strings.TrimSpace(e.ChildText(`.//a`))
		}
	})

	// 该行的标签写作「レーベル」，但 href 实际混用了 /category/（有修正、英語字幕）
	// 与 /tag/（美少女、美乳、ドラマ…）两类，本质是属性列表而非厂牌，
	// 故整体归入 Genres，不臆造 Label。
	c.OnXML(row("レーベル:"), func(e *colly.XMLElement) {
		genreTexts = append(genreTexts, e.ChildTexts(`.//a`)...)
	})

	c.OnScraped(func(_ *colly.Response) {
		info.Title = trimNumberPrefix(h1Title, info.Number)
		if info.Title == "" {
			err = provider.ErrInfoNotFound
			return
		}

		for _, g := range genreTexts {
			if g = strings.TrimSpace(g); g != "" {
				info.Genres = append(info.Genres, g)
			}
		}

		info.CoverURL = coverURL
		info.BigCoverURL = info.CoverURL
		info.ThumbURL = buildThumbURL(info.CoverURL)
		info.BigThumbURL = info.ThumbURL

		info.Runtime = parser.ParseRuntime(runtime)
		// ⚠️ 站点这个字段填的是「收录到本站的日期」而非影片原始发行日（实测偏差
		// 可达数年），但它是站点自己声明的字段，仍按原值写入，由调用方按优先级取舍。
		info.ReleaseDate = parser.ParseDate(releaseDate)
	})

	err = c.Visit(info.Homepage)
	return
}

func (jx *JavXXX) NormalizeMovieKeyword(keyword string) string {
	return strings.ToUpper(strings.TrimSpace(keyword))
}

func (jx *JavXXX) SearchMovie(keyword string) (results []*model.MovieSearchResult, err error) {
	c := jx.ClonedCollector()

	// 结果卡片：
	//   <div class="… item thumb-block">
	//     <div class="video-img"><a href="…/video/svdvd-907/" title="{英文标题}">
	//       <img class="img-fluid image-full video-with-trailer"
	//            src="https://pics.pornfhd.com/s/digital/video/1svdvd00907/1svdvd00907pl.jpg"
	//            alt="{英文标题}" data-mediabook="…preview.png">
	//       <span class="release">2025-11-09</span>
	//       <span class="duration"><i class="fas fa-clock"></i> 10:23:00</span>
	//     </a></div>
	//     <h3 class="video-desc mr-3"><a href="…/video/svdvd-907/" title="{英文标题}">…</a></h3>
	//   </div>
	// 注意：本站搜索结果的标题只有英文，日文标题需进详情页取。
	c.OnXML(`//div[contains(@class,'item') and contains(@class,'thumb-block')]`, func(e *colly.XMLElement) {
		href := e.ChildAttr(`.//div[contains(@class,'video-img')]//a`, "href")
		if href == "" {
			return
		}
		homepage := e.Request.AbsoluteURL(href)
		id, idErr := jx.ParseMovieIDFromURL(homepage)
		if idErr != nil || id == "" {
			return
		}

		title := strings.TrimSpace(e.ChildAttr(`.//div[contains(@class,'video-img')]//a`, "title"))
		if title == "" {
			title = strings.TrimSpace(e.ChildText(`.//h3[contains(@class,'video-desc')]//a`))
		}
		if title == "" {
			return
		}

		thumbURL := e.Request.AbsoluteURL(e.ChildAttr(`.//div[contains(@class,'video-img')]//img`, "src"))

		results = append(results, &model.MovieSearchResult{
			ID:       id,
			Number:   id, // slug 即品番，见 ParseMovieIDFromURL
			Title:    title,
			Provider: jx.Name(),
			Homepage: homepage,
			ThumbURL: thumbURL,
			CoverURL: buildCoverURL(thumbURL),
			// span.release 的值同样是收录日，不准确。
			ReleaseDate: parser.ParseDate(e.ChildText(`.//span[contains(@class,'release')]`)),
		})
	})

	// keyword 是路径段而非查询参数，须用 PathEscape。
	err = c.Visit(fmt.Sprintf(searchURL, url.PathEscape(keyword)))
	return
}

// buildThumbURL 由封面推导缩略图：
// https://pics.pornfhd.com/digital/video/ssis00001/ssis00001pl.jpg
//
//	-> https://pics.pornfhd.com/s/digital/video/ssis00001/ssis00001pl.jpg
func buildThumbURL(coverURL string) string {
	return strings.Replace(coverURL, "/digital/video/", "/s/digital/video/", 1)
}

// buildCoverURL 是 buildThumbURL 的逆向：搜索结果只给缩略图。
func buildCoverURL(thumbURL string) string {
	return strings.Replace(thumbURL, "/s/digital/video/", "/digital/video/", 1)
}

// trimNumberPrefix 去掉标题开头的品番前缀，如
// ("SSIS-001 [英語字幕] …", "SSIS-001") -> "[英語字幕] …"。
// 仅当品番之后紧跟非字母数字字符时才视为前缀，否则 "SSIS-0019" 会被 "SSIS-001" 误截。
func trimNumberPrefix(title, number string) string {
	if number == "" || len(title) < len(number) ||
		!strings.EqualFold(title[:len(number)], number) {
		return title
	}
	rest := title[len(number):]
	if r, _ := utf8.DecodeRuneInString(rest); unicode.IsLetter(r) || unicode.IsDigit(r) {
		return title
	}
	return strings.TrimSpace(rest)
}

func init() {
	provider.Register(Name, New)
}
