package download

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/MeteorsLiu/downloader/proxy/socks5"
	"github.com/rapid7/go-get-proxied/proxy"
	"github.com/schollz/progressbar/v3"
)

const (
	DefaultUserAgent      = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/114.0.0.0 Safari/537.36"
	DefaultConnectTimeout = 30 * time.Second
	DefaultRetry          = 5
	// if the file is larger than the DoubleThresh
	// double the default threadsNum
	// however, if you set the threadsNum,
	// this will be ignored.
	DoubleThresh = 10 * 1024 * 1024 * 1024
)

var (
	cb  = context.Background()
	reg = regexp.MustCompile(`\s|\n`)
)

type Downloader struct {
	verbose        bool
	retry          int
	fileSize       int
	threadsNum     int
	customThreads  bool
	target         string
	saveTo         string
	disableProxy   bool
	customProxy    string
	urlProxy       *url.URL
	pool           *Pool
	bar            *progressbar.ProgressBar
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
		d.customThreads = true
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

func WithVerbose(v bool) Options {
	return func(d *Downloader) {
		d.verbose = v
	}
}

func NewDownloader(opts ...Options) *Downloader {
	dl := &Downloader{
		connectTimeout: DefaultConnectTimeout,
		retry:          DefaultRetry,
		verbose:        true,
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
		return protocol + c[strings.Index(c, "|")+1:]
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
		// if the target is https
		// the proxy must be socks5
		// for https proxy in go is not able to used.
		return d.safeGetProxy("socks5")
	default:
		panic("致命错误：无法确定的协议")
	}
}

func (dl *Downloader) init() {
	_verbose = dl.verbose
	if dl.header == nil {
		dl.header = make(http.Header)
		dl.header.Set("User-Agent", DefaultUserAgent)
	}

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
	dl.pool = NewPool()
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

func (d *Downloader) setThreadsNum() {
	d.threadsNum = runtime.NumCPU()
	if d.fileSize > 0 && !d.customThreads && d.fileSize >= DoubleThresh {
		d.threadsNum *= 2
	}
}

func (d *Downloader) setSaveTo(header http.Header) {
	if d.saveTo == "" {
		cp := getHeaderValue(header, "Content-Disposition")
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

func (d *Downloader) setBar(desc string) {
	d.bar = progressbar.NewOptions(d.fileSize,
		progressbar.OptionSetWriter(os.Stderr),
		progressbar.OptionEnableColorCodes(true),
		progressbar.OptionShowBytes(true),
		progressbar.OptionFullWidth(),
		progressbar.OptionSetDescription(desc),
		progressbar.OptionSetTheme(progressbar.Theme{
			Saucer:        "[green]=[reset]",
			SaucerHead:    "[green]>[reset]",
			SaucerPadding: " ",
			BarStart:      "[",
			BarEnd:        "]",
		}))
}

func (d *Downloader) prefetch() {
	// don't use HEAD here
	// will cause EOF by HEAD method
	res, err := d.do("GET")
	if err != nil {
		panicWithPrefix(err.Error())
	}
	defer res.Body.Close()
	switch res.StatusCode {
	case http.StatusOK:
		clen := int(res.ContentLength)
		if clen == 0 {
			clen = getContentLength(res.Header)
		}
		d.fileSize = clen
	case http.StatusPartialContent:
		d.fileSize = getContentRange(res.Header)
	default:
		panicWithPrefix("下载文件返回了一个奇怪的状态码: " + res.Status)
	}

	if d.verbose {
		d.setBar("[cyan][1/2][reset] 正在下载...")
	}

	d.setThreadsNum()

	d.setSaveTo(res.Header)
}

func (d *Downloader) getWriterProgress(w io.Writer, isRetry ...bool) io.Writer {
	if d.verbose {
		if len(isRetry) == 0 || !isRetry[0] {
			return io.MultiWriter(w, d.bar)
		}
	}
	return w
}

func (d *Downloader) download(f *os.File, frombyteRange, tobyteRange int, isRetry bool) (int64, error) {
	header := d.header.Clone()
	header.Set("Range", fmt.Sprintf("bytes=%d-%d", frombyteRange, tobyteRange))
	res, err := d.do("GET", header)
	if err != nil {
		return 0, err
	}
	defer res.Body.Close()

	return io.Copy(d.getWriterProgress(f, isRetry), res.Body)
}

func (d *Downloader) newTask(f *os.File, id, from, to int) Task {
	return func() {
		var ticker *time.Ticker
		defer func() {
			if ticker != nil {
				ticker.Stop()
			}
		}()
		// the reason why to minus one
		// is 0-1000, 1000-2000 causes ranges repeated
		if id > 0 {
			to -= 1
		}
		for i := 0; i < d.retry; i++ {
			isRetry := i > 0
			if n, err := d.download(f, from, to, isRetry); err == nil {
				return
			} else {
				from += int(n)
				if errors.Is(err, os.ErrClosed) || from == to {
					// success or unexpected EOF?
					return
				}
				printErr(i, id, err)
			}

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

func (d *Downloader) getChunkSize(iter bool) int {
	eachChunk := d.fileSize / d.threadsNum
	// avoid the indefinite loop
	if !iter {
		// small chunk, too many threads
		// resize the threads to default
		// and re-calculate
		if eachChunk < 1024 && d.threadsNum > runtime.NumCPU() {
			d.customThreads = false
			d.setThreadsNum()
			return d.getChunkSize(true)
		}
	}
	return eachChunk
}

func (d *Downloader) Start(waitDone chan struct{}) {
	d.prefetch()
	var err error
	size := d.fileSize
	eachChunk := d.getChunkSize(false)
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
		close(waitDone)
	}()

	printMsgf("使用线程: %d ; 文件大小: %d 字节; 线程区块大小: %d 字节; 开始下载",
		d.threadsNum,
		d.fileSize,
		eachChunk)

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
		printMsg("已中断")
		return
	}
	if d.verbose {
		d.setBar("[cyan][2/2][reset] 正在合并分块...")
	}
	saveTo, err := os.OpenFile(d.saveTo, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		panicWithPrefix(err.Error())
	}
	defer saveTo.Close()
	// start to combine the chunks
	// we need to reverse the chunk map
	// because the calculation of the chunk is reversed
	writer := d.getWriterProgress(saveTo)
	for i := len(chunkMap) - 1; i >= 0; i-- {
		chunkMap[i].Seek(0, io.SeekStart)
		io.Copy(writer, chunkMap[i])
	}
	printMsg("下载已完成")
}

func (d *Downloader) Interrupt() {
	d.doStop()
}
