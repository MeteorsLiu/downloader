package downloader

import (
	"context"
	"fmt"
	"io"
	"log"
	"mime"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/MeteorsLiu/downloader/proxy/socks5"
	"github.com/rapid7/go-get-proxied/proxy"
)

const (
	DefaultConnectTimeout = 30 * time.Second
	DefaultRetry          = 5
)

var (
	cb      = context.Background()
	reg     = regexp.MustCompile(`\s|\n`)
	bufPool = sync.Pool{
		New: func() any {
			return make([]byte, 32768)
		},
	}
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
	pool           *Pool
	header         http.Header
	connectTimeout time.Duration
	timeout        time.Duration
	stop           context.Context
	doStop         context.CancelFunc
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
		threadsNum:     runtime.NumCPU(),
		connectTimeout: DefaultConnectTimeout,
		header:         make(http.Header),
		retry:          DefaultRetry,
	}
	for _, o := range opts {
		o(dl)
	}
	if dl.target == "" {
		panic("no target website")
	}
	dl.init()
	return dl
}

func (d *Downloader) safeGetProxy(protocol string) string {
	p := proxy.NewProvider("").
		GetProxy(protocol, d.target)
	if p != nil {
		c := p.String()
		prefix := "socks5"
		if protocol == "http" {
			prefix = "http"
		}
		return prefix + c[strings.Index(c, "|")+1:]
	}
	return ""
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
		return d.safeGetProxy("http")
	case "https":
		return d.safeGetProxy("https")
	default:
		panic("致命错误：无法确定的协议")
	}
}

func (dl *Downloader) init() {
	urlProxy := dl.proxy()
	if urlProxy == "" {
		// we need nil here
		// url.Parse, when the url is empty
		// it will not return nil
		dl.urlProxy = nil
	} else {
		dl.urlProxy, _ = url.Parse(urlProxy)
		if strings.Contains(dl.target, "https") && !socks5.CheckSocks5(dl.urlProxy.Host) {
			panicWithPrefix("您的代理不支持Socks5，必须要Socks5代理才能处理https请求")
		}
	}

	if dl.saveTo != "" {
		dir := path.Dir(dl.saveTo)
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			os.MkdirAll(dir, 0755)
		}
	}
	dl.stop, dl.doStop = context.WithCancel(cb)
	dl.pool = NewPool(dl.stop)
	dl.header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/114.0.0.0 Safari/537.36")
}

func (d *Downloader) Proxy() string {
	return d.urlProxy.String()
}

func (d *Downloader) Size() int {
	return d.fileSize
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

func (d *Downloader) getTimeout() (context.Context, context.CancelFunc) {
	if d.timeout > 0 {
		return context.WithTimeout(cb, d.timeout)
	}
	return cb, nil
}

func (d *Downloader) do(method string, header ...http.Header) (*http.Response, error) {
	ctx, cancel := d.getTimeout()
	if cancel != nil {
		defer cancel()
	}
	req, err := http.NewRequestWithContext(ctx, method, d.target, nil)
	if err != nil {
		return nil, err
	}
	if len(header) == 0 {
		req.Header = d.header
	} else {
		req.Header = header[0]
	}
	client := &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyURL(d.urlProxy),
			DialContext: func(c context.Context, network, addr string) (net.Conn, error) {
				dialer := &net.Dialer{}
				connectContext, cancelConnect := context.WithTimeout(c, d.connectTimeout)
				defer cancelConnect()
				return dialer.DialContext(connectContext, network, addr)
			},
		},
	}
	return client.Do(req)
}
func (d *Downloader) getSavedPathName() string {
	return strings.Split(path.Base(d.target), "?")[0]
}
func (d *Downloader) prefetch() {
	// don't use HEAD here
	// will cause EOF by HEAD method
	res, err := d.do("GET")
	if err != nil {
		panicWithPrefix(err.Error())
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusPartialContent {
		d.fileSize = getContentRange(res.Header)
	} else {
		clen := int(res.ContentLength)
		if clen == 0 {
			clen = getContentLength(res.Header)
		}
		d.fileSize = clen
	}
	if d.saveTo == "" {
		cp := getHeaderValue(res.Header, "Content-Disposition")
		if cp != "" {
			_, params, err := mime.ParseMediaType(cp)
			if err == nil {
				if fn, ok := params["filename"]; ok {
					d.saveTo = fn
					return
				}
			}
		}
		d.saveTo = d.getSavedPathName()
	}
}

func (d *Downloader) download(f *os.File, id, frombyteRange, tobyteRange int) error {
	header := d.header.Clone()
	if id > 0 {
		tobyteRange -= 1
	}
	ran := fmt.Sprintf("bytes=%d-%d", frombyteRange, tobyteRange)
	fmt.Println(ran, f.Name())
	header.Set("Range", ran)
	res, err := d.do("GET", header)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	_, err = io.Copy(f, res.Body)
	return err
}

func (d *Downloader) newTask(f *os.File, id, from, to int) Task {
	return func() {
		var ticker *time.Ticker
		defer func() {
			if ticker != nil {
				ticker.Stop()
			}
		}()
		for i := 0; i < d.retry; i++ {
			if err := d.download(f, id, from, to); err == nil {
				return
			} else {
				printErr(i, id, err)
			}
			os.Truncate(f.Name(), 0)

			sleepTime := 1 << i * time.Second
			if ticker == nil {
				ticker = time.NewTicker(sleepTime)
			} else {
				ticker.Reset(sleepTime)
			}

			select {
			case <-d.stop.Done():
				return
			case <-ticker.C:
			}
		}

		panicWithPrefix("重试超时")

	}
}

func (d *Downloader) Start() {
	d.prefetch()
	var err error
	size := d.fileSize
	eachChunk := d.fileSize / d.threadsNum
	chunkMap := map[int]*os.File{}
	fileName := path.Base(d.target)

	defer func() {
		// catch panic
		// make sure tmp files cleared
		_ = recover()
		for _, f := range chunkMap {
			fn := f.Name()
			f.Close()
			os.Remove(fn)
		}
	}()

	for i := 0; i < d.threadsNum; i++ {
		chunkMap[i], err = os.CreateTemp("", "*"+fileName)
		if err != nil {
			panicWithPrefix(err.Error())
		}
	}
	for i := 0; size > 0; i++ {
		to := size
		if size >= eachChunk {
			size -= eachChunk
		} else {
			size = 0
		}
		from := size
		file, ok := chunkMap[i]
		if !ok {
			file, err = os.CreateTemp("", "*"+fileName)
			if err != nil {
				panicWithPrefix(err.Error())
			}
			chunkMap[i] = file
		}
		d.pool.Add(d.newTask(file, i, from, to))
	}
	waitPool := make(chan struct{})
	go func() {
		d.pool.Wait()
		close(waitPool)
	}()
	select {
	case <-waitPool:
	case <-d.stop.Done():
		log.Println("已中断")
		return
	}
	printMsg("开始合并")
	saveTo, err := os.OpenFile(d.saveTo, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		panicWithPrefix(err.Error())
	}
	defer saveTo.Close()
	// start to combine the chunks
	// we need to reverse the chunk map
	// because the calculation of the chunk is reversed
	for i := len(chunkMap) - 1; i >= 0; i-- {
		io.Copy(saveTo, chunkMap[i])
	}
	printMsg("下载已完成")
}

func (d *Downloader) Interrupt() {
	d.doStop()
}
