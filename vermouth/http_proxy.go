package vermouth

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"strconv"
	"strings"
	"sync"
	"time"

	acl "github.com/qiancijun/vermouth/acl"
	"github.com/qiancijun/vermouth/balancer"
	"github.com/qiancijun/vermouth/config"
	"github.com/qiancijun/vermouth/discovery"
	"github.com/qiancijun/vermouth/logger"
	"github.com/qiancijun/vermouth/prefix"
	rate "github.com/qiancijun/vermouth/rate_limiter"
	"github.com/qiancijun/vermouth/utils"
)

const (
	OriginHostReqHeader    = "X-Origin-Host"
	ForwardedHostReqHeader = "X-Forwarded-Host"
	ProxyServerPathError   = "illegal proxy path, internal server error"
	IllegalPortError       = "illegal server Port, please ensure it's integer"
	ForbiddenAccessSystem  = "You're forbidden to access the system"
)

// 存储一些多余的信息，Prefixer 是一个接口类型，序列化存储的时候会出现 nil 异常
// 因为保存的是某一个特定的 prefixer 类型
// https 实现 https://www.nuomiphp.com/t/63104607aae2291c0f2d196d.html
type HttpReverseProxy struct {
	sync.RWMutex
	Port            int64
	PrefixerType    string
	Paths           map[string][]string
	BalancerType    map[string]string
	RateLimiterType map[string][]*rate.LimiterInfo
	StaticMapping   map[string]bool // 静态资源表
	Acl             *acl.RadixTree  // 访问控制列表
	Available       bool            // 对整个代理启停的控制
	// TODO 对每个 prefix 更细粒度的启停控制
	rateLimiters  map[string]rate.RateLimiter // 前缀限流表
	prefixMapping prefix.Prefixer             // 前缀映射表
	eventChan     chan discovery.Event

	reverseproxyPool sync.Pool
	Shutdown         chan struct{} `json:"-"`
}

// 创建一个空的代理器
func NewHttpReverseProxy(args config.HttpProxyArgs) *HttpReverseProxy {
	p, err := prefix.Build(args.PrefixerType)
	if err != nil {
		logger.Fatal("illgeal prefixer type")
	}
	reverseProxy := &HttpReverseProxy{
		Port:            args.Port,
		PrefixerType:    args.PrefixerType,
		BalancerType:    make(map[string]string),
		Paths:           make(map[string][]string),
		RateLimiterType: make(map[string][]*rate.LimiterInfo),
		StaticMapping:   make(map[string]bool),
		Acl:             acl.NewRadixTree(),
		Available:       true,
		rateLimiters:    make(map[string]rate.RateLimiter),
		prefixMapping:   p,
		reverseproxyPool: sync.Pool{
			New: func() any {
				return &httputil.ReverseProxy{
					ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
						if err != nil {
							w.Write([]byte(err.Error()))
						}
					},
				}
			},
		},
		Shutdown: make(chan struct{}),
	}
	return reverseProxy
}

func BuildHttpReverseProxyFromConfig(conf []config.HttpProxyArgs) []*HttpReverseProxy {
	list := make([]*HttpReverseProxy, 0)
	for _, proxyArg := range conf {
		paths := proxyArg.Paths

		proxy := NewHttpReverseProxy(proxyArg)
		for _, path := range paths {
			balancerMode, pattern, hosts, isStatic := path.BalanceMode, path.Pattern, path.Hosts, path.Static
			proxy.AddPrefix(pattern, balancerMode, hosts, isStatic)
			// TODO 存储信息抽取出来
			proxy.BalancerType[pattern] = balancerMode
		}
		list = append(list, proxy)
	}
	return list
}

func (proxy *HttpReverseProxy) Run() error {
	proxy.Lock()
	defer proxy.Unlock()
	listen, err := net.Listen("tcp", fmt.Sprintf(":%d", proxy.Port))
	if err != nil {
		return err
	}
	logger.Infof("http reverse proxy listen at %d port", proxy.Port)
	// go http.Serve(listen, proxy)
	hs := &http.Server{
		Handler: proxy,
	}
	go hs.Serve(listen)
	go func(proxy *HttpReverseProxy) {
		<-proxy.Shutdown
		logger.Debugf("[httpProxy:%d] receive shutdown command", proxy.Port)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		hs.Shutdown(ctx)
		delete(vermouth.Ctx.HttpReverseProxyList, proxy.Port)
	}(proxy)

	return nil
}

func (proxy *HttpReverseProxy) Close() {
	proxy.Shutdown <- struct{}{}
}

