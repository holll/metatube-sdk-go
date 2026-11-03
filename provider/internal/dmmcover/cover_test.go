package dmmcover

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/metatube-community/metatube-sdk-go/internal/envconfig"
	"github.com/metatube-community/metatube-sdk-go/model"
)

const testCID = "1dldss00409"

const testCandidate = "https://awsimgsrc.dmm.co.jp/pics_dig/digital/video/" + testCID + "/" + testCID + "pl.jpg"

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type countingBody struct {
	reader io.Reader
	read   int
	closed int
}

func (b *countingBody) Read(p []byte) (int, error) {
	n, err := b.reader.Read(p)
	b.read += n
	return n, err
}

func (b *countingBody) Close() error { b.closed++; return nil }

func testMovie() model.MovieInfo {
	return model.MovieInfo{
		ID: "test", Title: "テスト", Number: "TEST-001",
		CoverURL: "https://example.invalid/cover.jpg", BigCoverURL: "https://example.invalid/big-cover.jpg",
		ThumbURL: "https://example.invalid/thumb.jpg", BigThumbURL: "https://example.invalid/big-thumb.jpg",
		PreviewImages: []string{"https://pics.dmm.co.jp/digital/video/" + testCID + "/sample.jpg?size=small"},
	}
}

func setTestConfig(t *testing.T, u *Updater, value string) {
	t.Helper()
	config := envconfig.NewConfig()
	config.Set("use_dmm_hd_cover", value)
	if err := u.SetConfig(config); err != nil {
		t.Fatalf("设置配置失败: %v", err)
	}
}

func testJPEG(t *testing.T) []byte {
	t.Helper()
	var data bytes.Buffer
	if err := jpeg.Encode(&data, image.NewRGBA(image.Rect(0, 0, 2, 3)), nil); err != nil {
		t.Fatal(err)
	}
	return data.Bytes()
}

func assertMovie(t *testing.T, got, want model.MovieInfo) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("影片字段不符\n实际: %+v\n预期: %+v", got, want)
	}
}

func TestConfig(t *testing.T) {
	jpegData := testJPEG(t)
	for _, value := range []string{"true", "TRUE", "1", "false", "FALSE", "0"} {
		t.Run(value, func(t *testing.T) {
			u := New(nil)
			// 显式 false 必须能关闭之前已启用的设置。
			setTestConfig(t, u, "true")
			setTestConfig(t, u, value)
			calls := 0
			u.client.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
				calls++
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(jpegData))}, nil
			})
			info := testMovie()
			want := testMovie()
			wantCalls := 0
			if value == "true" || value == "TRUE" || value == "1" {
				wantCalls = 1
				want.CoverURL, want.BigCoverURL = testCandidate, testCandidate
			}
			u.Update(&info)
			assertMovie(t, info, want)
			if calls != wantCalls {
				t.Errorf("请求数 = %d，预期 %d", calls, wantCalls)
			}
		})
	}
	for _, mode := range []string{"默认关闭", "缺失键关闭已有配置"} {
		t.Run(mode, func(t *testing.T) {
			u := New(nil)
			if mode != "默认关闭" {
				setTestConfig(t, u, "true")
				if err := u.SetConfig(envconfig.NewConfig()); err != nil {
					t.Fatal(err)
				}
			}
			u.client.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
				t.Error("关闭时不应请求网络")
				return nil, errors.New("unexpected request")
			})
			info := testMovie()
			u.Update(&info)
			assertMovie(t, info, testMovie())
		})
	}
	for _, old := range []string{"true", "false"} {
		t.Run("非法配置保留_"+old, func(t *testing.T) {
			u := New(nil)
			setTestConfig(t, u, old)
			for _, invalid := range []string{"", "yes", "not-a-bool", " true "} {
				config := envconfig.NewConfig()
				config.Set("use_dmm_hd_cover", invalid)
				if err := u.SetConfig(config); err == nil || !strings.Contains(err.Error(), "use_dmm_hd_cover") {
					t.Errorf("非法 bool %q 应返回含键名的错误: %v", invalid, err)
				}
				calls := 0
				u.client.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
					calls++
					return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(jpegData))}, nil
				})
				info, want := testMovie(), testMovie()
				wantCalls := 0
				if old == "true" {
					wantCalls = 1
					want.CoverURL, want.BigCoverURL = testCandidate, testCandidate
				}
				u.Update(&info)
				assertMovie(t, info, want)
				if calls != wantCalls {
					t.Errorf("非法配置改变旧设置，请求数 = %d", calls)
				}
			}
		})
	}
}

