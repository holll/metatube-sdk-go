package javtrailers

import (
	"encoding/json"
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
	_ provider.MovieProvider = (*JavTrailers)(nil)
	_ provider.MovieSearcher = (*JavTrailers)(nil)
)

const (
	Name     = "JAVTRAILERS"
	Priority = 500
)

// 站点提供旧版与新 Beta 两套搜索：新版 /search?q= 由前端调
// /api/events/search（需鉴权，实测 403），无法抓取；
// 旧版路径形式 /search/{keyword} 仍服务端渲染，使用这一套。
const (
	baseURL   = "https://javtrailers.com/ja/"
	searchURL = baseURL + "search/%s"
)

// 详情页 URL 末段是站内 content id（如 "ssis00001"、"1svdvd00907"），
// 与品番之间没有稳定推导关系（前者无前缀、后者带 "1"），
// 故 GetMovieInfoByID 必须走「先搜索、再取详情页」。
type JavTrailers struct {
	*scraper.Scraper
}

func New() *JavTrailers {
	return &JavTrailers{
		scraper.NewDefaultScraper(Name, baseURL, Priority, language.Japanese,
			scraper.WithHeaders(map[string]string{
				"Referer": baseURL,
			}),
		),
	}
}

func (jt *JavTrailers) NormalizeMovieID(id string) string {
	// ID 即 URL 末段的 content id，保持原样。
	return strings.TrimSpace(id)
}

func (jt *JavTrailers) ParseMovieIDFromURL(rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	segs := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(segs) < 2 {
		return "", provider.ErrInvalidURL
	}
	return segs[len(segs)-1], nil // /{lang}/video/{contentID}
}

func (jt *JavTrailers) GetMovieInfoByID(id string) (*model.MovieInfo, error) {
	results, err := jt.SearchMovie(jt.NormalizeMovieKeyword(id))
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, provider.ErrInfoNotFound
	}
	// 搜索按相关度排序，精确匹配通常已在首位；仍显式比对一次，
	// 避免同名系列（如 SSIS-001 与 SSIS-193）串味。
	target := results[0]
	for _, r := range results {
		if strings.EqualFold(r.Number, id) || strings.EqualFold(r.ID, id) {
			target = r
			break
		}
	}
	return jt.GetMovieInfoByURL(target.Homepage)
}

func (jt *JavTrailers) GetMovieInfoByURL(rawURL string) (info *model.MovieInfo, err error) {
	id, err := jt.ParseMovieIDFromURL(rawURL)
	if err != nil {
		return
	}

	info = &model.MovieInfo{
		ID:            id,
		Provider:      jt.Name(),
		Homepage:      rawURL,
		Actors:        []string{},
		PreviewImages: []string{},
		Genres:        []string{},
	}

	// ld+json VideoObject 提供封面/时长/发行日的兜底来源。
	var jtTitle, jtThumb, jtDuration string
	var h1Title string

	c := jt.ClonedCollector()

	c.OnHTML(`script[type="application/ld+json"]`, func(e *colly.HTMLElement) {
		var ld struct {
			Type         string `json:"@type"`
			Name         string `json:"name"`
			ThumbnailURL string `json:"thumbnailUrl"`
			Duration     string `json:"duration"`
		}
		if json.Unmarshal([]byte(e.Text), &ld) != nil {
			return
		}
		if ld.Type != "VideoObject" {
			return
		}
		jtTitle, jtThumb, jtDuration = ld.Name, ld.ThumbnailURL, ld.Duration
	})

	c.OnXML(`//h1`, func(e *colly.XMLElement) {
		if h1Title == "" {
			h1Title = strings.TrimSpace(e.Text)
		}
	})

	// 信息块每行形如：
	//   <p class="mb-1"><span class="font-weight-bold mr-3">标签：</span>值</p>
	// 注意「DVD ID:」用半角冒号，其余标签用全角「：」。
	row := func(label string) string {
		return fmt.Sprintf(`//p[@class='mb-1' and span[contains(@class,'font-weight-bold')`+
			` and starts-with(normalize-space(.), '%s')]]`, label)
	}
	// 纯文本行：e.Text 含标签前缀，需去掉。
	textRow := func(label string, dst *string) {
		c.OnXML(row(label), func(e *colly.XMLElement) {
			if *dst == "" {
				*dst = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(e.Text), label))
			}
		})
	}

	textRow("DVD ID:", &info.Number)

	var releaseDate string
	textRow("商品発売日：", &releaseDate)

	var runtime string
	textRow("収録時間：", &runtime)

	textRow("監督：", &info.Director)

	// 链接行：值在 <a> 里。
	c.OnXML(row("メーカー："), func(e *colly.XMLElement) {
		if info.Maker == "" {
			info.Maker = strings.TrimSpace(e.ChildText(`.//a`))
		}
	})

	c.OnXML(row("ジャンル："), func(e *colly.XMLElement) {
		for _, genre := range e.ChildTexts(`.//a`) {
			if genre = strings.TrimSpace(genre); genre != "" {
				info.Genres = append(info.Genres, genre)
			}
		}
	})

	c.OnXML(row("出演者："), func(e *colly.XMLElement) {
		for _, name := range e.ChildTexts(`.//a`) {
			if name = strings.TrimSpace(name); name != "" {
				info.Actors = append(info.Actors, name)
			}
		}
	})

	c.OnScraped(func(_ *colly.Response) {
		// h1 是完整日文标题（形如「SSIS-001 一ヶ月間の… 葵つかさ 乙白さやか」），
		// ld+json 的 name 被截断且带站点后缀，故以 h1 优先。
		info.Title = firstNonEmpty(trimNumberPrefix(h1Title, info.Number), jtTitle)
		if info.Title == "" {
			err = provider.ErrInfoNotFound
			return
		}

		info.CoverURL = jtThumb
		info.BigCoverURL = info.CoverURL
		info.ThumbURL = buildThumbURL(info.CoverURL)
		info.BigThumbURL = info.ThumbURL

		if info.Runtime = parser.ParseRuntime(runtime); info.Runtime == 0 {
			info.Runtime = parser.ParseRuntime(jtDuration)
		}
		info.ReleaseDate = parser.ParseDate(releaseDate)
	})

	err = c.Visit(info.Homepage)
	return
}

