package vermouth

import (
	"errors"

	"github.com/qiancijun/vermouth/config"
	"github.com/qiancijun/vermouth/discovery"
	"github.com/qiancijun/vermouth/logger"
)

var (
	ErrHttpProxyNotExists    = errors.New("http proxy not exists")
	ErrHttpProxyAlreayExists = errors.New("http proxy already exists")
)

func (Vermouth *Vermouth) addHttpProxyPrefix(body *addHttpProxyPrefixBody) error {
	proxy, ok := vermouth.Ctx.HttpReverseProxyList[body.Port]
	if !ok {
		return ErrHttpProxyNotExists
	}
	if vermouth.Ctx.HttpReverseProxyList[body.Port].PrefixExists(body.Prefix) {
		return errors.New("prefix already exixts")
	}
	switch vermouth.Mode {
	case "stand":

		proxy.AddPrefix(body.Prefix, body.BalanceType, body.ProxyPass, body.Static)
	case "cluster":
		if err := vermouth.RaftNode.AppendLogEntry(ADD_HTTP_PROXY_PREFIX, body); err != nil {
			return err
		}
	}
	return nil
}

func (Vermouth *Vermouth) removeHttpProxyPrefix(body *removeHttpProxyPrefixBody) error {
	proxy, ok := vermouth.Ctx.HttpReverseProxyList[body.Port]
	if !ok {
		return ErrHttpProxyNotExists
	}
	if !proxy.PrefixExists(body.Prefix) {
		return errors.New("prefix not exists")
	}
	switch vermouth.Mode {
	case Stand:
		proxy.RemovePrefix(body.Prefix)
	case Cluster:
		if err := vermouth.RaftNode.AppendLogEntry(RMV_HTTP_PROXY_PREFIX, body); err != nil {
			return err
		}
	}
	return nil
}

func (vermouth *Vermouth) removeHttpProxyHost(body *httpProxyHostBody) error {
	proxy, ok := vermouth.Ctx.HttpReverseProxyList[body.Port]
	if !ok {
		return ErrHttpProxyNotExists
	}
	if proxy.HostExists(body.Prefix, body.Host) {
		return errors.New("host not exists")
	}
	switch vermouth.Mode {
	case Stand:
		proxy.RemoveHost(body.Prefix, body.Host)
	case Cluster:
		if err := vermouth.RaftNode.AppendLogEntry(RMV_HTTP_PROXY_HOST, body); err != nil {
			return err
		}
	}
	return nil
}

func (vermouth *Vermouth) addHttpProxyHost(body *httpProxyHostBody) error {
	proxy, ok := vermouth.Ctx.HttpReverseProxyList[body.Port]
	if !ok {
		return ErrHttpProxyNotExists
	}
	if proxy.HostExists(body.Prefix, body.Host) {
		return errors.New("host already exists")
	}
	switch vermouth.Mode {
	case Stand:
		proxy.AddHost(body.Prefix, body.Host)
	case Cluster:
		if err := vermouth.RaftNode.AppendLogEntry(ADD_HTTP_PROXY_HOST, body); err != nil {
			return err
		}
	}
	return nil
}

func (vermouth *Vermouth) addBlackList(body *httpBlackListBody) error {
	switch vermouth.Mode {
	case Stand:
		proxy, ok := vermouth.Ctx.HttpReverseProxyList[body.Port]
		if !ok {
			return ErrHttpProxyNotExists
		}
		proxy.AddBlackList(body.Ips)
	case Cluster:
		if err := vermouth.RaftNode.AppendLogEntry(ADD_HTTP_BLACK_LIST, body); err != nil {
			return err
		}
	}
	return nil
}

func (vermouth *Vermouth) removeBlackList(body *httpBlackListBody) error {
	switch vermouth.Mode {
	case Stand:
		proxy, ok := vermouth.Ctx.HttpReverseProxyList[body.Port]
		if !ok {
			return ErrHttpProxyNotExists
		}
		proxy.RemoveBlackList(body.Ips)
	case Cluster:
		if err := vermouth.RaftNode.AppendLogEntry(RMV_HTTP_BLACK_LIST, body); err != nil {
			return err
		}
	}
	return nil
}

func (vermouth *Vermouth) setRateLimiter(body *httpRateLimiterBody) error {
	proxy, ok := vermouth.Ctx.HttpReverseProxyList[body.Port]
	if !ok {
		return ErrHttpProxyNotExists
	}
	switch vermouth.Mode {
	case Stand:
		proxy.SetRateLimit(body.Prefix, body.Url, body.Type, body.Volumn, body.Speed)
	case Cluster:
		if err := vermouth.RaftNode.AppendLogEntry(SET_RATE_LIMITER, body); err != nil {
			return err
		}
	}
	return nil
}