func TestCID(t *testing.T) {
	for _, hostPath := range []string{"pics.dmm.co.jp/digital/video/", "awsimgsrc.dmm.co.jp/pics_dig/digital/video/"} {
		for _, scheme := range []string{"http", "https"} {
			for _, id := range []string{testCID, "test_001", "00001"} {
				raw := scheme + "://" + hostPath + id + "/sample.jpg?key=value"
				if got := cid(raw); got != id {
					t.Errorf("cid(%q) = %q，预期 %q", raw, got, id)
				}
			}
		}
	}
	invalid := []string{
		"", "/digital/video/test001/sample.jpg", "//pics.dmm.co.jp/digital/video/test001/sample.jpg",
		"ftp://pics.dmm.co.jp/digital/video/test001/sample.jpg",
		"https://pics.dmm.co.jp.evil.invalid/digital/video/test001/sample.jpg",
		"https://evil.invalid/pics.dmm.co.jp/digital/video/test001/sample.jpg",
		"https://sub.pics.dmm.co.jp/digital/video/test001/sample.jpg",
		"https://pics.dmm.co.jp./digital/video/test001/sample.jpg",
		"https://user@pics.dmm.co.jp/digital/video/test001/sample.jpg",
		"https://user:pass@awsimgsrc.dmm.co.jp/pics_dig/digital/video/test001/sample.jpg",
		"https://pics.dmm.co.jp:443/digital/video/test001/sample.jpg",
		"http://pics.dmm.co.jp:80/digital/video/test001/sample.jpg",
		"https://pics.dmm.co.jp:bad/digital/video/test001/sample.jpg",
		"https://pics.dmm.co.jp/pics_dig/digital/video/test001/sample.jpg",
		"https://awsimgsrc.dmm.co.jp/digital/video/test001/sample.jpg",
		"https://pics.dmm.co.jp/Digital/video/test001/sample.jpg",
		"https://pics.dmm.co.jp/digital/video//sample.jpg",
		"https://pics.dmm.co.jp/digital/video/test001/",
		"https://pics.dmm.co.jp/digital/video/test001",
		"https://pics.dmm.co.jp/digital/video/test001/sub/sample.jpg",
		"https://pics.dmm.co.jp/digital/video/../sample.jpg",
		"https://pics.dmm.co.jp/digital/video/TEST001/sample.jpg",
		"https://pics.dmm.co.jp/digital/video/test-001/sample.jpg",
		"https://pics.dmm.co.jp/digital/video/%74est001/sample.jpg",
		"https://pics.dmm.co.jp/digital/video/test001/sample%2ejpg",
		"https://pics.dmm.co.jp/digital/video/test001%2fother/sample.jpg",
		"https://pics.dmm.co.jp/digital/video/test001/sample.jpg#fragment",
		"https://pics.dmm.co.jp/digital/video/test001/%zz",
	}
	for _, raw := range invalid {
		t.Run(raw, func(t *testing.T) {
			if got := cid(raw); got != "" {
				t.Errorf("应拒绝 URL，得到 CID %q", got)
			}
			// 无效地址不仅不能提取 CID，也不能触发探测。
			u := New(nil)
			setTestConfig(t, u, "true")
			u.client.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
				t.Error("非法地址不应触发网络")
				return nil, errors.New("unexpected request")
			})
			info := testMovie()
			info.PreviewImages = []string{raw}
			want := info
			u.Update(&info)
			assertMovie(t, info, want)
		})
	}
}

func TestUpdateSkipsAmbiguousOrExistingCover(t *testing.T) {
	for _, mode := range []string{"无预览", "多个CID冲突", "已知封面CID冲突", "已是候选图"} {
		t.Run(mode, func(t *testing.T) {
			u := New(nil)
			setTestConfig(t, u, "true")
			u.client.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
				t.Error("不应触发网络")
				return nil, errors.New("unexpected request")
			})
			info := testMovie()
			switch mode {
			case "无预览":
				info.PreviewImages = nil
			case "多个CID冲突":
				info.PreviewImages = append(info.PreviewImages, "https://awsimgsrc.dmm.co.jp/pics_dig/digital/video/other001/sample.jpg")
			case "已知封面CID冲突":
				info.CoverURL = "https://pics.dmm.co.jp/digital/video/other001/cover.jpg"
			case "已是候选图":
				info.CoverURL = testCandidate
			}
			want := info
			u.Update(&info)
			assertMovie(t, info, want)
		})
	}
}

