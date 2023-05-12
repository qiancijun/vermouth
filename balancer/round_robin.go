package balancer

import (
	"sync"

	"github.com/qiancijun/vermouth/logger"
)

// 轮询负载均衡器
type RoundRobin struct {
	sync.RWMutex
	idx  uint64
	hosts []string
}

var _ Balancer = (*RoundRobin)(nil)

const (
	TypeRoundRobin = "round-robin"
)

func init() {
	factories["round-robin"] = NewRoundRobin
}

func NewRoundRobin(hosts []string) Balancer {
	logger.Infof("round-robin balance for %v", hosts)
	return &RoundRobin{
		idx:  0,
		hosts: hosts,
	}
}

// 拒绝重复添加
func (r *RoundRobin) Add(host string) {
	r.Lock()
	defer r.Unlock()
	for _, h := range r.hosts {
		if h == host {
			return
		}
	}
	logger.Infof("round-robin balance for %s", host)
	r.hosts = append(r.hosts, host)
}

func (r *RoundRobin) Remove(host string) {
	r.Lock()
	defer r.Unlock()
	for i, h := range r.hosts {
		if h == host {
			r.hosts = append(r.hosts[:i], r.hosts[i+1:]...)
			return
		}
	}
}

func (r *RoundRobin) Balance(_ string) (string, error) {
	r.RLock()
	defer r.RUnlock()
	if len(r.hosts) == 0 {
		return "", NoHostError
	}
	host := r.hosts[r.idx%uint64(len(r.hosts))]
	r.idx++
	return host, nil
}

// Inc .
func (r *RoundRobin) Inc(_ string) {}

// Done .
func (r *RoundRobin) Done(_ string) {}

func (r *RoundRobin) Len() int { return len(r.hosts) }

func (r *RoundRobin) Mode() string { return "round-robin" }

func (r *RoundRobin) Hosts() []string {
	return r.hosts
}