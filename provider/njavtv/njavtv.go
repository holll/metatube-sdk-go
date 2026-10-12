package njavtv

import (
	"fmt"
	"net/url"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/gocolly/colly/v2"
	"github.com/lib/pq"
	"golang.org/x/text/language"

	"github.com/metatube-community/metatube-sdk-go/common/parser"
	"github.com/metatube-community/metatube-sdk-go/model"
	"github.com/metatube-community/metatube-sdk-go/provider"
	"github.com/metatube-community/metatube-sdk-go/provider/internal/scraper"
)

var (
	_ provider.MovieProvider = (*NjavTV)(nil)
	_ provider.MovieSearcher = (*NjavTV)(nil)
)

const (
	Name     = "NJAVTV"
	Priority = 500
)

// 详情页规范路径形如 /dm{n}/{lang}/{slug}，其中 dm{n} 按影片固定但无法由品番推导；
// 短链 /{lang}/{slug} 会 301 到规范路径，colly 默认跟随重定向，故无需先搜索。
const (
	baseURL   = "https://njavtv.com/ja/"
	searchURL = baseURL + "search/%s"
)

type NjavTV struct {
	*scraper.Scraper
}

func New() *NjavTV {
	return &NjavTV{
		scraper.NewDefaultScraper(Name, baseURL, Priority, language.Japanese,
			scraper.WithHeaders(map[string]string{
				"Referer": baseURL,
			}),
		),
	}
}

// NormalizeMovieID 站内 slug 就是品番的小写形式（ssis-001），统一转大写即品番。
func (nt *NjavTV) NormalizeMovieID(id string) string {
	return strings.ToUpper(strings.TrimSpace(id))
}

func (nt *NjavTV) ParseMovieIDFromURL(rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	segs := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(segs) < 2 {
		return "", provider.ErrInvalidURL
	}
	return nt.NormalizeMovieID(segs[len(segs)-1]), nil // …/{lang}/{slug}
}

func (nt *NjavTV) GetMovieInfoByID(id string) (*model.MovieInfo, error) {
	// 短链会被 301 到带 dm{n} 前缀的规范路径。
	return nt.GetMovieInfoByURL(fmt.Sprintf("%s%s", baseURL, strings.ToLower(id)))
}

func (nt *NjavTV) GetMovieInfoByURL(rawURL string) (info *model.MovieInfo, err error) {
	id, err := nt.ParseMovieIDFromURL(rawURL)
	if err != nil {
		return
	}

	info = &model.MovieInfo{
		ID:            id,
		Provider:      nt.Name(),
		Homepage:      rawURL,
		Actors:        []string{},
		PreviewImages: []string{},
		Genres:        []string{},
	}

	// og: 系列 meta 提供语言无关的兜底值。
	var ogTitle, ogImage, ogReleaseDate, ogDirector string
	var ogDuration int
	var ogActors pq.StringArray

	var h1Title, releaseDate string

	c := nt.ClonedCollector()

	meta := func(property string) string {
		return fmt.Sprintf(`//meta[@property='%s']/@content`, property)
	}

	c.OnXML(meta("og:title"), func(e *colly.XMLElement) {
		if ogTitle == "" {
			ogTitle = strings.TrimSpace(e.Text)
		}
	})

	c.OnXML(meta("og:image"), func(e *colly.XMLElement) {
		if ogImage == "" {
			ogImage = strings.TrimSpace(e.Text)
		}
	})

	c.OnXML(meta("og:video:release_date"), func(e *colly.XMLElement) {
		if ogReleaseDate == "" {
			ogReleaseDate = strings.TrimSpace(e.Text)
		}
	})

	// 时长为秒数，如 8821。
	c.OnXML(meta("og:video:duration"), func(e *colly.XMLElement) {
		if ogDuration == 0 {
			ogDuration = parser.ParseInt(e.Text)
		}
	})

	c.OnXML(meta("og:video:director"), func(e *colly.XMLElement) {
		if ogDirector == "" {
			ogDirector = strings.TrimSpace(e.Text)
		}
	})

	c.OnXML(meta("og:video:actor"), func(e *colly.XMLElement) {
		if name := strings.TrimSpace(e.Text); name != "" {
			ogActors = append(ogActors, name)
		}
	})

	c.OnXML(`//h1`, func(e *colly.XMLElement) {
		if h1Title == "" {
			h1Title = strings.TrimSpace(e.Text)
		}
	})

	// 信息块每行形如：
	//   <div class="text-secondary"><span>标签:</span> 值</div>
	row := func(label string) string {
		return fmt.Sprintf(`//div[contains(@class,'text-secondary')`+
			` and span[starts-with(normalize-space(.), '%s')]]`, label)
	}
	links := func(label string, dst *pq.StringArray) {
		c.OnXML(row(label), func(e *colly.XMLElement) {
			for _, s := range e.ChildTexts(`.//a`) {
				if s = strings.TrimSpace(s); s != "" {
					*dst = append(*dst, s)
				}
			}
		})
	}

	c.OnXML(row("品番"), func(e *colly.XMLElement) {
		if info.Number == "" {
			info.Number = strings.TrimSpace(e.ChildText(`.//span[contains(@class,'font-medium')]`))
		}
	})

	// 配信開始日 行内是 <time datetime="2021-02-18T11:00:54+08:00">2021-02-18</time>。
	// 取元素的显示文本（纯日期）而非 datetime 属性：后者带时区，
	// 换算 UTC 时可能跨日（如 +08:00 的凌晨会退到前一天）。
	c.OnXML(row("配信開始日"), func(e *colly.XMLElement) {
		if releaseDate == "" {
			releaseDate = strings.TrimSpace(e.ChildText(`.//time`))
		}
	})

	// 女優 行只含女演员；og:video:actor 会把男優（如「平田司」）一并混入，故以该行为准。
	links("女優", &info.Actors)
	links("ジャンル", &info.Genres)

	c.OnXML(row("メーカー"), func(e *colly.XMLElement) {
		if info.Maker == "" {
			info.Maker = strings.TrimSpace(e.ChildText(`.//a`))
		}
	})

	c.OnXML(row("レーベル"), func(e *colly.XMLElement) {
		if info.Label == "" {
			info.Label = strings.TrimSpace(e.ChildText(`.//a`))
		}
	})

	c.OnXML(row("シリーズ"), func(e *colly.XMLElement) {
		if info.Series == "" {
			info.Series = strings.TrimSpace(e.ChildText(`.//a`))
		}
	})

	c.OnXML(row("監督"), func(e *colly.XMLElement) {
		if info.Director == "" {
			info.Director = strings.TrimSpace(e.ChildText(`.//a`))
		}
	})

	c.OnScraped(func(_ *colly.Response) {
		info.Title = firstNonEmpty(trimNumberPrefix(h1Title, info.Number), ogTitle)
		if info.Title == "" {
			err = provider.ErrInfoNotFound
			return
		}

		if len(info.Actors) == 0 {
			info.Actors = ogActors
		}
		if info.Director == "" {
			info.Director = ogDirector
		}

		info.CoverURL = ogImage
		info.BigCoverURL = info.CoverURL
		info.ThumbURL = buildThumbURL(info.CoverURL)
		info.BigThumbURL = info.ThumbURL

		// og:video:duration 是秒。
		if info.Runtime == 0 {
			info.Runtime = ogDuration / 60
		}
		info.ReleaseDate = parser.ParseDate(firstNonEmpty(releaseDate, ogReleaseDate))
	})

	err = c.Visit(info.Homepage)
	return
}

