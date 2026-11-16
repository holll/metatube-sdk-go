package ggjav

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/gocolly/colly/v2"
	"golang.org/x/text/language"

	"github.com/metatube-community/metatube-sdk-go/common/parser"
	"github.com/metatube-community/metatube-sdk-go/model"
	"github.com/metatube-community/metatube-sdk-go/provider"
	"github.com/metatube-community/metatube-sdk-go/provider/internal/previewimage"
	"github.com/metatube-community/metatube-sdk-go/provider/internal/scraper"
)

var (
	_ provider.MovieProvider = (*GGJAV)(nil)
	_ provider.MovieSearcher = (*GGJAV)(nil)
)

// numberRe matches a leading number token of a search result title,
// e.g. "NHDTB-817" in "NHDTB-817 長裙女學生痴漢…".
var numberRe = regexp.MustCompile(`^(?i)[A-Z0-9_-]+$`)

const (
	Name     = "GGJAV"
	Priority = 500
)

const (
	baseURL   = "https://ggjav.com/ja/"
	movieURL  = "https://ggjav.com/ja/main/video?id=%s"
	searchURL = "https://ggjav.com/ja/main/search?string=%s"
)

type GGJAV struct {
	*scraper.Scraper
}

func New() *GGJAV {
	return &GGJAV{
		scraper.NewDefaultScraper(Name, baseURL, Priority, language.Japanese,
			scraper.WithHeaders(map[string]string{
				"Referer": baseURL,
			}),
		),
	}
}

func (gg *GGJAV) NormalizeMovieID(id string) string {
	// ID is a numeric internal ID, e.g. "301794"
	return strings.TrimSpace(id)
}

func (gg *GGJAV) ParseMovieIDFromURL(rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	id := u.Query().Get("id")
	if id == "" {
		return "", provider.ErrInvalidURL
	}
	return id, nil
}

func (gg *GGJAV) GetMovieInfoByID(id string) (*model.MovieInfo, error) {
	return gg.GetMovieInfoByURL(fmt.Sprintf(movieURL, id))
}

func (gg *GGJAV) GetMovieInfoByURL(rawURL string) (info *model.MovieInfo, err error) {
	id, err := gg.ParseMovieIDFromURL(rawURL)
	if err != nil {
		return
	}

	info = &model.MovieInfo{
		ID:            id,
		Provider:      gg.Name(),
		Homepage:      rawURL,
		Actors:        []string{},
		PreviewImages: []string{},
		Genres:        []string{},
	}

	c := gg.ClonedCollector()

	// The current template uses h2; older pages used div. Keep the first
	// title, not later section headings such as the previews heading.
	c.OnHTML(`div.title_text, h2.title_text`, func(e *colly.HTMLElement) {
		if info.Title == "" {
			title := strings.TrimSpace(e.Text)
			if title != "" {
				info.Title = title
			}
		}
	})

	// Cover image: img inside the info section (first large image)
	c.OnXML(`//div[contains(@class,"info")]//div[contains(@class,"large-6")]//img`, func(e *colly.XMLElement) {
		if info.CoverURL == "" {
			src := e.Attr("src")
			if src != "" && !strings.Contains(src, "no_image") {
				info.CoverURL = e.Request.AbsoluteURL(src)
			}
		}
	})

	// Current template separates the label and value into sibling elements.
	c.OnHTML(`div.video_number`, func(e *colly.HTMLElement) {
		if number := strings.TrimSpace(e.Text); number != "" && info.Number == "" {
			info.Number = number
		}
	})

	// Number: div containing "番号："(日語) / "番號："(繁中)。
	// 两种写法都兼容，避免站点切换语言版本时标签变字导致 Number 解析为空，
	// 进而使 MovieInfo.IsValid() 失败、详情返回 500。
	c.OnXML(`//div[starts-with(normalize-space(text()),"番号：") or starts-with(normalize-space(text()),"番號：")]`,
		func(e *colly.XMLElement) {
			text := strings.TrimSpace(e.Text)
			for _, label := range []string{"番号：", "番號："} {
				if after, ok := strings.CutPrefix(text, label); ok {
					if number := strings.TrimSpace(after); number != "" && info.Number == "" {
						info.Number = number
					}
					return
				}
			}
		})

	// Genres: links with class ctg_button
	c.OnXML(`//a[contains(@class,"ctg_button")]`, func(e *colly.XMLElement) {
		genre := strings.TrimSpace(e.Text)
		if genre != "" {
			info.Genres = append(info.Genres, genre)
		}
	})

	// Actors: model_name divs inside model divs
	c.OnXML(`//div[@class="model"]//div[@class="model_name"]`, func(e *colly.XMLElement) {
		name := strings.TrimSpace(e.Text)
		if name != "" {
			info.Actors = append(info.Actors, name)
		}
	})

	// Only accept explicitly listed stills belonging to this movie. Match the
	// class token (not e.g. "other-previews") and the observed {id}_{n} path.
	previewRe := regexp.MustCompile(`^/media/preview/` + regexp.QuoteMeta(id) + `_[0-9]+\.(?i:jpg|jpeg|png|webp|avif)$`)
	c.OnHTML(`div.previews img`, func(e *colly.HTMLElement) {
		// Prefer the real lazy-loaded image to a placeholder or low-res src.
		for _, attr := range []string{"data-src", "data-original", "data-lazy-src", "src"} {
			images := previewimage.Append(nil, e.Attr(attr), e.Request.URL.String())
			if len(images) == 0 {
				continue
			}
			u, parseErr := url.Parse(images[0])
			if parseErr != nil || !previewRe.MatchString(u.Path) {
				continue
			}
			info.PreviewImages = previewimage.Append(info.PreviewImages, images[0], e.Request.URL.String())
			break
		}
	})

	// Score from like/dislike ratio (computed in OnScraped)
	var likeCount, dislikeCount int
	c.OnXML(`//script[contains(text(),"var like_time")]`, func(e *colly.XMLElement) {
		likeRe := regexp.MustCompile(`var like_time\s*=\s*(\d+)`)
		dislikeRe := regexp.MustCompile(`var dislike_time\s*=\s*(\d+)`)
		if sub := likeRe.FindStringSubmatch(e.Text); len(sub) == 2 {
			likeCount = parser.ParseInt(sub[1])
		}
		if sub := dislikeRe.FindStringSubmatch(e.Text); len(sub) == 2 {
			dislikeCount = parser.ParseInt(sub[1])
		}
	})

	c.OnScraped(func(_ *colly.Response) {
		if info.CoverURL == "" {
			// Fallback: construct cover URL from ID pattern
			info.CoverURL = fmt.Sprintf("https://cdn-1.ggjav.com/media/video/large_%s.jpg", id)
		}
		if info.Title == "" {
			err = provider.ErrInfoNotFound
			return
		}
		// Compute score from like/dislike
		total := likeCount + dislikeCount
		if total > 0 {
			info.Score = parser.ParseScore(fmt.Sprintf("%.1f", float64(likeCount)/float64(total)*10))
		}
	})

	err = c.Visit(info.Homepage)
	return
}