func (proxy *HttpReverseProxy) Stop() {
	proxy.Lock()
	defer proxy.Unlock()
	proxy.Available = false
	logger.Infof("stop http reverse proxy at port %d success", proxy.Port)
}

func (proxy *HttpReverseProxy) Start() {
	proxy.Lock()
	defer proxy.Unlock()
	proxy.Available = true
	logger.Infof("start http reverse proxy at port %d success", proxy.Port)
}

// 在读取完快照后使用，读取快照只是读取配置信息，没有真正配置
// 所以读取完快照后代理服务是信息不全的
func (proxy *HttpReverseProxy) Restore() error {
	p, err := prefix.Build(proxy.PrefixerType)
	if err != nil {
		return err
	}
	proxy.prefixMapping = p
	paths := proxy.Paths
	proxy.Paths = make(map[string][]string)
	for k, v := range paths {
		proxy.AddPrefix(k, proxy.BalancerType[k], v, proxy.StaticMapping[k])
	}

	// 恢复 acl
	ips := proxy.Acl.Record
	blackList := acl.NewRadixTree()
	for ip := range ips {
		blackList.Add(ip, ip)
	}
	proxy.Acl = blackList
	// 恢复限流器
	proxy.rateLimiters = make(map[string]rate.RateLimiter)
	for prefix, infos := range proxy.RateLimiterType {
		for _, v := range infos {
			proxy.createRateLimiter(prefix, v.Url, v.Type, v.Init, v.Speed)
		}
	}
	return nil
}

// 流程：
// 1. 判断是否被限制访问
// 2. 根据前缀动态解析地址(PrefixMapping)
// 3. 限流判断
// 4. 访问真实请求地址
func (proxy *HttpReverseProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// proxy.RLock()
	// defer proxy.RUnlock()
	logger.Debugf("[httpProxy:%d] receive a new http request %s", proxy.Port, r.URL)
	if !proxy.Available || !VermouthPeer().Available() {
		logger.Debugf("[httpProxy:%d] is not available", proxy.Port)
		w.WriteHeader(http.StatusNotFound)
		return
	}
	// TODO 如果是域名，解析成 ip
	// 第一步
	forbidden := proxy.Acl.AccessControl(utils.RemoteIp(r))
	if forbidden {
		// 禁止访问
		logger.Debugf("[httpProxy:%d] forbid to access", proxy.Port)
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(ForbiddenAccessSystem))
		return
	}
	// 第二步
	prefix, apiPath, realPath, err := proxy.prefixMapping.MappingPath(r.URL.Path)
	if err != nil {
		logger.Debugf("[httpProxy:%d] user try to access unknown proxy path", proxy.Port)
		w.Write([]byte(err.Error()))
		return
	}
	logger.Debugf("[httpProxy:%d] find prefix[%s] apiPath[%s] and realPath[%s]", proxy.Port, prefix, apiPath, realPath)

	// 第三步
	// compeletePath 是包含了代理匹配前缀和真实路径的url
	var compeletePath string
	if prefix == "/" {
		compeletePath = apiPath
	} else {
		compeletePath = prefix + apiPath
	}

	// 静态资源判断
	if !proxy.isStaticResource(prefix) {
		limiter, ok := proxy.rateLimiters[compeletePath]
		if !ok {
			logger.Debugf("[httpProxy:%d] urlPath: %s has no rate limiter", proxy.Port, compeletePath)
			// 还没有为该代理地址配置限流器
			limiter, err = proxy.createRateLimiter(prefix, apiPath, "qps", rate.INF, rate.INF)
			if err != nil {
				// 创建不出默认 qps 限流器，系统级别错误
				logger.Fatalf("[httpProxy:%d]  can't create rate limiter, %s", proxy.Port, err.Error())
			}
		}
		// TODO 从配置文件中读取超时时间
		err = limiter.Take()
		if err != nil {
			w.Write([]byte(err.Error()))
			logger.Errorf("[httpProxy:%d] can't take token from rateLimiter, %s", proxy.Port, err.Error())
			return
		}
	}
	// 第四步
	proxy.forwardToSpecficPath(w, r, realPath)
}

