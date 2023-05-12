package vermouth

import (
	"encoding/binary"
	"errors"
	"io"
	"sync"

	"github.com/hashicorp/raft"
	jsoniter "github.com/json-iterator/go"
	"github.com/qiancijun/vermouth/config"
	"github.com/qiancijun/vermouth/discovery"
	"github.com/qiancijun/vermouth/logger"
)

const (
	ADD_HTTP_PROXY        uint16 = 0x0001
	ADD_HTTP_PROXY_PREFIX uint16 = 0x0002
	ADD_HTTP_PROXY_HOST   uint16 = 0x0003
	ADD_HTTP_BLACK_LIST   uint16 = 0x0004
	RMV_HTTP_PROXY_PREFIX uint16 = 0x0005
	RMV_HTTP_PROXY_HOST   uint16 = 0x0006
	RMV_HTTP_BLACK_LIST   uint16 = 0x0007
	RMV_HTTP_PROXY        uint16 = 0x0008
	SET_RATE_LIMITER      uint16 = 0x0009
	CHG_STATIC            uint16 = 0x0010
)

var (
	ErrPortAlreadyBeenUsed = errors.New("port has been used")
	optionTypeString       = map[uint16]string{
		ADD_HTTP_PROXY:        "add-http-proxy",
		ADD_HTTP_PROXY_PREFIX: "add-http-proxy-prefix",
		ADD_HTTP_PROXY_HOST:   "add-http-proxy-host",
		ADD_HTTP_BLACK_LIST:   "add-http-black-list",
		RMV_HTTP_PROXY_PREFIX: "remove-http-proxy-prefix",
		RMV_HTTP_PROXY_HOST:   "remove-http-proxy-host",
		RMV_HTTP_BLACK_LIST:   "remove-http-black-list",
		RMV_HTTP_PROXY:        "remove-http-proxy",
		SET_RATE_LIMITER:      "set-rate-limiter",
		CHG_STATIC:            "change-static",
	}
)

// 状态管理对象，相当于内存数据库
type StateContext struct {
	sync.Mutex `json:"-"`
	Raft       *raft.Raft `json:"-"`
	// 一个端口对应一个 HTTP 代理服务
	HttpReverseProxyList map[int64]*HttpReverseProxy

	discoveryMap   map[int64]discovery.Discovery
	discoverNotify map[int64]chan discovery.Event
}

func NewStateContext() *StateContext {
	ctx := &StateContext{
		HttpReverseProxyList: make(map[int64]*HttpReverseProxy),
		discoverNotify:       make(map[int64]chan discovery.Event),
		discoveryMap:         make(map[int64]discovery.Discovery),
	}
	return ctx
}

func (ctx *StateContext) Apply(logEntry *raft.Log) interface{} {
	if logEntry == nil {
		return nil
	}

	bidata := logEntry.Data
	opt := binary.BigEndian.Uint16(bidata)
	bidata = bidata[4:]

	logger.Debugf("%s receive a new logEntry, option Type: %s", ctx.Raft.String(), optionTypeString[opt])

	// 原则上这里应该使用责任链
	var ret interface{}
	switch opt {
	case ADD_HTTP_PROXY:
		ret = handleAddHttpProxy(bidata, ctx)
	case ADD_HTTP_PROXY_PREFIX:
		ret = handleAddHttpProxyPrefix(bidata, ctx)
	case ADD_HTTP_PROXY_HOST:
		ret = handleAddHttpHost(bidata, ctx)
	case ADD_HTTP_BLACK_LIST:
		ret = handleAddBlackList(bidata, ctx)
	case RMV_HTTP_PROXY_PREFIX:
		ret = handleRemoveHttpProxyPrefix(bidata, ctx)
	case RMV_HTTP_PROXY_HOST:
		ret = handleRemoveHttpProxyHost(bidata, ctx)
	case RMV_HTTP_BLACK_LIST:
		ret = handleRemoveHttpBlackList(bidata, ctx)
	case RMV_HTTP_PROXY:
		ret = handleRemoveHttpProxy(bidata, ctx)
	case SET_RATE_LIMITER:
		ret = handleSetRateLimiter(bidata, ctx)
	case CHG_STATIC:
		ret = handleChangeStatic(bidata, ctx)
	}

	_, v := ret.(error)
	if v {
		logger.Warnf("option %d has error %v", opt, ret)
	}
	return ret
}

