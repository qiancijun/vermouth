package client

import (
	"context"
	"errors"

	"github.com/qiancijun/vermouth/pb"
	"google.golang.org/grpc"
)

const (
	Success = "success"
)

type RpcClient struct {
	addr    string
	conn    *grpc.ClientConn
	service pb.VermouthGrpcClient

	port int64
	prefix string
	localAddr string
}

func NewVermouthRpcClient(remoteAddr string) *RpcClient {
	client := &RpcClient{
		addr: remoteAddr,
	}
	con, err := grpc.Dial(remoteAddr, grpc.WithInsecure())
	if err != nil {
		panic(err)
	}
	client.conn = con
	client.service = pb.NewVermouthGrpcClient(con)
	return client
}

func (client *RpcClient) RegisterToProxy(port int64, prefix, addr string, opt... string) error {
	req := &pb.RegisterToProxyReq{
		Port:        port,
		Prefix:      prefix,
		BalanceMode: "round-robin",
		LocalAddr:   addr,
		Static:      false,
	}
	if len(opt) > 0 {
		req.BalanceMode = opt[0]
	}
	res, err := client.service.RegisterToProxy(context.Background(), req)
	if err != nil {
		return err
	}
	if res.GetMessage() != Success {
		return errors.New("server has encounter some error, please check server log")
	}
	// 注册成功，记录信息
	client.port, client.prefix, client.localAddr = port, prefix, addr
	return nil
}

func (client *RpcClient) Cancel() error {
	req := &pb.CancalReq{
		Port: client.port,
		Prefix: client.prefix,
		LocalAddr: client.localAddr,
	}
	res, err := client.service.Cancel(context.Background(), req)
	if err != nil {
		return err
	}
	if res.GetMessage() != Success {
		return errors.New("server has encounter some error, please check server log")
	}
	return nil
}

func (client *RpcClient) JoinCluster(tcpAddress, name string) error {
	req := &pb.JoinClusterReq{
		NodeName: name,
		TcpAddress: tcpAddress,
	}
	res, err := client.service.JoinCluster(context.Background(), req)
	if err != nil {
		return err
	}
	if res.GetMessage() != Success {
		return errors.New("server has encounter some error, please check server log")
	}
	return nil
}

func (client *RpcClient) Close() {
	client.conn.Close()
}