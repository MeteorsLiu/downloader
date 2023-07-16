package socks5

import (
	"io"
	"net"
	"time"
)

func CheckSocks5(addr string) bool {
	c, err := net.DialTimeout("tcp", addr, 15*time.Second)
	if err != nil {
		return false
	}
	defer c.Close()
	buf := []byte{5, 1, 0}
	_, err = c.Write(buf)
	if err != nil {
		return false
	}

	if _, err := io.ReadFull(c, buf[:2]); err != nil {
		return false
	}
	// socks5 only
	if buf[0] != 5 || buf[1] != 0 {
		return false
	}

	return true

}