func (ctx *StateContext) Snapshot() (raft.FSMSnapshot, error) {
	return ctx, nil
}

// ========== 对 raft 包快照接口的实现 ==========
func (ctx *StateContext) Persist(sink raft.SnapshotSink) error {
	snapshotBytes, err := ctx.Marshal()
	if err != nil {
		sink.Cancel()
		return err
	}
	if _, err := sink.Write(snapshotBytes); err != nil {
		sink.Cancel()
		return err
	}
	if err := sink.Close(); err != nil {
		sink.Cancel()
		return err
	}
	return nil
}

func (ctx *StateContext) Release() {}

// ========== 对 raft 包中 fsm 机制接口的实现 ==========
func (ctx *StateContext) Restore(serialized io.ReadCloser) error {
	logger.Debug("find snapshot file successful, ready to restore old data")
	if err := ctx.UnMarshal(serialized); err != nil {
		logger.Errorf("can't restore data, maybe snapshot file is break! %v", err)
		return err
	}
	// 成功从快照中读取数据，acl 表需要重新添加会数据结构里
	// 同时启动每一个代理服务
	for _, hp := range ctx.HttpReverseProxyList {
		rxTree := hp.Acl
		for k := range rxTree.Record {
			rxTree.Add(k, k)
		}
		if err := hp.Restore(); err != nil {
			logger.Errorf("can't restore http reverse proxy from context, this is internal error, it's a bug! %v", err)
			continue
		}
		go hp.Start()
	}
	return nil
}

// ========== 序列化与反序列化 ==========
func (ctx *StateContext) Marshal() ([]byte, error) {
	bytes, err := jsoniter.Marshal(ctx)
	return bytes, err
}

func (ctx *StateContext) UnMarshal(serialized io.ReadCloser) error {
	if err := jsoniter.NewDecoder(serialized).Decode(&ctx); err != nil {
		return err
	}
	return nil
}

// ========== 对 Context 的修改 ==========
func (ctx *StateContext) AddHttpReverseProxy(httpProxys ...*HttpReverseProxy) error {
	ctx.Lock()
	defer ctx.Unlock()
	for _, httpProxy := range httpProxys {
		port := httpProxy.Port
		_, ok := ctx.HttpReverseProxyList[port]
		if ok {
			logger.Errorf("%d %v", port, ErrPortAlreadyBeenUsed)
			continue
		}
		ctx.HttpReverseProxyList[port] = httpProxy
	}
	return nil
}

func (ctx *StateContext) Run() {
	// 启动 http 反向代理
	wg := new(sync.WaitGroup)
	logger.Infof("find %d http reverse proxy", len(ctx.HttpReverseProxyList))
	for _, httpProxy := range ctx.HttpReverseProxyList {
		wg.Add(1)
		go func(httpProxy *HttpReverseProxy, wg *sync.WaitGroup) {
			defer wg.Done()
			if err := httpProxy.Run(); err != nil {
				logger.Errorf("can't listen at Port %d, %s", httpProxy.Port, err.Error())
				return
			}
			logger.Debugf("%d start success", httpProxy.Port)
		}(httpProxy, wg)
	}
	wg.Wait()
	logger.Info("start all http reverse proxy success")
}

// TODO 没有写停止
func (ctx *StateContext) startDiscovery() {
	logger.Info("start server discovery")
	for _, dis := range ctx.discoveryMap {
		go dis.Run()
	}
	for port, notify := range ctx.discoverNotify {
		go vermouth.HandleNamespaceEvent(notify, port)
	}
}

func (ctx *StateContext) buildDiscovery(args []config.Discovery) {
	for _, dis := range args {
		// ctx.checkAndCreate(dis.Port)
		notify := make(chan discovery.Event)
		namespaceServer, err := discovery.Build(dis.Namespace, notify, dis)
		if err != nil {
			logger.Errorf("can't build %s service discovery, %s", dis.Namespace, err.Error())
			continue
		}
		ctx.discoverNotify[dis.Port] = notify
		ctx.discoveryMap[dis.Port] = namespaceServer
	}
}

func (ctx *StateContext) checkAndCreate(port int64) {
	if _, ok := ctx.HttpReverseProxyList[port]; !ok {
		httpProxy := NewHttpReverseProxy(config.HttpProxyArgs{
			PrefixerType: "hash-prefixer",
			Port:         port,
		})
		ctx.AddHttpReverseProxy(httpProxy)
	}
}
