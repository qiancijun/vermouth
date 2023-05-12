package vermouth

import (
	"fmt"
	"testing"

	"github.com/qiancijun/vermouth/config"
	"github.com/stretchr/testify/assert"
)

func TestRunStand(t *testing.T) {
	peer := VermouthPeer()
	peer.Start()
}

func TestBuildDiscovery(t *testing.T) {
	c := config.NewVermouthConfig()
	c.ReadConfig("../config/test_config")
	v := NewVermouth(c)

	v.Ctx.buildDiscovery(v.args.Discovery)

	for k, d := range c.Discovery {
		t.Run(fmt.Sprintf("[%d]%s", k, d.Namespace), func(t *testing.T) {
			assert.NotNil(t, v.Ctx.discoverNotify[d.Port])
			assert.NotNil(t, v.Ctx.discoveryMap[d.Port])
		})
	}
}