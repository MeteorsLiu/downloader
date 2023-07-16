package socks5

import "testing"

func TestSocks5(t *testing.T) {
	t.Log(CheckSocks5("127.0.0.1:8888"))
}
