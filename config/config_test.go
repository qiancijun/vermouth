package config

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

func TestViper(t *testing.T) {
	viper.AddConfigPath("../conf")
	viper.SetConfigName("conf")
	viper.SetConfigType("toml")
	err := viper.ReadInConfig()
	assert.NoError(t, err)
	list := viper.Get("http_proxy").([]interface{})
	// args := list[0].(map[string][]interface{})
	t.Log(list)
	panic(1)
}

func TestReadHttpProxy(t *testing.T) {
	viper.AddConfigPath("../conf")
	viper.SetConfigName("conf")
	viper.SetConfigType("toml")
	err := viper.ReadInConfig()
	assert.NoError(t, err)
	httpProxys := viper.Get("http_proxy").([]interface{})
	for _, hp := range httpProxys {
		for k, v := range hp.(map[string]interface{}) {
			t.Log(k, " -> ", v)
		}
	}
	// t.Log(httpProxys)
	panic(1)
}

func TestReadLogger(t *testing.T) {
	viper.AddConfigPath("../conf")
	viper.SetConfigName("conf")
	viper.SetConfigType("toml")
	err := viper.ReadInConfig()
	assert.NoError(t, err)
	slice := viper.Get("log").([]interface{})
	for _, l := range slice {
		args := l.(map[string]interface{})
		for k, v := range args {
			t.Log(k, "->", v)
		}
	}
	panic(1)
}

func TestReadConfigStruct(t *testing.T) {
	c := NewVermouthConfig()
	err := c.ReadConfig("./test_config.toml")
	assert.NoError(t, err)

	server := c.Server
	assert.Equal(t, server.HttpServer, true)
	assert.Equal(t, server.HttpPort, int64(9119))
	assert.Equal(t, server.Name, "vermouth_node_1")
	assert.Equal(t, server.Mode, "cluster")
	assert.Equal(t, server.RootLevel, "Info")
	assert.Equal(t, server.DataDir, "./data")

	hp := c.HttpProxy[0]
	assert.Equal(t, hp.Port, int64(9000))
	assert.Equal(t, hp.PrefixerType, "hash-prefixer")

	api, api2 := hp.Paths[0], hp.Paths[1]
	assert.Equal(t, api.BalanceMode, "round-robin")
	assert.Equal(t, api.Pattern, "/api")
	assert.Equal(t, api.Hosts, []string{"127.0.0.1:8080"})
	assert.Equal(t, api2.BalanceMode, "round-robin")
	assert.Equal(t, api2.Pattern, "/api2")
	assert.Equal(t, api2.Hosts, []string{
		"192.168.3.1:8080",
		"192.168.3.2:8080",
		"192.168.3.3:8080",
	})

	zk := c.Discovery[0]
	assert.Equal(t, zk.Port, int64(9000))
	assert.Equal(t, zk.Namespace, "zookeeper")
	assert.Equal(t, zk.Address, []string{"localhost:2181"})
	assert.Equal(t, zk.ConnectTimeout, int64(10))

	raft := c.Raft
	assert.Equal(t, raft.TcpAddress, "127.0.0.1:7000")
	assert.Equal(t, raft.Leader, true)
	assert.Equal(t, raft.LeaderAddress, "127.0.0.1:9119")
	assert.Equal(t, raft.SnapshotInterval, 10)
	assert.Equal(t, raft.SnapshotThreshold, 1)
	assert.Equal(t, raft.HeartbeatTimeout, 2)
	assert.Equal(t, raft.ElectionTimeout, 2)
}

func TestCheckArg(t *testing.T) {
	c := NewVermouthConfig()
	err := c.ReadConfig("./test_config.toml")
	assert.NoError(t, err)

	vc := reflect.ValueOf(c).Elem()
	for i := 0; i < vc.NumField(); i++ {
		singleArg := vc.Field(i)
		argType := singleArg.Type().Kind()
		switch argType {
		case reflect.Slice:
			l := singleArg.Len()
			for j := 0; j < l; j++ {
				sliceArg := singleArg.Index(j)
				t.Run(fmt.Sprintf("%d->%s", j, singleArg.Type().Name()), func(t *testing.T) {
					ok, err := c.checkArg(sliceArg)
					assert.NoError(t, err)
					assert.Equal(t, true, ok)
				})
			}
		default:
			t.Run(fmt.Sprintf("%d->%s", i, vc.Field(i).Type().Name()), func(t *testing.T) {
				ok, err := c.checkArg(singleArg)
				assert.NoError(t, err)
				assert.Equal(t, true, ok)
			})
		}
	}
}

func TestCheckSingleArg(t *testing.T) {
	cfg := NewVermouthConfig()
	cases := []struct {
		name   string
		val    interface{}
		rule   string
		expect bool
	}{
		{
			name:   "test-required-string-true",
			val:    "notNull",
			rule:   REQUIRED,
			expect: true,
		},
		{
			name:   "test-required-string-false",
			val:    "",
			rule:   REQUIRED,
			expect: false,
		},
		{
			name:   "test-required-int-true",
			val:    9000,
			rule:   REQUIRED,
			expect: true,
		},
		{
			name:   "test-required-int-false",
			val:    0,
			rule:   REQUIRED,
			expect: false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			v := reflect.ValueOf(c.val)
			println(v.Type().Kind().String())
			ok, err := cfg.checkSingleTag(v, c.rule)
			assert.NoError(t, err)
			assert.Equal(t, c.expect, ok)
		})
	}
}

func TestCheckString(t *testing.T) {
	cfg := NewVermouthConfig()
	cases := []struct {
		name   string
		arg    string
		rule   string
		expect bool
	}{
		{
			name:   "test-required-true",
			arg:    "nouNull",
			rule:   REQUIRED,
			expect: true,
		},
		{
			name:   "test-required-false",
			arg:    "",
			rule:   REQUIRED,
			expect: false,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ok, err := cfg.checkString(c.arg, REQUIRED)
			assert.NoError(t, err)
			assert.Equal(t, c.expect, ok)
		})
	}
}

func TestCheckInt(t *testing.T) {
	cfg := NewVermouthConfig()
	cases := []struct {
		name   string
		arg    int
		rule   string
		expect bool
	}{
		{
			name:   "test-required-true",
			arg:    9000,
			rule:   REQUIRED,
			expect: true,
		},
		{
			name:   "test-required-false",
			arg:    0,
			rule:   REQUIRED,
			expect: false,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ok, err := cfg.checkInt(int64(c.arg), REQUIRED)
			assert.NoError(t, err)
			assert.Equal(t, c.expect, ok)
		})
	}
}