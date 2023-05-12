package vermouth

import (
	"bytes"
	"encoding/gob"
)

type addHttpProxyBody struct {
	Port       int64  `json:"port"`
	PrefixType string `json:"prefixType"`
}

func (body *addHttpProxyBody) Encode() ([]byte, error) {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	if err := enc.Encode(body); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

type changeHttpProxyStateBody struct {
	Port  int64 `json:"port"`
	State int   `json:"state"`
}

type addHttpProxyPrefixBody struct {
	Port        int64    `json:"port"`
	Static      bool     `json:"static"`
	Prefix      string   `json:"prefix"`
	BalanceType string   `json:"balanceType"`
	ProxyPass   []string `json:"proxyPass"`
}

type httpProxyHostBody struct {
	Port   int64  `json:"port"`
	Prefix string `json:"prefix"`
	Host   string `json:"host"`
}

type httpBlackListBody struct {
	Port int64    `json:"port"`
	Ips  []string `json:"ips"`
}

type removeHttpProxyPrefixBody struct {
	Port   int64  `json:"port"`
	Prefix string `json:"prefix"`
}

type removeHttpProxyBody struct {
	Port int64 `json:"port"`
}

type httpRateLimiterBody struct {
	Port   int64  `json:"port"`
	Prefix string `json:"prefix"`
	Url    string `json:"url"`
	Type   string `json:"type"`
	Volumn int    `json:"volumn"`
	Speed  int64  `json:"speed"`
}

type httpChangeStatic struct {
	Port   int64  `json:"port"`
	Prefix string `json:"prefix"`
}