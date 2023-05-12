package vermouth

import (
	"bytes"
	"encoding/binary"

	rate "github.com/qiancijun/vermouth/rate_limiter"
)

// 还没规划存储的方法

type SnapShot struct {
	HttpProxyMapping map[int64]HttpProxySnap
}

type HttpProxySnap struct {
	Port            int64
	PrefixerType    string
	Paths           map[string][]string
	BalancerType    map[string]string
	RateLimiterType map[string][]*rate.LimiterInfo
	Record          map[string]bool
	Available       bool // 对整个代理启停的控制
}

func (snap *HttpProxySnap) Marshal() ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := binary.Write(buf, binary.BigEndian, snap); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}