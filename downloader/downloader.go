package downloader

import (
	"context"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/rapid7/go-get-proxied/proxy"
)

var (
	cb  = context.Background()
	reg = regexp.MustCompile(`\s|\n`)
)

type Downloader struct {
	retry          int
	fileSize       int
	threadsNum     int
	target         string
	saveTo         string
	disableProxy   bool
	customProxy    string
	urlProxy       *url.URL
	header         http.Header
	connectTimeout time.Duration
	timeout        time.Duration
}

type Options func(*Downloader)

func trimTarget(target string) string {
	return reg.ReplaceAllString(target, "")
}

func WithThreadsNum(n int) Options {
	return func(d *Downloader) {
		d.threadsNum = n
	}
}

func WithHeader(header http.Header) Options {
	return func(d *Downloader) {
		d.header = header
	}
}

func WithTarget(target string) Options {
	return func(d *Downloader) {
		d.target = trimTarget(target)
	}
}

func WithTimeout(timeout time.Duration) Options {
	return func(d *Downloader) {
		d.timeout = timeout
	}
}

func WithRetry(retry int) Options {
	return func(d *Downloader) {
		d.retry = retry
	}
}

func WithSaveTo(saveTo string) Options {
	return func(d *Downloader) {
		d.saveTo = saveTo
	}
}

func WithConnectTimeout(timeout time.Duration) Options {
	return func(d *Downloader) {
		d.connectTimeout = timeout
	}
}

func WithProxy(proxy string) Options {
	return func(d *Downloader) {
		d.customProxy = proxy
	}
}

func WithNoProxy() Options {
	return func(d *Downloader) {
		d.disableProxy = true
	}
}

func NewDownloader(opts ...Options) *Downloader {
	dl := &Downloader{
		threadsNum: runtime.NumCPU(),
	}
	for _, o := range opts {
		o(dl)
	}
	if dl.target == "" {
		panic("no target website")
	}
	urlProxy := dl.proxy()
	if urlProxy == "" {
		// we need nil here
		// url.Parse, when the url is empty
		// it will not return nil
		dl.urlProxy = nil
	} else {
		dl.urlProxy, _ = url.Parse(urlProxy)
	}
	return dl
}

func (d *Downloader) SetThreadsNum(n int) {
	d.threadsNum = n
}

func (d *Downloader) SetHeader(header http.Header) {
	d.header = header
}

func (d *Downloader) SetTarget(target string) {
	d.target = trimTarget(target)
}

func (d *Downloader) SetTimeout(timeout time.Duration) {
	d.timeout = timeout
}

func (d *Downloader) SetConnectTimeout(timeout time.Duration) {
	d.connectTimeout = timeout
}

func (d *Downloader) SetRetry(retry int) {
	d.retry = retry
}

func (d *Downloader) proxy() string {
	if d.disableProxy {
		return ""
	}
	if d.customProxy != "" {
		return d.customProxy
	}
	switch strings.ToLower(d.target[0:5]) {
	case "http:":
		return proxy.NewProvider("").
			GetProxy("http", d.target).
			String()
	case "https":
		return proxy.NewProvider("").
			GetProxy("https", d.target).
			String()
	default:
		panic("致命错误：无法确定的协议")
	}
}

func (d *Downloader) do(method string) (*http.Response, error) {
	ctx, cancel := context.WithTimeout(cb, d.timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, method, d.target, nil)
	if err != nil {
		return nil, err
	}
	req.Header = d.header
	client := &http.Client{
		Transport: &http.Transport{
			Proxy: func(_ *http.Request) (*url.URL, error) {
				return d.urlProxy, nil
			},
			DialContext: func(c context.Context, network, addr string) (net.Conn, error) {
				dialer := &net.Dialer{}
				connectContext, cancelConnect := context.WithTimeout(c, d.connectTimeout)
				defer cancelConnect()
				return dialer.DialContext(connectContext, network, addr)
			},
		},
	}
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	return res, nil
}

func (d *Downloader) prefetch() {
	res, err := d.do("HEAD")
	if err != nil {
		panicWithPrefix(err.Error())
	}
	clen := res.Header.Get("Content-Length")
	if clen == "" {
		// cannot get Content-Length
		// try the lowercase
		for k, v := range res.Header {
			if strings.ToLower(k) == "content-length" {
				clen = v[0]
			}
		}
		if clen == "" {
			panicWithPrefix("无法获取文件长度")
		}
	}
	d.fileSize, _ = strconv.Atoi(clen)
}

func (d *Downloader) download(byteRange int) {

}

func (d *Downloader) Start() {
	d.prefetch()
	size := d.fileSize
	eachChunk := size / d.threadsNum
	for i := 0; i < d.threadsNum; i++ {
		go d.download(size)
		size -= eachChunk
	}
}