func (proxy *HttpReverseProxy) AddPrefix(prefix, algo string, hosts []string, isStatic bool) {
	proxy.Lock()
	defer proxy.Unlock()
	// proxy.Paths[prefixPath] = append(proxy.Paths[prefixPath], hosts...)
	logger.Debugf("[httpProxy:%d] add prefix will add {prefix: %s, algo: %s, hosts: %v}", proxy.Port, prefix, algo, hosts)
	m := proxy.prefixMapping.List()
	if _, ok := m[prefix]; ok {
		return
	}
	proxy.StaticMapping[prefix] = isStatic
	proxy.prefixMapping.Add(prefix, balancer.Algorithm(algo), hosts)
	proxy.BalancerType[prefix] = algo
	proxy.Paths[prefix] = proxy.prefixMapping.GetBalancer(prefix).Hosts()
}

func (proxy *HttpReverseProxy) RemovePrefix(prefix string) {
	proxy.Lock()
	defer proxy.Unlock()
	delete(proxy.Paths, prefix)
	delete(proxy.BalancerType, prefix)
	delete(proxy.rateLimiters, prefix)
	delete(proxy.RateLimiterType, prefix)
	proxy.prefixMapping.Remove(prefix)
}

func (proxy *HttpReverseProxy) GetHosts(prefix string) []string {
	proxy.RLock()
	defer proxy.RUnlock()
	return proxy.Paths[prefix]
}

// TODO prefix 不存在时自动创建一个
func (proxy *HttpReverseProxy) AddHost(prefix, host string) {
	proxy.Lock()
	defer proxy.Unlock()
	proxy.prefixMapping.GetBalancer(prefix).Add(host)
	proxy.Paths[prefix] = proxy.prefixMapping.GetBalancer(prefix).Hosts()
}

func (proxy *HttpReverseProxy) RemoveHost(prefix, host string) {
	proxy.Lock()
	defer proxy.Unlock()
	proxy.prefixMapping.GetBalancer(prefix).Remove(host)
	proxy.Paths[prefix] = proxy.prefixMapping.GetBalancer(prefix).Hosts()
}

func (proxy *HttpReverseProxy) PrefixExists(prefix string) bool {
	_, ok := proxy.Paths[prefix]
	return ok
}

func (proxy *HttpReverseProxy) HostExists(prefix, host string) bool {
	hosts := proxy.Paths[prefix]
	for _, v := range hosts {
		if host == v {
			return true
		}
	}
	return false
}

// forwardToSpecficPath 只负责将请求转发，不关注请求的地址拼接
// 确保传入 forwardToSpecficPath 的路径是 ip:[端口]/地址，
func (proxy *HttpReverseProxy) forwardToSpecficPath(
	w http.ResponseWriter,
	r *http.Request,
	rawPath string) {
	index := strings.Index(rawPath, "/")
	if index == -1 {
		index = len(rawPath)
		// w.Write([]byte(ProxyServerPathError))
		// return
	}
	var host string
	var Port int64
	var err error
	apiUrl := rawPath[index:]
	serverAddr := rawPath[:index]
	split := strings.Split(serverAddr, ":")
	if len(split) > 0 {
		host = split[0]
		if len(split) == 1 {
			Port = 80
		} else {
			Port, err = strconv.ParseInt(split[1], 10, 64)
			if err != nil {
				w.Write([]byte(IllegalPortError))
				return
			}
		}
	}
	// logger.Debugf("[httpProxy:%d] apiUrl: %s serverAddr: %s host: %s Port: %d", proxy.Port, apiUrl, serverAddr, host, Port)
	director := func(req *http.Request) {
		req.Header.Add(ForwardedHostReqHeader, req.Host)
		req.Header.Add(OriginHostReqHeader, host)
		req.URL.Scheme = "http"
		req.URL.Host = fmt.Sprintf("%s:%d", host, Port)
		req.URL.Path = apiUrl
		req.Method = r.Method
	}

	// errHandler := func(w http.ResponseWriter, r *http.Request, err error) {
	// 	if err != nil {
	// 		w.Write([]byte(err.Error()))
	// 	}
	// }

	// resHandler := func(res *http.Response) error {
	// 	// logger.Info("http reverse proxy success: ", res)
	// 	return nil
	// }

	// TODO 对象复用
	reverseproxy := &httputil.ReverseProxy{
		Director: director,
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			if err != nil {
				w.Write([]byte(err.Error()))
			}
		},
	}
	// reverseproxy := proxy.reverseproxyPool.Get().(*httputil.ReverseProxy)
	reverseproxy.Director = director
	reverseproxy.Transport = &http.Transport{ResponseHeaderTimeout: 5000 * time.Millisecond}
	reverseproxy.ServeHTTP(w, r)
	// proxy.reverseproxyPool.Put(reverseproxy)
}

