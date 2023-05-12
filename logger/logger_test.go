package logger

import (
	"testing"

	"github.com/qiancijun/vermouth/config"
	"github.com/stretchr/testify/assert"
)

func TestConsoleLogger(t *testing.T) {
	c := config.NewVermouthConfig()
	c.ReadConfig("../conf/conf")

	BuildFromConfig(c)
	assert.NotNil(t, Log)
	Infof("hello world")
	// for i := 1; i <= 20; i++ {
	// 	Infof("第 %d 行", i)
	// }
}