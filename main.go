package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/MeteorsLiu/downloader/download"
)

func main() {
	var target string
	var threadsNum int
	var timeout int
	var connectTimeout int
	var retry int
	var saveTo string
	var proxy string
	var verbose bool
	var disableProxy bool

	flag.BoolVar(&verbose, "v", true, "是否开启状态提示(Verbose), 默认开启")
	flag.BoolVar(&disableProxy, "no-proxy", false, "是否关闭代理,  默认不关闭")
	flag.StringVar(&target, "target", "", "下载目标URL")
	flag.StringVar(&saveTo, "o", "", "下载目标保存地址, 默认为当前目录")
	flag.StringVar(&proxy, "p", "", "自定义代理, 默认自动检测")

	flag.IntVar(&threadsNum, "threads", 0, "下载线程数, 默认为CPU核心数量")
	flag.IntVar(&timeout, "timeout", 0, "下载超时时间, 默认不超时, 单位秒")
	flag.IntVar(&connectTimeout, "connect", 0, "目标URL连接超时时间, 默认30秒, 单位秒")
	flag.IntVar(&retry, "retry", 0, "失败重连次数, 默认5次")

	flag.Parse()
	var opts []download.Options
	opts = append(opts, download.WithTarget(target))
	opts = append(opts, download.WithVerbose(verbose))
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
	fmt.Println(flag.Args())
	sig := make(chan os.Signal)
	signal.Notify(sig, syscall.SIGKILL, syscall.SIGTERM, syscall.SIGINT)
	wait := make(chan struct{})
	dw := download.NewDownloader(opts...)
	go dw.Start(wait)
	select {
	case <-sig:
		dw.Interrupt()
		<-wait
	case <-wait:
	}
}