func (nt *NjavTV) NormalizeMovieKeyword(keyword string) string {
	return strings.ToUpper(strings.TrimSpace(keyword))
}

func (nt *NjavTV) SearchMovie(keyword string) (results []*model.MovieSearchResult, err error) {
	c := nt.ClonedCollector()

	// 结果卡片：
	//   <div class="thumbnail group">
	//     <a href="…/dm26/ja/svdvd-907" alt="svdvd-907">
	//       <img class="lozad w-full" data-src="https://fourhoi.com/svdvd-907/cover-t.jpg"
	//            src="data:image/png;base64,…" alt="{无番号的日文标题}">
	//     </a>
	//     <a href="…/dm26/ja/svdvd-907" alt="svdvd-907">
	//       <span class="…bottom-1 right-1…">10:22:35</span>
	//     </a>
	//     <div class="…truncate"><a href="…" alt="svdvd-907">SVDVD-907 {日文标题}</a></div>
	//   </div>
	// 注意 img/@src 是 base64 占位图，真实地址在 data-src。
	c.OnXML(`//div[contains(@class,'thumbnail')][.//a[@alt]]`, func(e *colly.XMLElement) {
		href := e.ChildAttr(`.//a[@alt]`, "href")
		if href == "" {
			return
		}
		homepage := e.Request.AbsoluteURL(href)
		id, idErr := nt.ParseMovieIDFromURL(homepage)
		if idErr != nil || id == "" {
			return
		}

		// a/@alt 就是品番的小写形式。
		number := strings.ToUpper(strings.TrimSpace(e.ChildAttr(`.//a[@alt]`, "alt")))
		title := strings.TrimSpace(e.ChildText(`.//div[contains(@class,'truncate')]//a`))
		if number == "" || title == "" {
			return
		}

		thumbURL := e.Request.AbsoluteURL(e.ChildAttr(`.//img`, "data-src"))

		results = append(results, &model.MovieSearchResult{
			ID:       id,
			Number:   number,
			Title:    title,
			Provider: nt.Name(),
			Homepage: homepage,
			ThumbURL: thumbURL,
			CoverURL: buildCoverURL(thumbURL),
		})
	})

	// keyword 是路径段而非查询参数，须用 PathEscape。
	err = c.Visit(fmt.Sprintf(searchURL, url.PathEscape(keyword)))
	return
}

// buildThumbURL 由封面推导缩略图：
// https://fourhoi.com/ssis-001/cover-n.jpg -> https://fourhoi.com/ssis-001/cover-t.jpg
func buildThumbURL(coverURL string) string {
	return strings.Replace(coverURL, "cover-n.jpg", "cover-t.jpg", 1)
}

// buildCoverURL 是 buildThumbURL 的逆向：搜索结果只给缩略图。
func buildCoverURL(thumbURL string) string {
	return strings.Replace(thumbURL, "cover-t.jpg", "cover-n.jpg", 1)
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
