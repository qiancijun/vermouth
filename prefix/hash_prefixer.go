package prefix

import (
	"fmt"
	"strings"
	"sync"

	"github.com/qiancijun/vermouth/balancer"
)

// HashPrefixer 是一种最简单的前缀匹配方式
// 采用暴力搜索的方法，匹配每一个子段是不是一个前缀
// 那么对于一个请求，它最后要被代理到“/”
// 这样的请求的子段会被全部搜索一遍，效率不是很高
// 对于系统中有很多长路径代理，慎重使用 HashPrefixer

type HashPrefixer struct {
	sync.RWMutex
	Mapping map[string]balancer.Balancer
}

var _ Prefixer = (*HashPrefixer)(nil)

const (
	TypeHashPrefixer = "hash-prefixer"
)

func init() {
	factories["hash-prefixer"] = NewHashPrefixer
}

func NewHashPrefixer() Prefixer {
	return &HashPrefixer{
		Mapping: make(map[string]balancer.Balancer),
	}
}

func (p *HashPrefixer) Add(prefixPath string, algo balancer.Algorithm, hosts []string) error {
	p.Lock()
	defer p.Unlock()
	b, err := balancer.Build(algo, hosts)
	if err != nil {
		return nil
	}
	p.Mapping[prefixPath] = b
	return nil
}

func (p *HashPrefixer) List() map[string]balancer.Balancer {
	p.RLock()
	defer p.RUnlock()
	list := make(map[string]balancer.Balancer)
	for k, v := range p.Mapping {
		list[k] = v
	}
	return list
}

func (p *HashPrefixer) Remove(prefixPath string) {
	p.Lock()
	defer p.Unlock()
	delete(p.Mapping, prefixPath)
}

func (p *HashPrefixer) GetBalancer(prefixPath string) balancer.Balancer {
	p.RLock()
	defer p.RUnlock()
	return p.Mapping[prefixPath]
}

// 哈希前缀匹配器算法流程：时间复杂度O(m), m 为 len(apiPath)
// 1. 对 apiPath 地址拆分，根据“/”拆分
// 2. 依次查询匹配
func (p *HashPrefixer) MappingPath(apiPath string) (string, string, string, error) {
	p.RLock()
	defer p.RUnlock()
	// 第一个元素是空需要略过
	apiPathSplits := strings.Split(apiPath, "/")[1:]
	prefix, nextPath := "", ""
	idx, offset := 0, 0
	for _, v := range apiPathSplits {
		nextPath = nextPath + "/" + v
		idx += len(v) + 1
		if _, ok := p.Mapping[nextPath]; ok {
			prefix = nextPath
			offset = idx
		}
	}
	// 第一段都没有出现在映射表里
	if len(prefix) == 0 {
		prefix = "/"
	}
	// logger.Debugf("[hash_prefixer] find the patterPath: %s\n", patternPath)
	// 从映射表里拿取负载均衡器，获取真实地址
	b, ok := p.Mapping[prefix]
	if !ok {
		return "", "", "", PathNotFoundError
	}
	host, err := b.Balance("")
	if err != nil {
		return "", "", "", err
	}
	// 拼接真实地址，返回转换结果
	realPath := fmt.Sprintf("%s%s", host, apiPath[offset:])
	return prefix, apiPath[offset:], realPath, nil
}
