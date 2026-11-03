package dmmcover

import (
	"bytes"
	"fmt"
	"image"
	_ "image/jpeg"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync/atomic"
	"time"

	"github.com/metatube-community/metatube-sdk-go/model"
	"github.com/metatube-community/metatube-sdk-go/provider"
)

const maxProbeBytes = 128 * 1024

var cidPattern = regexp.MustCompile(`^[a-z0-9_]+$`)

type Updater struct {
	enabled atomic.Bool
	client  *http.Client
}

func New(client *http.Client) *Updater {
	if client == nil {
		client = &http.Client{
			Timeout:   10 * time.Second,
			Transport: http.DefaultTransport.(*http.Transport).Clone(),
		}
	}
	copy := *client
	copy.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &Updater{client: &copy}
}

func (u *Updater) SetConfig(config provider.Config) error {
	enabled := false
	if config.Has("use_dmm_hd_cover") {
		var err error
		enabled, err = config.GetBool("use_dmm_hd_cover")
		if err != nil {
			return fmt.Errorf("use_dmm_hd_cover: %w", err)
		}
	}
	u.enabled.Store(enabled)
	return nil
}

func (u *Updater) SetProxy(raw string) error {
	proxy, err := url.Parse(raw)
	if err != nil {
		return err
	}
	transport := u.client.Transport.(*http.Transport).Clone()
	transport.Proxy = http.ProxyURL(proxy)
	u.client.Transport = transport
	return nil
}

func (u *Updater) SetRequestTimeout(timeout time.Duration) { u.client.Timeout = timeout }

func cid(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.User != nil || parsed.Port() != "" || parsed.RawPath != "" || parsed.Fragment != "" {
		return ""
	}
	prefix := ""
	switch strings.ToLower(parsed.Host) {
	case "pics.dmm.co.jp":
		prefix = "/digital/video/"
	case "awsimgsrc.dmm.co.jp":
		prefix = "/pics_dig/digital/video/"
	default:
		return ""
	}
	if !strings.HasPrefix(parsed.Path, prefix) {
		return ""
	}
	parts := strings.Split(strings.TrimPrefix(parsed.Path, prefix), "/")
	if len(parts) != 2 || !cidPattern.MatchString(parts[0]) || parts[1] == "" {
		return ""
	}
	return parts[0]
}

func (u *Updater) Update(info *model.MovieInfo) {
	if !u.enabled.Load() {
		return
	}
	var contentID string
	for _, raw := range info.PreviewImages {
		if current := cid(raw); current != "" {
			if contentID != "" && current != contentID {
				return
			}
			contentID = current
		}
	}
	if contentID == "" {
		return
	}
	if coverID := cid(info.CoverURL); coverID != "" && coverID != contentID {
		return
	}
	candidate := "https://awsimgsrc.dmm.co.jp/pics_dig/digital/video/" + contentID + "/" + contentID + "pl.jpg"
	if info.CoverURL == candidate {
		return
	}
	req, err := http.NewRequest(http.MethodGet, candidate, nil)
	if err != nil {
		return
	}
	req.Header.Set("Range", fmt.Sprintf("bytes=0-%d", maxProbeBytes-1))
	req.Header.Set("Accept", "image/jpeg")
	resp, err := u.client.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		return
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxProbeBytes))
	if err != nil {
		return
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(body))
	if err != nil || format != "jpeg" || config.Width <= 0 || config.Height <= 0 {
		return
	}
	info.CoverURL = candidate
	info.BigCoverURL = candidate
}
