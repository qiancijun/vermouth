package utils

import (
	"net"
	"testing"
)

func TestLookupIp(t *testing.T) {
	addr, _ := net.LookupIP("cheryl")
	t.Log(addr)
	panic(1)
}
