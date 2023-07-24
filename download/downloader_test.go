package download

import (
	"os"
	"os/signal"
	"path"
	"strings"
	"syscall"
	"testing"
)

func TestTrimTarget(t *testing.T) {
	tstring := `    https://www.com 
	`
	t.Log(strings.ReplaceAll(tstring, " ", "*"))
	t.Log(strings.ReplaceAll(reg.ReplaceAllString(tstring, ""), " ", "*"))
}

func TestGetProxy(t *testing.T) {
	tstring := `    https://speed.hetzner.de/100MB.bin
	`
	t.Log(NewDownloader(WithTarget(tstring)).Proxy())
}

func TestFileSize(t *testing.T) {
	tstring := `https://github.com/puzpuzpuz/xsync`
	dl := NewDownloader(WithTarget(tstring))
	dl.prefetch()
	t.Log(dl.FileSize, dl.ThreadsNum)
}

func TestDownload(t *testing.T) {
	sig := make(chan os.Signal)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	tstring := `https://speed.hetzner.de/1GB.bin`
	dl := NewDownloader(WithTarget(tstring))
	wait := make(chan struct{})
	go dl.Start(wait)
	<-sig
	dl.Interrupt()
	<-wait
}

func TestDir(t *testing.T) {
	_, err := os.Stat(path.Dir("xxx.log"))
	t.Log(os.IsNotExist(err))

	t.Log(os.UserCacheDir())
	t.Log(os.UserConfigDir())
	t.Log(os.UserHomeDir())

	t.Log(getUserPath())
}
