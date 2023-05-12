package acl

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRadixTreeAdd(t *testing.T) {
	cases := []struct {
		name   string
		hosts  []string
		expect []error
	}{
		{
			"test-1",
			[]string{
				"192.168.3.1/32", "192.168.3.2/32", "192.168.3.3/32",
			},
			[]error{nil, nil, nil},
		},
		{
			"test-2",
			[]string{
				"1234567", "abcdefg", "111.222.333", "1.2.3.4/33",
			},
			[]error{InvaildIpAddress, InvaildIpAddress, InvaildIpAddress, InvaildNetMask},
		},
	}
	for _, c := range cases {
		radixTree := NewRadixTree()
		t.Run(c.name, func(t *testing.T) {
			for i, host := range c.hosts {
				err := radixTree.Add(host, host)
				assert.Equal(t, c.expect[i], err)
			}
		})
	}
}

func TestAccessControl(t *testing.T) {
	cases := []struct {
		name string
		hosts []string
		args []string
		expect []bool
	} {
		{
			"test-1",
			[]string {
				"192.168.3.0/24",
				"192.168.100.100/32",
				"172.12.0.0/16",
			},
			[]string {
				"192.168.3.1",
				"192.168.100.100",
				"172.12.100.100",
			},
			[]bool {
				true, true, true,
			},
		},{
			"test-2",
			[]string {
				"192.168.3.0/24",
				"192.168.100.100/32",
				"172.12.1.0/24",
			},
			[]string {
				"192.168.3.1",
				"192.168.100.101",
				"172.12.2.2",
			},
			[]bool {true, false, false},
		},
	}
	for _, c := range cases {
		radixTree := NewRadixTree()
		t.Run(c.name, func(t *testing.T) {
			for _, host := range c.hosts {
				radixTree.Add(host, host)
			}
			for i, arg := range c.args {
				result := radixTree.AccessControl(arg)
				assert.Equal(t, c.expect[i], result)
			}
		})
	}
}