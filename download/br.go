package download

import (
	"context"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
)

var (
	Verbose = true
	lg      = log.New(os.Stderr, "", log.Lshortfile|log.LstdFlags)
	reg     = regexp.MustCompile(`\s|\n`)
)

func Trim(target string) string {
	return reg.ReplaceAllString(target, "")
}

func panicWithPrefix(c string) {
	lg.Panicln("致命错误：" + c)
}

func panicWithPrefixf(format string, c ...any) {
	lg.Panicf("致命错误："+format, c...)
}
func printErr(t, id int, err error) {
	if Verbose {
		lg.Output(2, fmt.Sprintf("\n第 %d 次线程序号：%d 报错: %v", t, id, err))
	}
}

func PrintErr(t, id int, err error) {
	printErr(t, id, err)
}

func printMsg(v ...any) {
	if Verbose {
		fmt.Printf("\n")
		fmt.Println(v...)
	}
}

func PrintMsg(v ...any) {
	printMsg(v...)
}

func printMsgf(f string, v ...any) {
	if Verbose {
		fmt.Printf(f+"\n", v...)
	}
}

func PrintMsgf(f string, v ...any) {
	printMsgf(f, v...)
}

func WaitUserInputf(f string, msg ...any) bool {
	if Verbose {
		var input string
		printMsgf(f, msg...)
		fmt.Scanln(&input)
		input = strings.ToLower(Trim(input))
		if input == "y" || input == "yes" {
			return true
		}

	}
	return false
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

func getContentRange(header http.Header) int64 {
	defer func() {
		if err := recover(); err != nil {
			panicWithPrefix("无法获取文件长度")
		}
	}()
	r := getHeaderValue(header, "Content-Range")
	rg, _ := strconv.ParseInt(strings.Split(r, "/")[1], 10, 64)
	return rg
}

func getContentLength(header http.Header) int64 {
	r := getHeaderValue(header, "Content-Length")
	if r == "" {
		panicWithPrefix("无法获取文件长度")
	}
	rg, _ := strconv.ParseInt(r, 10, 64)
	return rg
}

func getFileModifyTag(header http.Header) (tag string) {
	tag = getHeaderValue(header, "ETag")
	if tag == "" {
		tag = getHeaderValue(header, "Last-Modified")
	}
	return
}

func asyncWork(ctx context.Context, f func(), onFail func()) bool {
	waitPool := make(chan struct{})
	go func() {
		f()
		close(waitPool)
	}()
	select {
	case <-waitPool:
	case <-ctx.Done():
		onFail()
		return false
	}
	return true
}

func sha1File(filename string) ([]byte, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	s1 := sha1.New()
	if _, err := io.Copy(s1, f); err != nil {
		return nil, err
	}
	return s1.Sum(nil), nil
}

func sha1FileString(filename string) (string, error) {
	b, err := sha1File(filename)
	if err != nil {
		return "", nil
	}
	return hex.EncodeToString(b), nil
}
func sha1FileAndVerify(filename, sum string) bool {
	sum2, _ := hex.DecodeString(sum)
	sum1, err := sha1File(filename)
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare(sum1, sum2) == 1
}

func acceptAcceptRange(header http.Header) bool {
	return getHeaderValue(header, "Accept-Ranges") == "bytes"
}
