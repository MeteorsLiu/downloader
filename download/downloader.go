package download

import (
	"context"
	"errors"
	"io"
	"math/rand"
	"mime"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
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
	// this will be ignored
	DoubleThresh = 10 * 1024 * 1024
)

var (
	cb = context.Background()
)

type Downloader struct {
	*DownloaderOptions
	urlProxy *url.URL
	pool     *Pool
	bar      *progressbar.ProgressBar
	stop     context.Context
	doStop   context.CancelFunc
}

func NewDownloader(opts ...Options) *Downloader {
	dl := &Downloader{
		DownloaderOptions: &DownloaderOptions{
			ConnectTimeout: DefaultConnectTimeout,
			Retry:          DefaultRetry,
			verbose:        true,
		},
	}
	for _, o := range opts {
		o(dl.DownloaderOptions)
	}

	if !dl.isRecovering && dl.Target == "" {
		panicWithPrefix("无目标下载")
	}

	dl.init()
	return dl
}

func (d *Downloader) safeGetProxy(protocol string) string {
	p := proxy.NewProvider("").
		GetProxy(protocol, d.Target)
	if p != nil {
		c := p.String()
		return protocol + c[strings.Index(c, "|")+1:]
	}
	return ""
}

func (d *Downloader) proxy() string {
	if d.DisableProxy {
		return ""
	}
	if d.CustomProxy != "" {
		return d.CustomProxy
	}
	switch strings.ToLower(d.Target[0:5]) {
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
func (dl *Downloader) initProxy() {
	urlProxy := dl.proxy()
	if urlProxy == "" {
		// we need nil here
		// url.Parse, when the url is empty
		// it will not return nil
		dl.urlProxy = nil
	} else {
		dl.urlProxy, _ = url.Parse(urlProxy)
		if strings.Contains(dl.Target, "https") && !socks5.CheckSocks5(dl.urlProxy.Host) {
			panicWithPrefix("您的代理不支持Socks5，必须要Socks5代理才能处理https请求")
		}
	}
}

func (dl *Downloader) init() {
	if dl.Header == nil {
		dl.Header = make(http.Header)
		dl.Header.Set("User-Agent", DefaultUserAgent)
	}

	if !dl.isRecovering {
		dl.initProxy()

		if dl.SaveTo != "" {
			dir := filepath.Dir(dl.SaveTo)
			if _, err := os.Stat(dir); os.IsNotExist(err) {
				if err := os.MkdirAll(dir, 0755); err != nil {
					panicWithPrefix(err.Error())
				}
			}
		}
	}
	dl.stop, dl.doStop = context.WithCancel(cb)
	dl.pool = NewPool()
}

func (d *Downloader) Proxy() string {
	return d.urlProxy.String()
}

func (d *Downloader) Size() int64 {
	return d.FileSize
}

func (d *Downloader) SetOptions(opts *DownloaderOptions) {
	d.DownloaderOptions = opts
}

func (d *Downloader) SetRecoverMode() {
	d.isRecovering = true
}

func (d *Downloader) getTimeout() (context.Context, context.CancelFunc) {
	if d.Timeout > 0 {
		return context.WithTimeout(cb, d.Timeout)
	}
	return cb, nil
}

func (d *Downloader) do(method string, header ...http.Header) (*http.Response, error) {
	ctx, cancel := d.getTimeout()
	if cancel != nil {
		defer cancel()
	}
	req, err := http.NewRequestWithContext(ctx, method, d.Target, nil)
	if err != nil {
		return nil, err
	}
	if len(header) == 0 {
		req.Header = d.Header
	} else {
		req.Header = header[0]
	}
	client := &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyURL(d.urlProxy),
			DialContext: func(c context.Context, network, addr string) (net.Conn, error) {
				dialer := &net.Dialer{}
				connectContext, cancelConnect := context.WithTimeout(c, d.ConnectTimeout)
				defer cancelConnect()
				return dialer.DialContext(connectContext, network, addr)
			},
		},
	}
	return client.Do(req)
}
func (d *Downloader) getSavedPathName() string {
	return strings.Split(filepath.Base(d.Target), "?")[0]
}

func (d *Downloader) setThreadsNum() {
	// do safe work
	if d.FileSize <= 0 || d.FileSize <= int64(d.ThreadsNum) {
		d.ThreadsNum = 1
		return
	}
	if !d.CustomThreads {
		d.ThreadsNum = runtime.NumCPU()
		if d.FileSize > 0 && d.FileSize/1024 >= DoubleThresh {
			d.ThreadsNum *= 2
		}
	}
}

func (d *Downloader) setSaveTo(header http.Header) {
	if d.SaveTo == "" {
		cp := getHeaderValue(header, "Content-Disposition")
		if cp != "" {
			_, params, err := mime.ParseMediaType(cp)
			if err == nil {
				if fn, ok := params["filename"]; ok {
					d.SaveTo = fn
					return
				}
			}
		}
		d.SaveTo, _ = filepath.Abs(d.getSavedPathName())
	}
}