func (jt *JavTrailers) NormalizeMovieKeyword(keyword string) string {
	return strings.ToUpper(strings.TrimSpace(keyword))
}

func (jt *JavTrailers) SearchMovie(keyword string) (results []*model.MovieSearchResult, err error) {
	c := jt.ClonedCollector()

	// 结果卡片：
	//   <a href="/ja/video/{contentID}" class="video-link" title="{英文标题}">
	//     <img data-src="…ps.w360.webp" alt="{品番} jav" class="card-img-top video-image">
	//     <span class="badge duration-badge">10:23:00</span>
	//     <p class="card-text title mb-0 vid-title">{品番} {日文标题}</p>
	//     <small class="card-text title text-muted">23 Feb 2022</small>
	//   </a>
	// 注意 a/@title 与卡片文本是英文标题，日文标题只在 p.vid-title。
	c.OnXML(`//a[contains(@class,'video-link')]`, func(e *colly.XMLElement) {
		href := e.Attr("href")
		if href == "" {
			return
		}
		homepage := e.Request.AbsoluteURL(href)
		id, idErr := jt.ParseMovieIDFromURL(homepage)
		if idErr != nil {
			return
		}

		// img/@alt 形如「SVDVD-907 jav」，去掉站点后缀即品番。
		// 注意 img 的 class 是「card-img-top video-image」，只能用 contains 匹配。
		number := strings.TrimSpace(strings.TrimSuffix(
			strings.TrimSpace(e.ChildAttr(`.//img[contains(@class,'card-img-top')]`, "alt")), " jav"))

		title := strings.TrimSpace(e.ChildText(`.//p[contains(@class,'vid-title')]`))
		if title == "" || number == "" {
			return
		}

		thumbURL := e.Request.AbsoluteURL(e.ChildAttr(`.//img[contains(@class,'card-img-top')]`, "data-src"))

		results = append(results, &model.MovieSearchResult{
			ID:          id,
			Number:      number,
			Title:       title,
			Provider:    jt.Name(),
			Homepage:    homepage,
			ThumbURL:    thumbURL,
			CoverURL:    buildCoverURL(thumbURL),
			ReleaseDate: parser.ParseDate(e.ChildText(`.//small[contains(@class,'text-muted')]`)),
		})
	})

	// keyword 是路径段而非查询参数，须用 PathEscape。
	err = c.Visit(fmt.Sprintf(searchURL, url.PathEscape(keyword)))
	return
}

// buildThumbURL 由封面大图推导缩略图：
// https://…/ssis00001/ssis00001pl.w800.webp -> https://…/ssis00001/ssis00001ps.w360.webp
func buildThumbURL(coverURL string) string {
	return strings.Replace(coverURL, "pl.w800.webp", "ps.w360.webp", 1)
}

// buildCoverURL 是 buildThumbURL 的逆向：搜索结果只给缩略图。
func buildCoverURL(thumbURL string) string {
	return strings.Replace(thumbURL, "ps.w360.webp", "pl.w800.webp", 1)
}

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if s != "" {
			return s
		}
	}
	return ""
}

// trimNumberPrefix 去掉标题开头的品番前缀，如
// ("SSIS-001 一ヶ月間の…", "SSIS-001") -> "一ヶ月間の…"。
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
