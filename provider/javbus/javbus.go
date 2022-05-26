package javbus

import (
	"fmt"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strings"
	"sync"

	"github.com/gocolly/colly/v2"

	"github.com/javtube/javtube-sdk-go/common/parser"
	"github.com/javtube/javtube-sdk-go/model"
	"github.com/javtube/javtube-sdk-go/provider"
	"github.com/javtube/javtube-sdk-go/provider/internal/scraper"
)

var (
	_ provider.MovieProvider = (*JavBus)(nil)
	_ provider.MovieSearcher = (*JavBus)(nil)
)

const (
	Name     = "JavBus"
	Priority = 1000 - 4
)

const (
	baseURL             = "https://www.javbus.com/"
	movieURL            = "https://www.javbus.com/ja/%s"
	searchURL           = "https://www.javbus.com/ja/search/%s"
	searchUncensoredURL = "https://www.javbus.com/ja/uncensored/search/%s"
)

type JavBus struct {
	*scraper.Scraper
}

func New() *JavBus {
	return &JavBus{
		Scraper: scraper.NewDefaultScraper(Name, baseURL, Priority,
			scraper.WithCookies(baseURL, []*http.Cookie{
				// existmag=all
				{Name: "existmag", Value: "all"},
			})),
	}
}

func (bus *JavBus) NormalizeID(id string) string {
	return strings.ToUpper(id)
}

func (bus *JavBus) GetMovieInfoByID(id string) (info *model.MovieInfo, err error) {
	return bus.GetMovieInfoByURL(fmt.Sprintf(movieURL, id))
}

func (bus *JavBus) ParseIDFromURL(rawURL string) (string, error) {
	homepage, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	return bus.NormalizeID(path.Base(homepage.Path)), nil
}

func (bus *JavBus) GetMovieInfoByURL(rawURL string) (info *model.MovieInfo, err error) {
	id, err := bus.ParseIDFromURL(rawURL)
	if err != nil {
		return
	}

	info = &model.MovieInfo{
		ID:            id,
		Provider:      bus.Name(),
		Homepage:      rawURL,
		Actors:        []string{},
		PreviewImages: []string{},
		Tags:          []string{},
	}

	c := bus.ClonedCollector()

	// Image+Title
	c.OnXML(`//a[@class="bigImage"]/img`, func(e *colly.XMLElement) {
		info.Title = e.Attr("title")
		info.CoverURL = e.Request.AbsoluteURL(e.Attr("src"))
		if re := regexp.MustCompile(`(?i)/cover/([a-z\d]+)(?:_b)?\.(jpg|png)`); re.MatchString(info.CoverURL) {
			info.ThumbURL = re.ReplaceAllString(info.CoverURL, "/thumb/${1}.${2}")
		}
	})

	// Fields
	c.OnXML(`//div[@class="col-md-3 info"]/p`, func(e *colly.XMLElement) {
		switch e.ChildText(`.//span`) {
		case "品番:":
			info.Number = e.ChildText(`.//span[2]`)
		case "発売日:":
			fields := strings.Fields(e.Text)
			info.ReleaseDate = parser.ParseDate(fields[len(fields)-1])
		case "収録時間:":
			fields := strings.Fields(e.Text)
			info.Runtime = parser.ParseRuntime(fields[len(fields)-1])
		case "監督:":
			info.Director = e.ChildText(`.//a`)
		case "メーカー:":
			info.Maker = e.ChildText(`.//a`)
		case "レーベル:":
			info.Publisher = e.ChildText(`.//a`)
		case "シリーズ:":
			info.Series = e.ChildText(`.//a`)
		}
	})

	// Tags
	c.OnXML(`//span[@class="genre"]`, func(e *colly.XMLElement) {
		if tag := strings.TrimSpace(e.ChildText(`.//label/a`)); tag != "" {
			info.Tags = append(info.Tags, tag)
		}
	})

	// Previews
	c.OnXML(`//*[@id="sample-waterfall"]/a`, func(e *colly.XMLElement) {
		info.PreviewImages = append(info.PreviewImages, e.Request.AbsoluteURL(e.Attr("href")))
	})

	// Actors
	c.OnXML(`//div[@class="star-name"]`, func(e *colly.XMLElement) {
		info.Actors = append(info.Actors, e.ChildAttr(`.//a`, "title"))
	})

	err = c.Visit(info.Homepage)
	return
}

func (bus *JavBus) TidyKeyword(keyword string) string {
	if regexp.MustCompile(`^(?i)FC2-`).MatchString(keyword) {
		return "" // JavBus has no FC2 contents.
	}
	return strings.ToUpper(keyword)
}

func (bus *JavBus) SearchMovie(keyword string) (results []*model.MovieSearchResult, err error) {
	c := bus.ClonedCollector()
	c.Async = true /* ASYNC */

	var mu sync.Mutex
	c.OnXML(`//a[@class="movie-box"]`, func(e *colly.XMLElement) {
		mu.Lock()
		defer mu.Unlock()

		var thumb, cover string
		thumb = e.Request.AbsoluteURL(e.ChildAttr(`.//div[1]/img`, "src"))
		if re := regexp.MustCompile(`(?i)/thumbs?/([a-z\d]+)(?:_b)?\.(jpg|png)`); re.MatchString(thumb) {
			cover = re.ReplaceAllString(thumb, "/cover/${1}_b.${2}") // guess
		}

		homepage := e.Request.AbsoluteURL(e.Attr("href"))
		id, _ := bus.ParseIDFromURL(homepage)
		results = append(results, &model.MovieSearchResult{
			ID:          id,
			Number:      e.ChildText(`.//div[2]/span/date[1]`),
			Title:       strings.SplitN(e.ChildText(`.//div[2]/span`), "\n", 2)[0],
			Provider:    bus.Name(),
			Homepage:    homepage,
			ThumbURL:    thumb,
			CoverURL:    cover,
			ReleaseDate: parser.ParseDate(e.ChildText(`.//div[2]/span/date[2]`)),
		})
	})

	for _, u := range []string{
		fmt.Sprintf(searchURL, keyword),
		fmt.Sprintf(searchUncensoredURL, keyword),
	} {
		if err = c.Visit(u); err != nil {
			return nil, err
		}
	}
	c.Wait()
	return
}

func init() {
	provider.RegisterMovieFactory(Name, New)
}
