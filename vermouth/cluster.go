package vermouth

import (
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/raft"
	raftboltdb "github.com/hashicorp/raft-boltdb"
	"github.com/qiancijun/vermouth/client"
	"github.com/qiancijun/vermouth/logger"
)

const (
	JOIN_SUCCESS = "ok"
)

type raftRunArgs struct {
	tcpAddress        string
	serverName        string
	snapshotInterval  time.Duration
	snapshotThreshold uint64
	heartbeatTimeout  time.Duration
	electionTimeout   time.Duration
	dataDir           string
	leader            bool
}

func newRaftTransport(args raftRunArgs) (*raft.NetworkTransport, error) {
	// TODO 从配置文件中获取地址
	address, err := net.ResolveTCPAddr("tcp", args.tcpAddress)
	if err != nil {
		return nil, err
	}
	transport, err := raft.NewTCPTransport(address.String(), address, 3, 10*time.Second, logger.Log)
	if err != nil {
		return nil, err
	}
	return transport, nil
}

func createRaftNode(args raftRunArgs, leaderNotifyChan chan bool, ctx *StateContext) (*RaftNodeInfo, error) {
	raftCfg := raft.DefaultConfig()
	// TODO 从配置文件中读取信息
	raftCfg.LocalID = raft.ServerID(args.serverName)
	raftCfg.SnapshotInterval = args.snapshotInterval * time.Second
	raftCfg.SnapshotThreshold = args.snapshotThreshold
	raftCfg.NotifyCh = leaderNotifyChan
	raftCfg.Logger = hclog.New(hclog.DefaultOptions)

	raftCfg.HeartbeatTimeout = args.heartbeatTimeout * time.Second
	raftCfg.ElectionTimeout = args.electionTimeout * time.Second

	transport, err := newRaftTransport(args)
	if err != nil {
		return nil, err
	}

	path := filepath.Join(args.dataDir, args.serverName)
	if err := os.MkdirAll(path, 0700); err != nil {
		return nil, err
	}

	snapshotStore, err := raft.NewFileSnapshotStore(path, 1, logger.Log)
	if err != nil {
		return nil, err
	}

	logStore, err := raftboltdb.NewBoltStore(filepath.Join(path, "raft-log.bolt"))
	if err != nil {
		return nil, err
	}

	stableStore, err := raftboltdb.NewBoltStore(filepath.Join(path, "raft-stable.bolt"))
	if err != nil {
		return nil, err
	}

	raftNode, err := raft.NewRaft(raftCfg, ctx, logStore, stableStore, snapshotStore, transport)
	if err != nil {
		return nil, err
	}
	ctx.Raft = raftNode

	// 分布式模式
	isLeader := args.leader
	if isLeader {
		configuration := raft.Configuration{
			Servers: []raft.Server{
				{
					ID:      raftCfg.LocalID,
					Address: transport.LocalAddr(),
				},
			},
		}
		raftNode.BootstrapCluster(configuration)
	}

	return &RaftNodeInfo{
		Raft:      raftNode,
		Transport: transport,
		ctx:       ctx,
	}, nil
}

func joinRaftClusterUseHttp(leaderAddress, tcpAddress, name string) error {
	url := fmt.Sprintf("http://%s/join?tcpAddress=%s&name=%s", leaderAddress, tcpAddress, name)
	logger.Debugf("will join cluster: %s", url)
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if string(body) != JOIN_SUCCESS {
		logger.Errorf("join cluster fail, %s", string(body))
		return fmt.Errorf("join cluster fail")
	}
	logger.Infof("%s join cluster success", name)
	return nil
}

func joinRaftClusterUseRpc(leaderAddress, tcpAddress, name string) error {
	// 创建一个 rpc 客户端，集群环境 Vermouth 之间的通信走 raft
	// 所以 rpc 客户端仅仅在这里需要使用，使用完毕后可以销毁
	c := client.NewVermouthRpcClient(leaderAddress)
	if err := c.JoinCluster(tcpAddress, name); err != nil {
		return err
	}
	c.Close()
	return nil
}