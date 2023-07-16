package downloader

import (
	"os"
	"os/signal"
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
	tstring := `https://speed.hetzner.de/100MB.bin`
	dl := NewDownloader(WithTarget(tstring))
	dl.prefetch()
	t.Log(dl.fileSize)
}

func TestDownload(t *testing.T) {
	sig := make(chan os.Signal)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	tstring := `https://speed.hetzner.de/1GB.bin`
	dl := NewDownloader(WithTarget(tstring))
	go dl.Start()
	<-sig
	dl.Interrupt()

}