// === 对 Acl 方法提供访问 ===
func (proxy *HttpReverseProxy) AddBlackList(ips []string) {
	proxy.Lock()
	defer proxy.Unlock()
	for _, ip := range ips {
		if err := proxy.Acl.Add(ip, ip); err != nil {
			logger.Errorf("[httpProxy:%d] can't add black ip, %s", proxy.Port, err.Error())
		}
	}
}
func (proxy *HttpReverseProxy) RemoveBlackList(ips []string) {
	proxy.Lock()
	defer proxy.Unlock()
	for _, ip := range ips {
		if err := proxy.Acl.Delete(ip); err != nil {
			logger.Errorf("[httpProxy:%d] can't remove black ip, %s", proxy.Port, err.Error())
		}
	}
}

// === 对限流器提供方法访问 ===
func (proxy *HttpReverseProxy) SetRateLimit(prefix, apiPath, limiterType string, init int, speed int64) {
	proxy.Lock()
	defer proxy.Unlock()
	compeleteUrl := prefix + apiPath
	limiter, ok := proxy.rateLimiters[compeleteUrl]
	if !ok {
		logger.Debugf("[httpProxy:%d] not found %s rate limiter, create a qps limiter", proxy.Port, compeleteUrl)
		// 直接创建
		_, err := proxy.createRateLimiter(prefix, apiPath, limiterType, init, speed)
		if err != nil {
			logger.Errorf("[httpProxy:%d] can't create rate limiter, %s", proxy.Port, err.Error())
		}
		return
	}
	// O(n) 时间复杂度对配置信息进行修改
	for _, info := range proxy.RateLimiterType[prefix] {
		if info.Url == apiPath {
			info.Init, info.Speed = init, speed
			break
		}
	}
	limiter.SetRate(init, speed)
	logger.Infof("[httpProxy:%d] success set rate for path %s, {init: %d, speed: %d}", proxy.Port, compeleteUrl, init, speed)
}

func (proxy *HttpReverseProxy) createRateLimiter(prefix, apiPath, limiterType string, init int, speed int64) (rate.RateLimiter, error) {
	proxy.Lock()
	defer proxy.Unlock()
	var compeleteUrl string
	if prefix == "/" {
		compeleteUrl = apiPath
	} else {
		compeleteUrl = prefix + apiPath
	}
	limiter, err := rate.Build(rate.LimiterType(limiterType))
	if err != nil {
		return nil, err
	}
	proxy.rateLimiters[compeleteUrl] = limiter
	proxy.RateLimiterType[prefix] = append(proxy.RateLimiterType[prefix], &rate.LimiterInfo{
		Url:   apiPath,
		Type:  limiterType,
		Init:  init,
		Speed: speed,
	})
	limiter.SetRate(init, speed)
	logger.Infof("[httpProxy:%d] create rate limiter for path: %s", proxy.Port, compeleteUrl)
	return limiter, nil
}

func (proxy *HttpReverseProxy) checkPrefixExists(prefix string) bool {
	proxy.Lock()
	defer proxy.Unlock()
	_, ok := proxy.Paths[prefix]
	return ok
} 

func (proxy *HttpReverseProxy) changeStatic(prefix string) {
	proxy.Lock()
	defer proxy.Unlock()
	old := proxy.StaticMapping[prefix]
	proxy.StaticMapping[prefix] = !old
}

// TODO channel 统一处理
func (proxy *HttpReverseProxy) HandleNamespaceEvent() {
	for {
		select {
		case e := <-proxy.eventChan:
			logger.Debugf("[httpProxy] receive new Event: %+v", e)
			switch e.Opt {
			case discovery.ADD_HOST:
				proxy.AddHost(e.ServiceName, e.Host)
			case discovery.REMOVE_HOST:
				proxy.RemoveHost(e.ServiceName, e.Host)
			case discovery.ADD_SERVICE:
				proxy.AddPrefix(e.ServiceName, "round-robin", []string{}, false)
			case discovery.REMOVE_SERVICE:
				proxy.RemovePrefix(e.ServiceName)
			case discovery.REMOVE_HOSTS:
				visit := make(map[string]bool)
				for _, v := range e.Children {
					visit[v] = true
				}
				for _, v := range proxy.GetHosts(e.ServiceName) {
					if _, ok := visit[v]; !ok {
						proxy.RemoveHost(e.ServiceName, v)
					}
				}
			}
		}
	}
}

func (proxy *HttpReverseProxy) isStaticResource(prefix string) bool {
	return proxy.StaticMapping[prefix]
}
