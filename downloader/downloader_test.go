package downloader

import (
	"strings"
	"testing"
)

func TestTrimTarget(t *testing.T) {
	tstring := `    https://www.com 
	`
	t.Log(strings.ReplaceAll(tstring, " ", "*"))
	t.Log(strings.ReplaceAll(reg.ReplaceAllString(tstring, ""), " ", "*"))
}
