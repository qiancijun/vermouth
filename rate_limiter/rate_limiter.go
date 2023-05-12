package rate

import (
	"errors"
	"time"
)

// 流量限制从两个方面来执行
// 1. 从接口角度来说，设置每一个被代理地址的限流规则。既可以在提供服务的节点来限制
//	  也可以在网关处限制，网关处不提供对每一个提供服务的节点来限制，只做整体限制
// 2. 从用户角度来说，限制用户访问网关的速率（待规划）

type LimiterType string

type RateLimiter interface {
	Take() error
	TakeWithTimeout(time.Duration) error
	SetRate(int, int64) // 设置速率
	GetVolumn() int
	GetSpeed() int64
	GetTimeout() time.Duration
	SetTimeout(time.Duration)
}

type LimiterInfo struct {
	Url   string `json:"url"`
	Type  string `json:"type"`
	Init  int    `json:"init"`
	Speed int64  `json:"speed"`
}

type RateLimiterFactory func() RateLimiter

const (
	INF = 1e9 + 7
)

var (
	ErrLimiterAlreadyExists    = errors.New("limiter already exists")
	ErrLimiterTypeNotSupported = errors.New("limiter type not supported")
	ErrNoReaminToken           = errors.New("The token has been used up")
	rateLimiterFactories       = make(map[LimiterType]RateLimiterFactory)
)

func Build(t LimiterType) (RateLimiter, error) {
	factory, has := rateLimiterFactories[t]
	if !has {
		return nil, ErrLimiterTypeNotSupported
	}
	return factory(), nil
}

func GetLimiterType() (res []string) {
	for k := range rateLimiterFactories {
		res = append(res, string(k))
	}
	return
}
