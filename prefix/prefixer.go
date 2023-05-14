package prefix

import (
	"errors"
	"net/http"

	"github.com/qiancijun/vermouth/balancer"
)

// 前缀路径查询器，每一个前缀绑定了一个负载均衡器
type Prefixer interface {
	Add(string, balancer.Algorithm, []string) error
	List() map[string]balancer.Balancer
	Remove(string)
	GetBalancer(string) balancer.Balancer
	// 给定源请求路径例如 /api/service/hello 返回一个具体的访问地址
	// 若没有找到对应的前缀，返回 "/"
	// 返回值，匹配到的前缀，原pathUrl, 真实的代理地址
	// 例如: /api /hello 127.0.0.1:8080/hello, nil
	MappingPath(string, *http.Request) (string, string, string, error)
}

type Factory func() Prefixer

var (
	PrefixerTypeNotSupportedError = errors.New("Unknown prefixer type")
	PathNotFoundError             = errors.New("Path not found, please check prefix")
	factories                     = make(map[string]Factory)
)

func Build(t string) (Prefixer, error) {
	factory, ok := factories[t]
	if !ok {
		return nil, PrefixerTypeNotSupportedError
	}
	return factory(), nil
}
