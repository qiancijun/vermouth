package vermouth

import (
	"sync"
	"time"

	"github.com/hashicorp/raft"
	"github.com/qiancijun/vermouth/logger"
)

type RaftNodeInfo struct {
	sync.Mutex
	Raft             *raft.Raft
	Transport        *raft.NetworkTransport
	ServerName       string
	LeaderNotifyChan chan bool

	ctx *StateContext // 仅获取一些信息
}

func (r *RaftNodeInfo) writeLogEntry(opt uint16, data []byte) error {
	entry := LogEntry{opt, data}
	biData, err := entry.Encode()
	if err != nil {
		return err
	}
	future := r.Raft.Apply(biData, 2*time.Second)
	if err := future.Error(); err != nil {
		return err
	}
	idx := future.Index()
	logger.Debugf("write new logEntry idx: %d", idx)
	return nil
}

func (r *RaftNodeInfo) addHttpProxy(body *addHttpProxyBody) error {
	data, err := encode(body)
	if err != nil {
		return err
	}
	if err = r.writeLogEntry(ADD_HTTP_PROXY, data); err != nil {
		return err
	}
	return nil
}

func (r *RaftNodeInfo) addHttpProxyPrefix(body *addHttpProxyPrefixBody) error {
	// body := &addHttpProxyPrefixBody{port,static, prefix, balanceType, hosts}
	data, err := encode(body)
	if err != nil {
		return err
	}
	if err = r.writeLogEntry(ADD_HTTP_PROXY_PREFIX, data); err != nil {
		return err
	}
	return nil
}

func (r *RaftNodeInfo) removeHttpProxyPrefix(port int64, prefix string) error {
	body := &removeHttpProxyPrefixBody{port, prefix}
	data, err := encode(body)
	if err != nil {
		return err
	}
	if err = r.writeLogEntry(RMV_HTTP_PROXY_PREFIX, data); err != nil {
		return err
	}
	return nil
}

func (r *RaftNodeInfo) addHttpProxyHost(body *httpProxyHostBody) error {
	data, err := encode(body)
	if err != nil {
		return err
	}
	if err = r.writeLogEntry(ADD_HTTP_PROXY_HOST, data); err != nil {
		return err
	}
	return nil
}

func (r *RaftNodeInfo) removeHttpProxyHost(port int64, prefix, host string) error {
	body := &httpProxyHostBody{port, prefix, host}
	data, err := encode(body)
	if err != nil {
		return err
	}
	if err = r.writeLogEntry(RMV_HTTP_PROXY_HOST, data); err != nil {
		return err
	}
	return nil
}

func (r *RaftNodeInfo) addBlackList(port int64, ips []string) error {
	body := &httpBlackListBody{port, ips}
	data, err := encode(body)
	if err != nil {
		return err
	}
	if err = r.writeLogEntry(ADD_HTTP_BLACK_LIST, data); err != nil {
		return err
	}
	return nil
}

func (r *RaftNodeInfo) removeBlackList(port int64, ips []string) error {
	body := &httpBlackListBody{port, ips}
	data, err := encode(body)
	if err != nil {
		return err
	}
	if err = r.writeLogEntry(RMV_HTTP_BLACK_LIST, data); err != nil {
		return err
	}
	return nil
}

func (r *RaftNodeInfo) setRateLimiter(body *httpRateLimiterBody) error {
	data, err := encode(body)
	if err != nil {
		return err
	}
	if err = r.writeLogEntry(SET_RATE_LIMITER, data); err != nil {
		return err
	}
	return nil
}


func (r *RaftNodeInfo) changeStatic(body *httpChangeStatic) error {
	data, err := encode(body)
	if err != nil {
		return err
	}
	if err = r.writeLogEntry(CHG_STATIC, data); err != nil {
		return err
	}
	return nil
}