func TestUpdateResponses(t *testing.T) {
	jpegData := testJPEG(t)
	var pngData bytes.Buffer
	if err := png.Encode(&pngData, image.NewRGBA(image.Rect(0, 0, 2, 3))); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		status int
		data   []byte
		ok     bool
	}{
		{"JPEG_200", 200, jpegData, true},
		{"JPEG_206", 206, jpegData, true},
		{"404", 404, jpegData, false},
		{"405", 405, jpegData, false},
		{"204", 204, jpegData, false},
		{"500", 500, jpegData, false},
		{"HTML", 200, []byte("<html>test</html>"), false},
		{"空响应", 200, nil, false},
		{"损坏JPEG", 200, []byte{0xff, 0xd8, 0xff}, false},
		{"PNG", 200, pngData.Bytes(), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			u := New(nil)
			setTestConfig(t, u, "true")
			body := &countingBody{reader: bytes.NewReader(tc.data)}
			calls := 0
			u.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				if r.Method != http.MethodGet || r.URL.String() != testCandidate || r.Host != "awsimgsrc.dmm.co.jp" {
					t.Errorf("探测必须只请求固定 AWS 地址: %s %s", r.Method, r.URL)
				}
				if r.Header.Get("Range") != "bytes=0-131071" || r.Header.Get("Accept") != "image/jpeg" {
					t.Errorf("探测请求头不符: %v", r.Header)
				}
				return &http.Response{StatusCode: tc.status, Body: body, Header: http.Header{"Content-Type": {"image/jpeg"}}}, nil
			})
			info := testMovie()
			// 相同 CID 的两种 CDN 地址不应被误判为冲突。
			info.PreviewImages = append(info.PreviewImages, "https://awsimgsrc.dmm.co.jp/pics_dig/digital/video/"+testCID+"/sample2.jpg")
			info.CoverURL = "https://pics.dmm.co.jp/digital/video/" + testCID + "/cover.jpg"
			want := info
			want.PreviewImages = append([]string(nil), info.PreviewImages...)
			if tc.ok {
				want.CoverURL, want.BigCoverURL = testCandidate, testCandidate
			}
			u.Update(&info)
			assertMovie(t, info, want)
			if calls != 1 || body.closed != 1 {
				t.Errorf("请求数 = %d，关闭次数 = %d，均应为 1", calls, body.closed)
			}
		})
	}
}

func TestUpdateReadLimit(t *testing.T) {
	for _, valid := range []bool{false, true} {
		t.Run(map[bool]string{false: "超大无效响应", true: "超大JPEG响应"}[valid], func(t *testing.T) {
			data := bytes.Repeat([]byte("x"), 256*1024)
			if valid {
				data = append(testJPEG(t), data...)
			}
			body := &countingBody{reader: bytes.NewReader(data)}
			u := New(nil)
			setTestConfig(t, u, "true")
			u.client.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusOK, Body: body}, nil
			})
			info, want := testMovie(), testMovie()
			if valid {
				want.CoverURL, want.BigCoverURL = testCandidate, testCandidate
			}
			u.Update(&info)
			assertMovie(t, info, want)
			if body.read != 128*1024 || body.closed != 1 {
				t.Errorf("读取 %d 字节，关闭 %d 次，预期 128KiB 且关闭一次", body.read, body.closed)
			}
		})
	}
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

