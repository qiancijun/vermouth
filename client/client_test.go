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