func (vermouth *Vermouth) addHttpProxy(body *addHttpProxyBody) error {
	port := body.Port
	_, ok := vermouth.Ctx.HttpReverseProxyList[port]
	if ok {
		return ErrHttpProxyAlreayExists
	}
	switch vermouth.Mode {
	case Stand:
		proxy := NewHttpReverseProxy(config.HttpProxyArgs{
			Port:         body.Port,
			PrefixerType: body.PrefixType,
			Paths:        []config.HttpProxyPathArgs{},
		})
		vermouth.Ctx.AddHttpReverseProxy(proxy)
		proxy.Run()
	case Cluster:
		if err := vermouth.RaftNode.AppendLogEntry(ADD_HTTP_PROXY, body); err != nil {
			return err
		}
	}
	return nil
}

func (vermouth *Vermouth) changeStatic(body *httpChangeStatic) error {
	proxy, ok := vermouth.Ctx.HttpReverseProxyList[body.Port]
	if !ok {
		return ErrHttpProxyNotExists
	}
	switch vermouth.Mode {
	case Stand:
		proxy.changeStatic(body.Prefix)
	case Cluster:
		if err := vermouth.RaftNode.AppendLogEntry(CHG_STATIC, body); err != nil {
			return err
		}
	}
	return nil
}

func (vermouth *Vermouth) changeLb(body *httpChangeLbBody) error {
	proxy, ok := vermouth.Ctx.HttpReverseProxyList[body.Port]
	if !ok {
		return ErrHttpProxyNotExists
	}
	balancer := proxy.prefixMapping.GetBalancer(body.Prefix)
	if balancer == nil {
		return errors.New("prefix have no balancer")
	}
	if balancer.Mode() == body.Lb {
		return nil
	}
	switch vermouth.Mode {
	case Stand:
		proxy.changeLb(body.Prefix, body.Lb)
	case Cluster:
		if err := vermouth.RaftNode.AppendLogEntry(CHG_LB, body); err != nil {
			return err
		}
	}
	return nil
}

func (vermouth *Vermouth) changeProxyAvailable(body *changeHttpProxyStateBody) error {
	proxy, ok := vermouth.Ctx.HttpReverseProxyList[body.Port]
	if !ok {
		return ErrHttpProxyNotExists
	}
	switch vermouth.Mode {
	case Stand:
		proxy.changeState(body.State)
	case Cluster:
		if err := vermouth.RaftNode.AppendLogEntry(CHG_STATE, body); err != nil {
			return err
		}
	}
	return nil
}

func (vermouth *Vermouth) HandleNamespaceEvent(eventChan chan discovery.Event, port int64) {
	for e := range eventChan {
		logger.Debugf("[httpProxy] receive new Event: %+v", e)
		switch e.Opt {
		case discovery.ADD_HOST:
			NewStateContext().checkAndCreate(port)
			vermouth.addHttpProxyHost(&httpProxyHostBody{
				Port:   port,
				Prefix: "/" + e.ServiceName,
				Host:   e.Host,
			})
		case discovery.REMOVE_HOST:
			vermouth.removeHttpProxyHost(&httpProxyHostBody{
				Port:   port,
				Prefix: "/" + e.ServiceName,
				Host:   e.Host,
			})
		case discovery.ADD_SERVICE:
			NewStateContext().checkAndCreate(port)
			vermouth.addHttpProxyPrefix(&addHttpProxyPrefixBody{
				Port:        port,
				Prefix:      "/" + e.ServiceName,
				BalanceType: "round-robin",
				ProxyPass:   []string{},
			})
		case discovery.REMOVE_SERVICE:
			vermouth.removeHttpProxyPrefix(&removeHttpProxyPrefixBody{
				Port:   port,
				Prefix: "/" + e.ServiceName,
			})
		case discovery.REMOVE_HOSTS:
			visit := make(map[string]bool)
			for _, v := range e.Children {
				visit[v] = true
			}
			hosts := vermouth.Ctx.HttpReverseProxyList[port].GetHosts("/" + e.ServiceName)
			for _, v := range hosts {
				if _, ok := visit[v]; !ok {
					vermouth.removeHttpProxyHost(&httpProxyHostBody{
						Port:   port,
						Prefix: "/" + e.ServiceName,
						Host:   v,
					})
				}
			}
		}
	}
}
