package discovery

import (
	"reflect"
	"testing"

	"github.com/qiancijun/vermouth/config"
)

func TestReflect(t *testing.T) {
	z := NewZookeeperDiscovery(make(chan Event), config.Discovery{
		Port: 9000,
		Namespace: "zookeeper",
		Address: []string{
			"192.168.3.1:8080",
		},
		ConnectTimeout: 10,
	})
	t.Log(reflect.TypeOf(z))
	panic(1)
}