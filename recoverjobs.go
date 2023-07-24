package main

import (
	"os"

	"github.com/MeteorsLiu/downloader/download"
)

func doRemove(jobs [][]byte) {
	for _, job := range jobs {
		if id, c := download.FromJSON(nil, job); c != nil {
			c.Remove()
			download.Finished(id)
		}
	}
}

func doRecoverJobs(sig chan os.Signal) (interrupted bool) {
	jobs := download.Recover()
	if len(jobs) == 0 {
		return
	}
	if !download.WaitUserInputf("发现 %d 个未完成任务, 是否恢复 (y/n)", len(jobs)) {
		doRemove(jobs)
		return
	}
	for _, job := range jobs {
		dw := download.NewDownloader(download.WithRecoverMode())
		work(sig, func(wait chan struct{}) {
			dw.Start(wait, job)
		}, func() {
			dw.Interrupt()
			interrupted = true
		})
		if interrupted {
			break
		}
	}
	return
}
