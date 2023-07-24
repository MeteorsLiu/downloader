package main

import (
	"log"
	"net/http"
	"strings"

	"github.com/MeteorsLiu/downloader/download"
)

type header []string

func filterHeader(name string) bool {
	switch strings.ToLower(name) {
	case "range":
		return true
	default:
		return false
	}
}

func (i *header) String() string {
	var sb strings.Builder
	for _, v := range *i {
		sb.WriteString(v)
	}
	return sb.String()
}

func (i *header) Header() http.Header {
	if len(*i) == 0 {
		return nil
	}
	h := make(http.Header)
	for _, v := range *i {
		one := strings.Split(download.Trim(v), ":")
		if len(one) != 2 {
			log.Fatal("无法缺定的Http Header: ", one[0])
		}

		if !filterHeader(one[0]) {
			h.Set(one[0], one[1])
		}
	}
	return h
}

func (i *header) Set(value string) error {
	*i = append(*i, value)
	return nil
}
