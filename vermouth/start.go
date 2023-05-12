package vermouth

import (
	"errors"
	"os"
	"path/filepath"
	"time"

	jsoniter "github.com/json-iterator/go"
	"github.com/qiancijun/vermouth/logger"
	"github.com/qiancijun/vermouth/utils"
)

var (
	ErrorInterfaceConvert = errors.New("can't convert interface{}")
)

func (vermouth *Vermouth) runCluster() {
	logger.Info("run vermounth in cluster mode")
	args := vermouth.args
	// 是否是初次启动
	bootstrap, _ := utils.PathExists(filepath.Join(args.Server.DataDir, args.Server.Name))

	raftNode, err := createRaftNode(raftRunArgs{
		tcpAddress:        args.Raft.TcpAddress,
		serverName:        args.Server.Name,
		snapshotInterval:  time.Duration(args.Raft.SnapshotInterval),
		snapshotThreshold: uint64(args.Raft.SnapshotThreshold),
		heartbeatTimeout:  time.Duration(args.Raft.HeartbeatTimeout),
		electionTimeout:   time.Duration(args.Raft.ElectionTimeout),
		dataDir:           args.Server.DataDir,
		leader:            args.Raft.Leader,
	}, vermouth.leaderNotify, vermouth.Ctx)
	if err != nil {
		logger.Fatalf("create raft node failed! %v", err)
	}
	vermouth.SetRaftNode(raftNode)
	// defer vermouth.Ctx.Apply(nil)
	defer vermouth.Ctx.Run()

	// 如果是从节点，从配置文件中找主节点信息
	if !args.Raft.Leader {
		arg := vermouth.args
		method := arg.Raft.Method
		switch method {
		case "http":
			if err := joinRaftClusterUseHttp(arg.Raft.LeaderAddress, arg.Raft.TcpAddress, arg.Server.Name); err != nil {
				logger.Fatalf("join raft cluster failed! %v", err)
			} else {
				logger.Infof("join raft cluster successful")
			}
		case "rpc":
			if err := joinRaftClusterUseRpc(arg.Raft.LeaderAddress, arg.Raft.TcpAddress, arg.Server.Name); err != nil {
				logger.Fatalf("join raft cluster failed! %v", err)
			} else {
				logger.Infof("join raft cluster successful")
			}
		}

	}

	// 如果从快照文件中恢复，在恢复的时候就启动了服务
	if !bootstrap {
		// TODO 首次启动，从配置文件中读取配置信息，创建代理，添加到状态机中
		logger.Infof("first time start, according to configuration creating proxies")
		httpProxys := BuildHttpReverseProxyFromConfig(vermouth.args.HttpProxy)
		vermouth.Ctx.AddHttpReverseProxy(httpProxys...)
	}
	go func() {
		// Leader 通知
		for leader := range vermouth.leaderNotify {
			if leader {
				// 成为 leader，开放写权限，
				logger.Infof("%s become the cluster leader", vermouth.args.Server.Name)
				vermouth.Leader = true
				vermouth.Ctx.startDiscovery()
				vermouth.httpServer.SetWriteFlag(true)
				// 如果有 rpc 服务开启一下
				if vermouth.args.Server.RpcServer {
					go vermouth.rpcServer.Start(vermouth.rpcServerCloseChan)
				}
			} else {
				// 成为 follower，关闭写权限
				logger.Infof("%s become the follower", vermouth.args.Server.Name)
				vermouth.Leader = false
				vermouth.httpServer.SetWriteFlag(false)
				// 如果有 rpc 服务，停止掉
				if vermouth.args.Server.RpcServer {
					vermouth.rpcServerCloseChan <- struct{}{}
				}
			}
		}
	}()
}

// 单点模式，raft 完全摒弃，自己实现一个定时快照
func (vermouth *Vermouth) runStand() {
	logger.Info("run vermounth in stand mode")
	// 是否首次启动，单点模式和集群模式不沿用一个快照文件
	args := vermouth.args
	// TODO 找到一个事务 id 最大的文件进行恢复
	snapShotFilePath := filepath.Join(args.Server.DataDir, args.Server.Name+"_stand.json")
	logger.Debugf("snapfile will store at path %s", snapShotFilePath)
	bootstrap, _ := utils.PathExists(snapShotFilePath)

	if bootstrap {
		restoreFromSnap(snapShotFilePath, vermouth.Ctx)
	} else {
		// 从配置文件中读取
		httpProxys := BuildHttpReverseProxyFromConfig(vermouth.args.HttpProxy)
		vermouth.Ctx.AddHttpReverseProxy(httpProxys...)
	}
	// TODO 使用 go-timewheel 实现
	// https://blog.csdn.net/yanchendage/article/details/118735741
	// 添加定时任务，定期生成快照
	vermouth.snap(snapShotFilePath)

	vermouth.Ctx.Run()
	// 如果有 rpc 服务，启动一下
	if vermouth.args.Server.RpcServer {
		if vermouth.rpcServer == nil {
			logger.Error("rpcServer is permited, but rpcServer is nil, it's a bug")
		} else {
			go vermouth.rpcServer.Start(vermouth.rpcServerCloseChan)
		}
	}
	// 启动服务发现
	vermouth.Ctx.startDiscovery()
}

func restoreFromSnap(snapShotFilePath string, ctx *StateContext) {
	logger.Info("find snapshot file successful, restore from snapshot")
	// 读取文件，反序列化

	file, err := os.OpenFile(snapShotFilePath, os.O_RDONLY, 0744)
	if err != nil {
		logger.Errorf("can not read snapshot file, can't restore data from snapshot! %v", err)
		file.Close()
		return
	}
	if err := jsoniter.NewDecoder(file).Decode(&ctx); err != nil {
		logger.Errorf("can't decode snapshot file, maybe the file is crushed! %v", err)
		file.Close()
		return
	}

	// httpReverProxy 根据自身信息重新配置
	for _, httpProxy := range ctx.HttpReverseProxyList {
		if err := httpProxy.Restore(); err != nil {
			logger.Errorf("restore http reverse proxy has encounted error! %v", err)
		}
	}

	file.Close()
}

func (vermounth *Vermouth) snap(snapShotFilePath string) {
	logger.Debug("add cron task to timewheel")
	vermouth.TimeWheel.AddCron(60*time.Second, func() {
		// 快照不需要 stw，不需要对上下文加锁读取

		// 生成一个快照文件，以便后续使用
		exists, _ := utils.PathExists(snapShotFilePath)
		if !exists {
			if err := utils.CreateFile(snapShotFilePath); err != nil {
				logger.Errorf("can't create snapshot file, maybe cause data loss! %v", err)
			}
		}
		// TODO 定时快照每五秒打开一次文件非常消耗性能，待优化
		// TODO 记录修改的事务 id，只有修改过才进行快照
		file, err := os.OpenFile(snapShotFilePath, os.O_WRONLY|os.O_TRUNC, 0744)
		defer file.Close()
		if err != nil {
			logger.Errorf("can't open snapshot file, maybe cause data loss! %v", err)
			return
		}
		if err := jsoniter.NewEncoder(file).Encode(vermounth.Ctx); err != nil {
			logger.Errorf("can't take a snapshot, this is a bug! %v", err)
			return
		}
		logger.Info("take a snapshot successful")
	})
	vermouth.TimeWheel.Start()
}
