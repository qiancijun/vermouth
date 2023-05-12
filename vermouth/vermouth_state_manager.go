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
	switch vermouth.Mode {
	case "stand":
		proxy, ok := vermouth.Ctx.HttpReverseProxyList[body.Port]
		if !ok {
			return ErrHttpProxyNotExists
		}
		proxy.AddPrefix(body.Prefix, body.BalanceType, body.ProxyPass, body.Static)
	case "cluster":
		if !vermouth.Ctx.HttpReverseProxyList[body.Port].PrefixExists(body.Prefix) {
			if err := vermouth.RaftNode.addHttpProxyPrefix(body); err != nil {
				return err
			}
		}

	}
	return nil
}

func (Vermouth *Vermouth) removeHttpProxyPrefix(body *removeHttpProxyPrefixBody) error {
	switch vermouth.Mode {
	case "stand":
		proxy, ok := vermouth.Ctx.HttpReverseProxyList[body.Port]
		if !ok {
			return ErrHttpProxyNotExists
		}
		proxy.RemovePrefix(body.Prefix)
	case "cluster":
		if vermouth.Ctx.HttpReverseProxyList[body.Port].PrefixExists(body.Prefix) {
			if err := vermouth.RaftNode.removeHttpProxyPrefix(body.Port, body.Prefix); err != nil {
				return err
			}
		}
	}
	return nil
}

func (vermouth *Vermouth) removeHttpProxyHost(body *httpProxyHostBody) error {
	switch vermouth.Mode {
	case "stand":
		proxy, ok := vermouth.Ctx.HttpReverseProxyList[body.Port]
		if !ok {
			return ErrHttpProxyNotExists
		}
		proxy.RemoveHost(body.Prefix, body.Host)
	case "cluster":
		if vermouth.Ctx.HttpReverseProxyList[body.Port].HostExists(body.Prefix, body.Host) {
			if err := vermouth.RaftNode.removeHttpProxyHost(body.Port, body.Prefix, body.Host); err != nil {
				return err
			}
		}

	}
	return nil
}

func (vermouth *Vermouth) addHttpProxyHost(body *httpProxyHostBody) error {
	switch vermouth.Mode {
	case "stand":
		proxy, ok := vermouth.Ctx.HttpReverseProxyList[body.Port]
		if !ok {
			return ErrHttpProxyNotExists
		}
		proxy.AddHost(body.Prefix, body.Host)
	case "cluster":
		if !vermouth.Ctx.HttpReverseProxyList[body.Port].HostExists(body.Prefix, body.Host) {
			if err := vermouth.RaftNode.addHttpProxyHost(body); err != nil {
				return err
			}
		}

	}
	return nil
}

func (vermouth *Vermouth) addBlackList(body *httpBlackListBody) error {
	switch vermouth.Mode {
	case "stand":
		proxy, ok := vermouth.Ctx.HttpReverseProxyList[body.Port]
		if !ok {
			return ErrHttpProxyNotExists
		}
		proxy.AddBlackList(body.Ips)
	case "cluster":
		if err := vermouth.RaftNode.addBlackList(body.Port, body.Ips); err != nil {
			return err
		}
	}
	return nil
}

func (vermouth *Vermouth) removeBlackList(body *httpBlackListBody) error {
	switch vermouth.Mode {
	case "stand":
		proxy, ok := vermouth.Ctx.HttpReverseProxyList[body.Port]
		if !ok {
			return ErrHttpProxyNotExists
		}
		proxy.RemoveBlackList(body.Ips)
	case "cluster":
		if err := vermouth.RaftNode.removeBlackList(body.Port, body.Ips); err != nil {
			return err
		}
	}
	return nil
}

func (vermouth *Vermouth) setRateLimiter(body *httpRateLimiterBody) error {
	switch vermouth.Mode {
	case "stand":
		proxy, ok := vermouth.Ctx.HttpReverseProxyList[body.Port]
		if !ok {
			return ErrHttpProxyNotExists
		}
		proxy.SetRateLimit(body.Prefix, body.Url, body.Type, body.Volumn, body.Speed)
	case "cluster":
		if err := vermouth.RaftNode.setRateLimiter(body); err != nil {
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
	case "stand":
		proxy :=  NewHttpReverseProxy(config.HttpProxyArgs{
			Port:         body.Port,
			PrefixerType: body.PrefixType,
			Paths:        []config.HttpProxyPathArgs{},
		})
		vermouth.Ctx.AddHttpReverseProxy(proxy)
		go proxy.Run()
	case "cluster":
		if err := vermouth.RaftNode.addHttpProxy(body); err != nil {
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
	case "stand":
		proxy.changeStatic(body.Prefix)
	case "cluster":
		if err := vermouth.RaftNode.changeStatic(body); err != nil {
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
