package main

import "testing"

func TestHeader(t *testing.T) {
	var httpHeader header
	httpHeader.Set("xxx: yyy")
	httpHeader.Set("user: agent")
	t.Log(httpHeader.Header())
}