func (d *Downloader) setBar(desc string) {
	d.bar = progressbar.NewOptions64(d.FileSize,
		progressbar.OptionSetWriter(os.Stdout),
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

func (d *Downloader) getWriterProgress(w io.Writer) io.Writer {
	if d.verbose {
		return io.MultiWriter(w, d.bar)
	}
	return w
}

func (d *Downloader) retryTask(f func(i int) error) {
	var ticker *time.Ticker
	defer func() {
		if ticker != nil {
			ticker.Stop()
		}
	}()
	for i := 0; i < d.Retry; i++ {
		if err := f(i); err == nil {
			return
		}
		// 1, 2, 4, 8, 16s
		// the duration should not be large.
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

func (d *Downloader) getChunkSize(iter bool) int64 {
	eachChunk := d.FileSize / int64(d.ThreadsNum)
	// avoid the indefinite loop
	if !iter {
		switch {
		case eachChunk == 0:
			// chunk is really small
			d.CustomThreads = false
			d.ThreadsNum = 1
			return d.FileSize
		case eachChunk < 1024 && d.ThreadsNum > runtime.NumCPU():
			// small chunk, too many threads
			// resize the threads to default
			// and re-calculate
			d.CustomThreads = false
			d.setThreadsNum()
			return d.getChunkSize(true)

		}
	}
	return eachChunk
}

func (d *Downloader) prefetch(iter ...bool) {
	// don't use HEAD here
	// will cause EOF by HEAD method
	var res *http.Response
	var err error
	isIter := len(iter) > 0 && iter[0]
	if isIter {
		res, err = d.do("GET")
	} else {
		header := d.Header.Clone()
		header.Set("Range", "bytes=0-")
		// recover mode.
		if d.ModifiedTag != "" && d.isRecovering {
			header.Set("If-Range", d.ModifiedTag)
		}
		res, err = d.do("GET", header)
	}
	if err != nil {
		panicWithPrefix(err.Error())
	}
	defer res.Body.Close()
	isPartial := false
	switch res.StatusCode {
	case http.StatusOK:
		clen := res.ContentLength
		if clen <= 0 {
			cr := getHeaderValue(res.Header, "Content-Range")
			if cr != "" {
				clen, _ = strconv.ParseInt(cr, 10, 64)
			} else if !isIter {
				d.prefetch(true)
				return
			}
		}
		d.FileSize = clen
	case http.StatusPartialContent:
		if res.ContentLength > 0 {
			d.FileSize = res.ContentLength
		} else {
			d.FileSize = getContentRange(res.Header)
		}
		isPartial = true
	case http.StatusRequestedRangeNotSatisfiable:
		if isIter {
			d.FileSize = res.ContentLength
		} else {
			d.prefetch(true)
			return
		}
	default:
		panicWithPrefix("下载文件返回了一个奇怪的状态码: " + res.Status)
	}
	d.ModifiedTag = getFileModifyTag(res.Header)
	if isPartial || acceptAcceptRange(res.Header) {
		d.setThreadsNum()
	} else {
		d.ThreadsNum = 1
	}
	if d.verbose {
		switch d.ThreadsNum {
		case 1:
			d.setBar("[cyan][1/1][reset] 正在下载...")
		default:
			d.setBar("[cyan][1/2][reset] 正在下载...")
		}
	}

	d.setSaveTo(res.Header)
}

func (d *Downloader) download(chunk Chunk) (int64, error) {
	header := d.Header.Clone()
	header.Set("Range", chunk.Range())
	if d.ModifiedTag != "" {
		header.Set("If-Range", d.ModifiedTag)
	}
	res, err := d.do("GET", header)
	if err != nil {
		return 0, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusPartialContent {
		printMsgf("区块ID: %d 范围: %d-%d 发生变更, 保存的内容可能发生改变", chunk.ID(), chunk.From(), chunk.To())
	}
	return io.Copy(d.getWriterProgress(chunk), res.Body)
}

func (d *Downloader) newTask(chunk Chunk) Task {
	return func() {
		// pre-check wether stopped or not.
		select {
		case <-d.stop.Done():
		default:
			chunk.EnterWriting()
			defer chunk.ExitWriting()
			d.retryTask(func(i int) error {
				_, err := d.download(chunk)
				if err == nil {
					return nil
				}
				select {
				case <-d.stop.Done():
					return nil
				default:
					if errors.Is(err, os.ErrClosed) || chunk.IsDone() {
						// success or unexpected EOF?
						return nil
					}
				}
				printErr(i, chunk.ID(), err)
				return err
			})
		}
	}
}
func (d *Downloader) doMultiThreads(saveTo *os.File) {
	size := d.FileSize
	eachChunk := d.getChunkSize(false)
	chunkMap := NewChunkMap(eachChunk)
	fileName := strconv.Itoa(rand.Int())

	defer func() {
		// catch panic
		// make sure tmp files cleared
		_ = recover()
		chunkMap.Close()
		chunkMap.EnterAllWriting()
		if chunkMap.allCombined || !chunkMap.PreSave() {
			chunkMap.ExitAllWriting()
			chunkMap.Remove()
			return
		}

		// we have saved the chunks
		// write up the unfinished work.
		Save(chunkMap.ToJSON(d.DownloaderOptions))
		chunkMap.ExitAllWriting()
	}()

	printMsgf("使用线程: %d ; 文件大小: %d 字节; 线程区块大小: %d 字节; 开始下载",
		d.ThreadsNum,
		d.FileSize,
		eachChunk)

	for i := 0; size > 0; i++ {
		to := size
		if size >= eachChunk {
			size -= eachChunk
		} else {
			size = 0
		}
		from := size
		needMinus := i > 0
		chunk := chunkMap.NewChunk(needMinus, fileName, i, from, to)
		d.pool.Add(d.newTask(chunk))
	}

	if !asyncWork(d.stop, func() {
		d.pool.Wait()
	}, func() {
		printMsg("已中断")
	}) {
		return
	}

	if d.verbose {
		d.setBar("[cyan][2/2][reset] 正在合并分块...")
	}
	if !asyncWork(d.stop, func() {
		chunkMap.Combine(d.getWriterProgress(saveTo), saveTo)
	}, func() {
		printMsg("已中断")
	}) {
		return
	}
	printMsg("下载已完成")
}

func (d *Downloader) doSingleThread(saveTo *os.File) {
	d.retryTask(func(i int) error {
		res, err := d.do("GET")
		if err == nil {
			if _, err := io.Copy(d.getWriterProgress(saveTo), res.Body); err == nil {
				printMsg("下载已完成")
				return nil
			}
		}
		res.Body.Close()
		printErr(i, 0, err)
		return err
	})
}

func (d *Downloader) doDownload() {
	saveTo, err := os.OpenFile(d.SaveTo, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		panicWithPrefix(err.Error())
	}
	defer saveTo.Close()
	if d.ThreadsNum > 1 {
		d.doMultiThreads(saveTo)
	} else {
		asyncWork(d.stop, func() {
			d.doSingleThread(saveTo)
		}, func() {
			printMsg("已中断")
		})
	}
}

func (d *Downloader) doRecover(js []byte) {

	id, chunkMap := FromJSON(d, js)
	if chunkMap == nil {
		return
	}
	defer func() {
		_ = recover()
		chunkMap.Close()
		chunkMap.EnterAllWriting()

		if chunkMap.allCombined {
			chunkMap.ExitAllWriting()
			chunkMap.Remove()
			Finished(id)
			return
		}

		// rewrite the json
		Save(chunkMap.ToJSON(d.DownloaderOptions, id))
		printMsg("恢复失败")
		chunkMap.ExitAllWriting()
	}()
	d.initProxy()
	requireRestore := false
	saveTo, err := os.OpenFile(d.SaveTo, os.O_RDWR, 0644)
	if err != nil {
		saveTo, err = os.Create(d.SaveTo)
		if err != nil {
			printMsg("新建未保存文件失败: " + err.Error())
			return
		}
		requireRestore = true
	}
	defer saveTo.Close()

	if requireRestore {
		chunkMap.RestoreCombine()
	}
	if d.verbose {
		d.setBar("[cyan][1/2][reset] 恢复下载...")
	}

	chunkMap.Iter(func(i int, c Chunk) bool {
		if !c.IsDone() && !c.HasRead() {
			d.pool.Add(d.newTask(c))
		} else {
			d.bar.Add64(c.Size())
		}
		return true
	})
	if !asyncWork(d.stop, func() {
		d.pool.Wait()
	}, func() {
		printMsg("已中断")
	}) {
		return
	}

	if d.verbose {
		d.setBar("[cyan][2/2][reset] 恢复合并...")
	}
	if !asyncWork(d.stop, func() {
		chunkMap.Combine(d.getWriterProgress(saveTo), saveTo)
	}, func() {
		printMsg("已中断")
	}) {
		return
	}
	printMsg("已完成恢复")
}

func (d *Downloader) Start(waitDone chan struct{}, js ...[]byte) {
	if d.isRecovering && len(js) == 0 {
		panicWithPrefix("非法调用")
	}
	defer close(waitDone)

	if d.isRecovering {
		d.doRecover(js[0])
	} else {
		d.prefetch()
		d.doDownload()
	}

}

func (d *Downloader) Interrupt() {
	d.doStop()
}
