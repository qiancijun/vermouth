package vermouth

import (
	"bytes"
	"encoding/gob"

	"github.com/qiancijun/vermouth/config"
	"github.com/qiancijun/vermouth/logger"
)

func decode(data []byte, dest interface{}) error {
	buf := bytes.NewBuffer(data)
	dec := gob.NewDecoder(buf)
	if err := dec.Decode(dest); err != nil {
		return err
	}
	return nil
}

func encode(obj interface{}) ([]byte, error) {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	if err := enc.Encode(obj); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func handleAddHttpProxy(data []byte, ctx *StateContext) error {
	logger.Debug("receive new log entry, handleaddHttpProxy")
	body := &addHttpProxyBody{}
	if err := decode(data, body); err != nil {
		return err
	}
	newHttpProxy := NewHttpReverseProxy(config.HttpProxyArgs{
		PrefixerType: body.PrefixType,
		Port:         body.Port,
	})
	ctx.AddHttpReverseProxy(newHttpProxy)
	go newHttpProxy.Run()
	return nil
}

func handleAddHttpProxyPrefix(data []byte, ctx *StateContext) error {
	logger.Debug("receive new log entry, handleAddHttpProxyPrefix")
	body := &addHttpProxyPrefixBody{}
	if err := decode(data, body); err != nil {
		return err
	}
	hp := ctx.HttpReverseProxyList[body.Port]
	hp.AddPrefix(body.Prefix, body.BalanceType, body.ProxyPass, body.Static)
	return nil
}

func handleAddHttpHost(data []byte, ctx *StateContext) error {
	logger.Debug("receive new log entry, handleAddHttpHost")
	body := &httpProxyHostBody{}
	if err := decode(data, body); err != nil {
		return err
	}
	hp := ctx.HttpReverseProxyList[body.Port]
	hp.AddHost(body.Prefix, body.Host)
	return nil
}

func handleAddBlackList(data []byte, ctx *StateContext) error {
	body := &httpBlackListBody{}
	if err := decode(data, body); err != nil {
		return err
	}
	logger.Debugf("receive new log entry, handleAddBlackList, {port: %d}", body.Port)
	hp := ctx.HttpReverseProxyList[body.Port]
	hp.AddBlackList(body.Ips)
	return nil
}

func handleRemoveHttpProxyPrefix(data []byte, ctx *StateContext) error {
	logger.Debug("receive new log entry, handleRemoveHttpProxyPrefix")
	body := &removeHttpProxyPrefixBody{}
	if err := decode(data, body); err != nil {
		return err
	}
	hp := ctx.HttpReverseProxyList[body.Port]
	hp.RemovePrefix(body.Prefix)
	return nil
}

func handleRemoveHttpProxyHost(data []byte, ctx *StateContext) error {
	logger.Debug("receive new log entry, handleRemoveHttpProxyHost")
	body := &httpProxyHostBody{}
	if err := decode(data, body); err != nil {
		return err
	}
	hp := ctx.HttpReverseProxyList[body.Port]
	hp.RemoveHost(body.Prefix, body.Host)
	return nil
}

func handleRemoveHttpBlackList(data []byte, ctx *StateContext) error {
	logger.Debug("receive new log entry, handleRemoveHttpBlackList")
	body := &httpBlackListBody{}
	if err := decode(data, body); err != nil {
		return err
	}
	hp := ctx.HttpReverseProxyList[body.Port]
	hp.RemoveBlackList(body.Ips)
	return nil
}

func handleRemoveHttpProxy(data []byte, ctx *StateContext) error {
	logger.Debug("receive new log entry, handleRemoveHttpProxy")
	body := &removeHttpProxyBody{}
	if err := decode(data, body); err != nil {
		return err
	}
	hp := ctx.HttpReverseProxyList[body.Port]
	hp.Close()
	delete(ctx.HttpReverseProxyList, body.Port)
	return nil
}

func handleSetRateLimiter(data []byte, ctx *StateContext) error {
	body := &httpRateLimiterBody{}
	if err := decode(data, body); err != nil {
		return err
	}
	hp := ctx.HttpReverseProxyList[body.Port]
	hp.SetRateLimit(body.Prefix, body.Url, body.Type, body.Volumn, body.Speed)
	return nil
}

func handleChangeStatic(data []byte, ctx *StateContext) error {
	body := &httpChangeStatic{}
	if err := decode(data, body); err != nil {
		return err
	}
	hp := ctx.HttpReverseProxyList[body.Port]
	hp.changeStatic(body.Prefix)
	return nil
}
