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

func (r *RaftNodeInfo) AppendLogEntry(option uint16, body interface{}) error {
	data, err := encode(body)
	if err != nil {
		return err
	}
	if err = r.writeLogEntry(option, data); err != nil {
		return err
	}
	return nil
}