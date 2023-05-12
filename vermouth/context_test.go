package vermouth

import (
	"testing"

	"github.com/qiancijun/vermouth/config"
	"github.com/stretchr/testify/assert"
)

func TestSerializeContext(t *testing.T) {
	ctx := NewStateContext()
	args := config.HttpProxyArgs{
		PrefixerType: "hash-prefixer",
		Port: 7181,
	}
	httpProxy := NewHttpReverseProxy(args)
	ctx.AddHttpReverseProxy(httpProxy)
	httpProxy.AddPrefix("/api", "round-robin", []string{
		"192.168.3.1:8080",
		"192.168.3.1:8081",
		"192.168.3.1:8082",
	}, false)

	_, err := ctx.Marshal()
	assert.NoError(t, err)
}
