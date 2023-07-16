package downloader

import (
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
)

var (
	lg = log.New(os.Stderr, "", log.Lshortfile|log.LstdFlags)
)

func panicWithPrefix(c string) {
	lg.Panicln("致命错误：" + c)
}

func printErr(t, id int, err error) {
	lg.Printf("第 %d 次线程序号：%d 报错: %v", t, id, err)
}

func printMsg(v ...any) {
	lg.Println(v...)
}
func getHeaderValue(header http.Header, key string) string {
	value := header.Get(key)
	if value == "" {
		lowerKey := strings.ToLower(key)
		for k, v := range header {
			if strings.ToLower(k) == lowerKey {
				return v[0]
			}
		}
	}
	return value
}

func getContentRange(header http.Header) int {
	defer func() {
		if err := recover(); err != nil {
			panicWithPrefix("无法获取文件长度")
		}
	}()
	r := getHeaderValue(header, "Content-Range")
	rg, _ := strconv.Atoi(strings.Split(r, "/")[1])
	return rg
}

func getContentLength(header http.Header) int {
	r := getHeaderValue(header, "Content-Length")
	if r == "" {
		panicWithPrefix("无法获取文件长度")
	}
	rg, _ := strconv.Atoi(r)
	return rg
}
