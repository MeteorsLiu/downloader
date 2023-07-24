package download

import (
	"encoding/json"
	"net/http"
	"time"
)

type Options func(*DownloaderOptions)

type DownloaderOptions struct {
	verbose        bool
	isRecovering   bool
	FileSize       int64         `json:"fileSize"`
	AllowChanged   bool          `json:"allowChanged"`
	Retry          int           `json:"retry"`
	ThreadsNum     int           `json:"threads"`
	CustomThreads  bool          `json:"customThreads"`
	Target         string        `json:"target"`
	SaveTo         string        `json:"saveTo"`
	DisableProxy   bool          `json:"disableProxy"`
	CustomProxy    string        `json:"customProxy"`
	ModifiedTag    string        `json:"tag"`
	Header         http.Header   `json:"header"`
	ConnectTimeout time.Duration `json:"connectTimeout"`
	Timeout        time.Duration `json:"timeout"`
}

func WithThreadsNum(n int) Options {
	return func(d *DownloaderOptions) {
		d.ThreadsNum = n
		d.CustomThreads = true
	}
}

func WithHeader(header http.Header) Options {
	return func(d *DownloaderOptions) {
		d.Header = header
	}
}

func WithTarget(target string) Options {
	return func(d *DownloaderOptions) {
		d.Target = Trim(target)
	}
}

func WithTimeout(timeout time.Duration) Options {
	return func(d *DownloaderOptions) {
		d.Timeout = timeout
	}
}

func WithRetry(retry int) Options {
	return func(d *DownloaderOptions) {
		d.Retry = retry
	}
}

func WithSaveTo(saveTo string) Options {
	return func(d *DownloaderOptions) {
		d.SaveTo = saveTo
	}
}

func WithConnectTimeout(timeout time.Duration) Options {
	return func(d *DownloaderOptions) {
		d.ConnectTimeout = timeout
	}
}

func WithProxy(proxy string) Options {
	return func(d *DownloaderOptions) {
		d.CustomProxy = proxy
	}
}

func WithNoProxy() Options {
	return func(d *DownloaderOptions) {
		d.DisableProxy = true
	}
}

func WithVerbose(v bool) Options {
	return func(d *DownloaderOptions) {
		d.verbose = v
	}
}

func WithAllowChanged(b bool) Options {
	return func(d *DownloaderOptions) {
		d.AllowChanged = b
	}
}

func WithRecoverMode() Options {
	return func(d *DownloaderOptions) {
		d.isRecovering = true
	}
}

func OptsFromJSON(js []byte) *DownloaderOptions {
	var opts DownloaderOptions
	json.Unmarshal(js, &opts)
	if opts.Target == "" {
		return nil
	}
	return &opts
}

func (d *DownloaderOptions) ToJSON() string {
	b, _ := json.Marshal(d)
	return string(b)
}

func (d *DownloaderOptions) SetThreadsNum(n int) {
	d.ThreadsNum = n
}

func (d *DownloaderOptions) SetHeader(header http.Header) {
	d.Header = header
}

func (d *DownloaderOptions) SetTarget(target string) {
	if target != "" {
		d.Target = Trim(target)
	}
}

func (d *DownloaderOptions) SetTimeout(timeout time.Duration) {
	if timeout > 0 {
		d.Timeout = timeout
	}
}

func (d *DownloaderOptions) SetConnectTimeout(timeout time.Duration) {
	if timeout > 0 {
		d.ConnectTimeout = timeout
	}
}

func (d *DownloaderOptions) SetRetry(retry int) {
	d.Retry = retry
}

func (d *DownloaderOptions) SetTag(tag string) {
	if tag != "" {
		d.ModifiedTag = tag
	}
}
