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
	"github.com/metatube-community/metatube-sdk-go/provider/internal/scraper"
)

var (
	_ provider.MovieProvider = (*GGJAV)(nil)
	_ provider.MovieSearcher = (*GGJAV)(nil)
)

const (
	Name     = "GGJAV"
	Priority = 500
)

const (
	baseURL   = "https://ggjav.com"
	movieURL  = "https://ggjav.com/main/video?id=%s"
	searchURL = "https://ggjav.com/main/search?string=%s"
)

type GGJAV struct {
	*scraper.Scraper
}

func New() *GGJAV {
	return &GGJAV{
		scraper.NewDefaultScraper(Name, baseURL, Priority, language.Chinese,
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

	// Title: div.title_text
	c.OnXML(`//div[contains(@class,"title_text")]`, func(e *colly.XMLElement) {
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

	// Number: div containing "番號："
	c.OnXML(`//div[starts-with(normalize-space(text()),"番號：")]`, func(e *colly.XMLElement) {
		text := strings.TrimSpace(e.Text)
		if after, ok := strings.CutPrefix(text, "番號："); ok {
			info.Number = strings.TrimSpace(after)
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

	// Preview images: images inside the previews section
	// Pattern: https://cdn-1.ggjav.com/media/preview/{id}_{n}.jpg
	previewRe := regexp.MustCompile(`/media/preview/`)
	c.OnXML(`//div[contains(@class,"previews")]//img`, func(e *colly.XMLElement) {
		src := e.Attr("src")
		if src != "" && previewRe.MatchString(src) && !strings.Contains(src, "no_image") {
			imgURL := e.Request.AbsoluteURL(src)
			info.PreviewImages = append(info.PreviewImages, imgURL)
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
		thumbURL := e.Request.AbsoluteURL(e.ChildAttr(`.//img[@class="item_image"]`, "src"))
		// Cover URL uses "large_" prefix instead of "small_"
		coverURL := strings.Replace(thumbURL,
			"/media/video/small_", "/media/video/large_", 1)

		results = append(results, &model.MovieSearchResult{
			ID:       id,
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
