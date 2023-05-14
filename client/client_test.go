package client

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRegisterToVermouth(t *testing.T) {
	c := NewVermouthRpcClient("localhost:9229")
	err := c.RegisterToProxy(9090, "/vemrouth-grpc-test", "192.168.3.101:8888")
	assert.NoError(t, err)
	err = c.Cancel()
	assert.NoError(t, err)
}

func TestInvokeMethod(t *testing.T) {
	c := NewVermouthRpcClient("localhost:9229")
	res := c.InvokeMethod(9000, "/api2", "/hello", GetMethod, nil)
	assert.Equal(t, "<h1>hello world, i'm 8080</h1>", string(res))
}