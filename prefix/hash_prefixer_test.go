package prefix

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMappingFunction(t *testing.T) {
	p := NewHashPrefixer()
	p.Add("/api/service", "round-robin", []string {
		"192.168.3.1:8080",
		"192.168.3.2:8081",
		"192.168.3.3:8082",
	})
	p.Add("/", "round-robin", []string {
		"192.168.4.1",
		"192.168.4.2",
		"192.168.4.3",
	})
	cases := []struct {
		name string
		args []string
		expect []string
	} {
		{
			"test-1",
			[]string {
				"/api/service/hello",
				"/api/service/index",
				"/api/service/login",
			},
			[]string {
				"192.168.3.1:8080/hello",
				"192.168.3.2:8081/index",
				"192.168.3.3:8082/login",
			},
		}, 
		{
			"test-2",
			[]string {
				"/api/hello",
				"/api2/service/index",
				"/login",
			},
			[]string {
				"192.168.4.1/api/hello",
				"192.168.4.2/api2/service/index",
				"192.168.4.3/login",
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			for i, path := range c.args {
				_, _, realPath, err := p.MappingPath(path)
				assert.NoError(t, err)
				assert.Equal(t, c.expect[i], realPath)
			}
		})
	}
}