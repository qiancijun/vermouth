package vermouth

import (
	"context"
	"errors"
	"fmt"

	"github.com/hashicorp/raft"
	"github.com/qiancijun/vermouth/logger"
	"github.com/qiancijun/vermouth/pb"
	"github.com/qiancijun/vermouth/utils"
	"google.golang.org/grpc"
)

const (
	Success = "success"
)

type rpcServer struct {
	pb.UnimplementedVermouthGrpcServer
	port int64
}

func NewRpcServer(port int64) *rpcServer {
	server := &rpcServer{
		port: port,
	}
	return server
}

func (s *rpcServer) RegisterToProxy(ctx context.Context, req *pb.RegisterToProxyReq) (*pb.Res, error) {
	logger.Debugf("[rpcServer] receive register request, {port: %d, prefix: %s, host: %s}", req.GetPort(), req.GetPrefix(), req.GetLocalAddr())
	mode := VermouthPeer().Mode
	port := req.GetPort()
	proxy, ok := VermouthPeer().Ctx.HttpReverseProxyList[port]
	if !ok {
		if err := VermouthPeer().addHttpProxy(&addHttpProxyBody{
			Port:       req.Port,
			PrefixType: "hash-prefixer",
		}); err != nil {
			errMsg := fmt.Sprintf("rpc can't create new proxy at port %d, it is a bug", req.Port)
			logger.Error(errMsg)
			return &pb.Res{Message: "failed"}, nil
		}
		proxy = VermouthPeer().Ctx.HttpReverseProxyList[port]
		if proxy == nil {
			errMsg := fmt.Sprintf("rpc can't create new proxy at port %d, it is a bug", req.Port)
			logger.Error(errMsg)
			return &pb.Res{Message: "failed"}, nil
		}
	}
	switch mode {
	case "stand":
		// 前缀不存在，创建一个
		if !proxy.checkPrefixExists(req.Prefix) {
			proxy.AddPrefix(req.GetPrefix(), req.GetBalanceMode(), []string{req.GetLocalAddr()}, req.GetStatic())
		} else {
			// 前缀存在，往里添加
			proxy.AddHost(req.GetPrefix(), req.GetLocalAddr())
		}
	case "cluster":
		if !proxy.checkPrefixExists(req.Prefix) {
			if err := VermouthPeer().RaftNode.AppendLogEntry(ADD_HTTP_PROXY_PREFIX, &addHttpProxyPrefixBody{
				Port:        port,
				Prefix:      req.GetPrefix(),
				BalanceType: req.GetBalanceMode(),
				ProxyPass:   []string{req.GetLocalAddr()},
				Static:      req.GetStatic(),
			}); err != nil {
				logger.Errorf("[rpcServer] %s", err.Error())
				return &pb.Res{Message: "failed"}, nil
			}
		} else {
			if err := VermouthPeer().RaftNode.AppendLogEntry(ADD_HTTP_PROXY_HOST, &httpProxyHostBody{
				Port:   port,
				Prefix: req.GetPrefix(),
				Host:   req.GetLocalAddr(),
			}); err != nil {
				logger.Errorf("[rpcServer] %s", err.Error())
				return &pb.Res{Message: "failed"}, nil
			}
		}
	}
	return &pb.Res{Message: Success}, nil
}

// 注销，客户端终止、宕机应该调用此方法
func (s *rpcServer) Cancel(ctx context.Context, req *pb.CancalReq) (*pb.Res, error) {
	logger.Debugf("[rpcServer] receive cancel request, {port: %d, prefix: %s, host: %s}", req.GetPort(), req.GetPrefix(), req.GetLocalAddr())
	if err := VermouthPeer().removeHttpProxyHost(&httpProxyHostBody{
		Port:   req.GetPort(),
		Prefix: req.GetPrefix(),
		Host:   req.GetLocalAddr(),
	}); err != nil {
		return &pb.Res{Message: "failed"}, nil
	}
	// 检查 prefix 是不是为空，如果为空把 prefix 也清除掉
	proxy := VermouthPeer().Ctx.HttpReverseProxyList[req.Port]
	hosts := proxy.GetHosts(req.GetPrefix())
	if len(hosts) == 0 {
		if err := VermouthPeer().removeHttpProxyPrefix(&removeHttpProxyPrefixBody{
			Port:   req.GetPort(),
			Prefix: req.GetPrefix(),
		}); err != nil {
			return &pb.Res{Message: "failed"}, nil
		}
	}
	return &pb.Res{Message: Success}, nil
}

func (s *rpcServer) JoinCluster(ctx context.Context, req *pb.JoinClusterReq) (*pb.Res, error) {
	if VermouthPeer().Mode == Stand {
		return &pb.Res{Message: "failed"}, errors.New("dest vermouth server run with stand mode")
	}
	peerAddress, nodeName := req.GetTcpAddress(), req.GetNodeName()
	addPeerFuture := VermouthPeer().Ctx.Raft.AddVoter(raft.ServerID(nodeName), raft.ServerAddress(peerAddress), 0, 0)
	if err := addPeerFuture.Error(); err != nil {
		errMsg := fmt.Sprintf("Error joining peer to raft, peeraddress:%s, err:%v", peerAddress, err)
		logger.Warn(errMsg)
		return &pb.Res{Message: "failed"}, errors.New("joining cluster failed")
	}
	return &pb.Res{Message: Success}, nil
}

func (s *rpcServer) LoadBalance(ctx context.Context, req *pb.LoadBalanceReq) (*pb.Res, error) {
	logger.Debugf("[rpcServer] receive loadBalance request, {port: %d, prefix: %s}", req.GetPort(), req.GetPrefix())
	proxy := VermouthPeer().GetHttpProxy(req.GetPort())
	if proxy == nil {
		return &pb.Res{Message: ""}, errors.New("proxy not exists")
	}
	addr := proxy.LoadBalance(req.GetPrefix(), req.GetFact())
	return &pb.Res{Message: addr}, nil
}

func (s *rpcServer) Start(closeChan chan struct{}) {
	listen, err := utils.CreateListener(s.port)
	if err != nil {
		logger.Errorf("can't start rpc server, %s", err.Error())
		return
	}
	server := grpc.NewServer()
	pb.RegisterVermouthGrpcServer(server, s)
	go server.Serve(listen)
	logger.Infof("start rpc server success")
	<-closeChan
	server.Stop()
}
