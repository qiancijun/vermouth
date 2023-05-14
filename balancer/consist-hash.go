package balancer

import (
	"errors"
	"fmt"
	"hash/crc32"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/qiancijun/vermouth/logger"
	"github.com/qiancijun/vermouth/utils"
)

type Hash func(data []byte) uint32

type ConsistenceHash struct {
	hash Hash
	replicas int
	keys []int
	hosts map[int]string
	list []string
	sync.RWMutex
}

var _ Balancer = (*ConsistenceHash)(nil)

const (
	TypeConsistenceHash = "consistence-hash"
	Authorization = "Authorization"
)

func init() {
	factories[TypeConsistenceHash] = NewConsistenceHash
}

func NewConsistenceHash(hosts []string) Balancer {
	logger.Infof("consistence-hash balance for %v", hosts)
	ch :=  &ConsistenceHash{
		replicas: 3,
		hash: crc32.ChecksumIEEE,
		hosts: make(map[int]string),
		list: make([]string, 0),
	}
	for _, host := range hosts {
		ch.Add(host)
		ch.list = append(ch.list, host)
	}
	return ch
}

func (c *ConsistenceHash) Add(host string) {
	c.Lock()
	defer c.Unlock()
	for i := 0; i < c.replicas; i++ {
		hash := int(c.hash([]byte(strconv.Itoa(i) + host)))
		c.keys = append(c.keys, hash)
		c.hosts[hash] = host
	}
	sort.Ints(c.keys)
}

func (c *ConsistenceHash) Remove(host string) {
	c.Lock()
	defer c.Unlock()
	for i := 0; i < c.replicas; i++ {
		hash := int(c.hash([]byte(strconv.Itoa(i) + host)))
		idx := sort.SearchInts(c.keys, hash)
		c.keys = append(c.keys[:idx], c.keys[idx+1:]...)
		delete(c.hosts, hash)
	}
	for i, v := range c.list {
		if v == host {
			c.list = append(c.list[:i], c.list[i+1:]...)
			return
		}
	}
}

func (c *ConsistenceHash) Balance(url string, r *http.Request) (string, error) {
	c.RLock()
	defer c.RLocker()
	if len(c.keys) == 0 {
		return "", errors.New("can't find any host")
	}
	if r != nil {
		ipWithPort := utils.RemoteIp(r)
		portIdx := strings.LastIndex(ipWithPort, ":")
		ip := ipWithPort[:portIdx]
		token := r.Header.Get(Authorization)
		url = fmt.Sprintf("%s.%s", ip, token)
	}
	hash := int(c.hash([]byte(url)))
	idx := sort.Search(len(c.keys), func(i int) bool {
		return c.keys[i] >= hash
	})
	return c.hosts[c.keys[idx%len(c.keys)]], nil
}

func (c *ConsistenceHash) Inc(_ string) {}

func (c *ConsistenceHash) Done(_ string) {}

func (c *ConsistenceHash) Len() int { return len(c.hosts) }

func (c *ConsistenceHash) Mode() string { return TypeConsistenceHash }

func (c *ConsistenceHash) Hosts() []string {
	return c.list
}