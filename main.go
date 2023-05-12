package main

import (
	"github.com/qiancijun/vermouth/config"
	"github.com/qiancijun/vermouth/logger"
	"github.com/qiancijun/vermouth/vermouth"
)

// TODO cobra 命令行解析
func main() {
	args := config.NewVermouthConfig()
	err := args.ReadConfig("./conf/conf.toml")
	if err != nil {
		err = args.ReadConfig("../conf/conf.toml")
		if err != nil {
			return
		}
	}
	vermouth.NewVermouth(args)
	logger.BuildFromConfig(args)
	logger.Info(args.Server.DataDir)
	
	peer := vermouth.VermouthPeer()
	peer.Start()
}