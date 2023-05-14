package vermouth

import (
	"time"

	"github.com/hashicorp/raft"
	"github.com/qiancijun/vermouth/config"
	"github.com/qiancijun/vermouth/logger"
	tw "github.com/rfyiamcool/go-timewheel"
)

// 只在本进程内使用的上下文环境
type Vermouth struct {
	args       *config.VermouthConfig
	RaftNode   *RaftNodeInfo
	Ctx        *StateContext
	TimeWheel  *tw.TimeWheel // 仅 stand 模式有效
	httpServer *HttpServer
	rpcServer  *rpcServer
	Leader     bool

	Mode string
	// TODO 跨进程 IPC 通信
	Shutdown chan struct{}

	// 异步通信使用
	leaderNotify       chan bool
	rpcServerCloseChan chan struct{}
}

var vermouth *Vermouth

const (
	Stand = "stand"
	Cluster = "cluster"
)

func NewVermouth(conf *config.VermouthConfig) *Vermouth {
	vermouth = &Vermouth{
		args:               conf,
		Shutdown:           make(chan struct{}),
		Ctx:                NewStateContext(),
		Leader:             false,
		Mode:               conf.Server.Mode,
		leaderNotify:       make(chan bool),
		rpcServerCloseChan: make(chan struct{}),
	}
	// 构建服务发现
	vermouth.Ctx.buildDiscovery(conf.Discovery)
	if vermouth.Mode == "stand" {
		t, err := tw.NewTimeWheel(1*time.Second, 360)
		if err != nil {
			logger.Fatal("system internal error, can't init the timewheel, this is a bug")
		}
		vermouth.TimeWheel = t
	}
	// 如果配置文件里开启了 httpServer 才实例这个对象
	if conf.Server.HttpServer {
		vermouth.httpServer = NewHttpServer(conf.Server.HttpPort)
	}
	// 如果配置文件开启了 rpcServer 才实例这个对象
	if conf.Server.RpcServer {
		vermouth.rpcServer = NewRpcServer(conf.Server.RpcPort)
	}
	return vermouth
}

func VermouthPeer() *Vermouth {
	if vermouth == nil {
		// 测试才会走的代码
		vermouth = &Vermouth{
			Shutdown: make(chan struct{}),
			Ctx:      NewStateContext(),
			Leader:   false,
			Mode:     "stand",
		}
	}
	return vermouth
}

func (vermouth *Vermouth) SetRaftNode(r *RaftNodeInfo) {
	vermouth.RaftNode = r
}

func (vermouth *Vermouth) SetState(sc *StateContext) {
	vermouth.Ctx = sc
}

func (vermouth *Vermouth) Start() {
	mode := vermouth.Mode
	logger.Debugf("vermouth will run in %s mode", mode)
	switch mode {
	case "cluster":
		vermouth.runCluster()
	case "stand":
		vermouth.runStand()
	default:
		logger.Fatalf("Unknow execute mode %s, please check config, only support for cluster ans stand mode", mode)
	}
	if vermouth.httpServer != nil {
		vermouth.httpServer.Ctx = vermouth.Ctx
	}
	// HttpServer 只要配置了就启动，不管是 Leader 还是 Follower
	// Rpc 服务只有 Leader 启动
	vermouth.httpServer.Start(vermouth.Mode)
	<-vermouth.Shutdown
}

// 分布式环境下才使用
func (vermouth *Vermouth) Available() bool {
	if vermouth.Mode == "stand" {
		return true
	}
	role := vermouth.RaftNode.Raft.State()
	return role == raft.Follower || role == raft.Leader
}


func (vermouth *Vermouth) GetHttpProxy(port int64) *HttpReverseProxy {
	return vermouth.Ctx.HttpReverseProxyList[port]
}