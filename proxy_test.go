package main

import (
	"context"
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"net/url"
	"testing"
	"time"

	p "github.com/rapid7/go-get-proxied/proxy"
)

func TestProxy(t *testing.T) {
	res, _ := http.Get("http://whatismyip.akamai.com")
	b, _ := io.ReadAll(res.Body)
	t.Log(string(b))

	client := &http.Client{

		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
			Proxy: func(r *http.Request) (*url.URL, error) {
				return url.Parse("socks5://127.0.0.1:8888")
			},
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				dialer := &net.Dialer{}
				connectContext, cancelConnect := context.WithTimeout(ctx, 5*time.Second)
				defer cancelConnect()
				return dialer.DialContext(connectContext, network, addr)
			},
		},
	}

	res, _ = client.Get("https://whatismyip.akamai.com")
	b, _ = io.ReadAll(res.Body)
	t.Log(string(b))
	t.Log(p.NewProvider("").GetProxy("http", "http://whatismyip.akamai.com"))
}
