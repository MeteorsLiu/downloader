package main

import (
	"flag"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/MeteorsLiu/downloader/download"
)

func work(sig chan os.Signal, task func(wait chan struct{}), interrupt func()) {
	wait := make(chan struct{})
	go task(wait)
	select {
	case <-sig:
		interrupt()
		<-wait
	case <-wait:
	}
}

func main() {
	var target string
	var threadsNum int
	var timeout int
	var connectTimeout int
	var retry int
	var saveTo string
	var proxy string
	var verbose bool
	var allowChanged bool
	var disableProxy bool
	var recoverOnly bool
	var httpHeader header
	flag.BoolVar(&allowChanged, "allow-changed", false, "是否允许在区块发生改变情况下恢复文件, 默认不允许")
	flag.BoolVar(&verbose, "v", true, "是否开启状态提示(Verbose), 默认开启")
	flag.BoolVar(&disableProxy, "no-proxy", false, "是否关闭代理,  默认不关闭")
	flag.BoolVar(&recoverOnly, "recover-only", false, "仅恢复未完成任务, 不进行新的下载")
	flag.StringVar(&target, "target", "", "下载目标URL")
	flag.StringVar(&saveTo, "o", "", "下载目标保存地址, 默认为当前目录")
	flag.StringVar(&proxy, "p", "", "自定义代理, 默认自动检测")

	flag.IntVar(&threadsNum, "threads", 0, "下载线程数, 默认为CPU核心数量")
	flag.IntVar(&timeout, "timeout", 0, "下载超时时间, 默认不超时, 单位秒")
	flag.IntVar(&connectTimeout, "connect", 0, "目标URL连接超时时间, 默认30秒, 单位秒")
	flag.IntVar(&retry, "retry", 0, "失败重连次数, 默认5次")
	flag.Var(&httpHeader, "H", "自定义HTTP Header, 请参考curl")
	flag.Parse()
	var opts []download.Options
	opts = append(opts, download.WithTarget(target))
	opts = append(opts, download.WithVerbose(verbose))

	download.Verbose = verbose

	if threadsNum > 0 {
		opts = append(opts, download.WithThreadsNum(threadsNum))
	}
	if timeout > 0 {
		opts = append(opts, download.WithTimeout(time.Duration(timeout)*time.Second))
	}
	if connectTimeout > 0 {
		opts = append(opts, download.WithConnectTimeout(time.Duration(connectTimeout)*time.Second))
	}
	if retry > 0 {
		opts = append(opts, download.WithRetry(retry))
	}
	if saveTo != "" {
		opts = append(opts, download.WithSaveTo(saveTo))
	}
	if proxy != "" {
		opts = append(opts, download.WithProxy(proxy))
	}
	if disableProxy {
		opts = append(opts, download.WithNoProxy())
	}
	if len(httpHeader) > 0 {
		opts = append(opts, download.WithHeader(httpHeader.Header()))
	}
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGKILL, syscall.SIGTERM, syscall.SIGINT)

	if doRecoverJobs(sig) {
		return
	}
	if !recoverOnly {
		dw := download.NewDownloader(opts...)
		work(sig, func(wait chan struct{}) {
			dw.Start(wait)
		}, func() {
			dw.Interrupt()
		})
	}
}