func TestUpdateErrors(t *testing.T) {
	for _, mode := range []string{"网络错误", "读取错误", "超时"} {
		t.Run(mode, func(t *testing.T) {
			u := New(nil)
			setTestConfig(t, u, "true")
			u.SetRequestTimeout(20 * time.Millisecond)
			body := &countingBody{reader: failingReader{}}
			calls := 0
			u.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				switch mode {
				case "网络错误":
					return nil, errors.New("test network error")
				case "超时":
					if _, ok := r.Context().Deadline(); !ok {
						t.Error("超时设置未传递到请求")
						return nil, errors.New("missing deadline")
					}
					<-r.Context().Done()
					if !errors.Is(r.Context().Err(), context.DeadlineExceeded) {
						t.Errorf("请求未按时限取消: %v", r.Context().Err())
					}
					return nil, r.Context().Err()
				default:
					return &http.Response{StatusCode: http.StatusOK, Body: body}, nil
				}
			})
			info := testMovie()
			u.Update(&info)
			assertMovie(t, info, testMovie())
			if calls != 1 || (mode == "读取错误" && body.closed != 1) {
				t.Errorf("请求或关闭次数不符: %d, %d", calls, body.closed)
			}
		})
	}
}

func TestUpdateDoesNotFollowRedirect(t *testing.T) {
	var locationCalls atomic.Int32
	location := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		locationCalls.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer location.Close()
	for _, status := range []int{301, 302, 303, 307, 308} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			u := New(nil)
			setTestConfig(t, u, "true")
			calls := 0
			body := &countingBody{reader: bytes.NewReader(testJPEG(t))}
			u.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				if r.URL.String() != testCandidate {
					t.Errorf("不应请求重定向目标: %s", r.URL)
					return nil, errors.New("unexpected redirect")
				}
				return &http.Response{StatusCode: status, Header: http.Header{"Location": {location.URL}}, Body: body}, nil
			})
			info := testMovie()
			u.Update(&info)
			assertMovie(t, info, testMovie())
			if calls != 1 || body.closed != 1 || locationCalls.Load() != 0 {
				t.Errorf("重定向未被隔离: 请求 %d，关闭 %d，目标请求 %d", calls, body.closed, locationCalls.Load())
			}
		})
	}
}

func TestProxyAndTimeout(t *testing.T) {
	for _, timeout := range []bool{false, true} {
		t.Run(map[bool]string{false: "代理实际接收CONNECT", true: "代理等待被超时中断"}[timeout], func(t *testing.T) {
			requests := make(chan string, 1)
			release := make(chan struct{})
			proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests <- r.Method + " " + r.Host
				if timeout {
					<-release
				}
				w.WriteHeader(http.StatusBadGateway)
			}))
			defer proxy.Close()
			defer close(release)
			u := New(nil)
			setTestConfig(t, u, "true")
			if err := u.SetProxy(proxy.URL); err != nil {
				t.Fatal(err)
			}
			defer u.client.CloseIdleConnections()
			limit := 2 * time.Second
			if timeout {
				limit = 200 * time.Millisecond
			}
			u.SetRequestTimeout(limit)
			info := testMovie()
			done := make(chan struct{})
			go func() { u.Update(&info); close(done) }()
			select {
			case got := <-requests:
				if got != "CONNECT awsimgsrc.dmm.co.jp:443" {
					t.Errorf("代理目标不符: %s", got)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("本机代理未接收到请求")
			}
			select {
			case <-done:
			case <-time.After(3 * time.Second):
				t.Fatal("请求未在时限内返回")
			}
			assertMovie(t, info, testMovie())
		})
	}
}

func TestParallelUpdate(t *testing.T) {
	const workers = 64
	u := New(nil)
	setTestConfig(t, u, "true")
	data := testJPEG(t)
	var calls, closes atomic.Int32
	u.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		if r.URL.String() != testCandidate {
			t.Errorf("并行请求地址不符: %s", r.URL)
		}
		return &http.Response{StatusCode: http.StatusPartialContent, Body: &closeCountingBody{Reader: bytes.NewReader(data), closes: &closes}}, nil
	})
	var wg sync.WaitGroup
	for range workers {
		wg.Go(func() {
			// 同一 Updater 并发处理独立影片，不并发写同一个 MovieInfo。
			info, want := testMovie(), testMovie()
			want.CoverURL, want.BigCoverURL = testCandidate, testCandidate
			u.Update(&info)
			u.Update(&info)
			assertMovie(t, info, want)
		})
	}
	wg.Wait()
	if calls.Load() != workers || closes.Load() != workers {
		t.Errorf("并行请求 %d 次，关闭 %d 次，预期各 %d", calls.Load(), closes.Load(), workers)
	}
}

type closeCountingBody struct {
	io.Reader
	closes *atomic.Int32
}

func (b *closeCountingBody) Close() error { b.closes.Add(1); return nil }
