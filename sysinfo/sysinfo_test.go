package sysinfo

import (
	"fmt"
	"testing"

	"github.com/elastic/go-sysinfo"
	"github.com/stretchr/testify/assert"
)

func TestGetSysInfo(t *testing.T) {
	_, err := sysinfo.Self()
	assert.NoError(t, err)
	host, err := sysinfo.Host()
	memInfo, err := host.Memory()
	assert.NoError(t, err)
	fmt.Printf("%+v", memInfo)
	info := host.Info()
	fmt.Printf("+%v", info)
	panic(1)
}