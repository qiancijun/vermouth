package vermouth

import (
	"testing"
	"time"

	"github.com/qiancijun/vermouth/client"
	"github.com/qiancijun/vermouth/config"
	"github.com/stretchr/testify/assert"
)

func TestRpcServer(t *testing.T) {
	NewVermouth(&config.VermouthConfig{
		Server: config.ServerArgs{
			Mode: "stand",
		},
	})
	rpcServer := NewRpcServer(9229)
	go rpcServer.Start(make(chan struct{}))
	time.Sleep(1 * time.Second)
	rpcClient := client.NewVermouthRpcClient("localhost:9229")
	err := rpcClient.RegisterToProxy(9000, "/api", "round-robin", "127.0.0.1:8080")
	assert.NoError(t, err)
	proxy := VermouthPeer().Ctx.HttpReverseProxyList[9000]
	assert.NotNil(t, proxy)
	ok := proxy.checkPrefixExists("/api")
	assert.Equal(t, true, ok)
	hosts := proxy.Paths["/api"]
	assert.Equal(t, []string{"127.0.0.1:8080"}, hosts)
}