func (gg *GGJAV) NormalizeMovieKeyword(keyword string) string {
	return strings.ToUpper(strings.TrimSpace(keyword))
}

func (gg *GGJAV) SearchMovie(keyword string) (results []*model.MovieSearchResult, err error) {
	c := gg.ClonedCollector()

	// Each result item: div.columns.item
	// <div class="columns large-3 medium-6 small-12 item float-left;">
	//   <a href="/main/video?id=134785"><img class="item_image" src="...small_134785.jpg" alt="..."></a>
	//   <div class="item_title"><a class="gray_a" href="/main/video?id=134785">title text</a></div>
	// </div>
	c.OnXML(`//div[contains(@class,"item") and contains(@class,"float-left")]`, func(e *colly.XMLElement) {
		href := e.ChildAttr(`.//div[@class="item_title"]/a`, "href")
		if href == "" {
			return
		}
		homepage := e.Request.AbsoluteURL(href)
		id, idErr := gg.ParseMovieIDFromURL(homepage)
		if idErr != nil || id == "" {
			return
		}
		title := strings.TrimSpace(e.ChildText(`.//div[@class="item_title"]/a`))
		// 结果标题以番号开头（如 "NHDTB-817 長裙女學生…"）。
		// MovieSearchResult.IsValid() 要求 Number 非空，此前未填充该字段，
		// 导致引擎把每一条搜索结果都判为无效而丢弃，表现为“搜索无结果”。
		number, _, _ := strings.Cut(title, " ")
		if !numberRe.MatchString(number) {
			number = parser.ParseIDToNumber(id)
		}
		thumbURL := e.Request.AbsoluteURL(e.ChildAttr(`.//img[@class="item_image"]`, "src"))
		// Cover URL uses "large_" prefix instead of "small_"
		coverURL := strings.Replace(thumbURL,
			"/media/video/small_", "/media/video/large_", 1)

		results = append(results, &model.MovieSearchResult{
			ID:       id,
			Number:   number,
			Title:    title,
			Provider: gg.Name(),
			Homepage: homepage,
			ThumbURL: thumbURL,
			CoverURL: coverURL,
		})
	})

	err = c.Visit(fmt.Sprintf(searchURL, url.QueryEscape(keyword)))
	return
}

func init() {
	provider.Register(Name, New)
}
