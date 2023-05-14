package vermouth

import (
	"fmt"
	"net/http"
	"strconv"
	"sync/atomic"

	"github.com/elastic/go-sysinfo"
	"github.com/hashicorp/raft"
	"github.com/qiancijun/vermouth/balancer"
	"github.com/qiancijun/vermouth/logger"
	"github.com/qiancijun/vermouth/utils"
	"github.com/spf13/viper"
)

const (
	InternalErrorMsg   = "Internal error please check log"
	ENABLE_WRITE_TRUE  = int32(1)
	ENABLE_WRITE_FALSE = int32(0)
)

type HttpServer struct {
	port        int64
	Mux         *http.ServeMux
	Ctx         *StateContext
	enableWrite int32
}

func NewHttpServer(port int64) *HttpServer {
	httpServer := &HttpServer{
		Mux:         http.NewServeMux(),
		enableWrite: ENABLE_WRITE_FALSE,
		port:        port,
	}
	httpServer.addHandler("/join", httpServer.doJoin)
	httpServer.addHandler("/info", httpServer.doGetInfo)
	httpServer.addHandler("/http/add/proxy", httpServer.doAddProxy)
	httpServer.addHandler("/http_proxy_info", httpServer.doGetHttpProxy)
	httpServer.addHandler("/http/change", httpServer.doChangeHttpProxyState)
	httpServer.addHandler("/http/add/prefix", httpServer.doAddProxyPrefix)
	httpServer.addHandler("/http/remove/prefix", httpServer.doRemoveProxyPrefix)
	httpServer.addHandler("/http/remove/host", httpServer.doRemoveHost)
	httpServer.addHandler("/http/add/host", httpServer.doAddHost)
	httpServer.addHandler("/http/add/black-list", httpServer.doAddBlackList)
	httpServer.addHandler("/http/remove/black-list", httpServer.doRemoveBlackList)
	httpServer.addHandler("/http/proxy", httpServer.getHttpProxyInfo)
	httpServer.addHandler("/http/set/rate", httpServer.doSetRateLimiter)
	httpServer.addHandler("/http/change/static", httpServer.doChangeStatic)
	httpServer.addHandler("/http/change/lb", httpServer.doChangeLb)
	httpServer.addHandler("/http/lb/list", httpServer.doGetBalancerType)
	httpServer.addHandler("/meminfo", httpServer.getMeminfo)
	httpServer.addHandler("/sysinfo", httpServer.getSysinfo)
	httpServer.Mux.Handle("/", http.FileServer(http.Dir("../static")))

	return httpServer
}

func (s *HttpServer) Start(mode string) {
	if s == nil {
		logger.Debug("http server don't need to start")
		return
	}
	l, err := utils.CreateListener(s.port)
	if err != nil {
		logger.Errorf("can't start http server, %s", err.Error())
	}
	logger.Infof("http server listen at port %d", s.port)
	if mode == "stand" {
		s.SetWriteFlag(true)
	}
	go http.Serve(l, s.Mux)
}

func (s *HttpServer) addHandler(path string, fun func(w http.ResponseWriter, r *http.Request)) {
	s.Mux.HandleFunc(path, fun)
}

func (h *HttpServer) SetWriteFlag(flag bool) {
	if flag {
		atomic.StoreInt32(&h.enableWrite, ENABLE_WRITE_TRUE)
	} else {
		atomic.StoreInt32(&h.enableWrite, ENABLE_WRITE_FALSE)
	}
}

func (h *HttpServer) checkWritePermission() bool {
	return atomic.LoadInt32(&h.enableWrite) == ENABLE_WRITE_TRUE
}

func (s *HttpServer) doJoin(w http.ResponseWriter, r *http.Request) {
	variables := r.URL.Query()
	peerAddress := variables.Get("tcpAddress")
	name := variables.Get("name")
	if peerAddress == "" {
		errMsg := "doJoin: invaild peerAddress"
		logger.Error(errMsg)
		fmt.Fprint(w, errMsg)
		return
	}
	logger.Debugf("peerAddress %s will join the cluster", peerAddress)
	addPeerFuture := s.Ctx.Raft.AddVoter(raft.ServerID(name), raft.ServerAddress(peerAddress), 0, 0)
	if err := addPeerFuture.Error(); err != nil {
		errMsg := fmt.Sprintf("Error joining peer to raft, peeraddress:%s, err:%v, code:%d", peerAddress, err, http.StatusInternalServerError)
		logger.Warn(errMsg)
		fmt.Fprint(w, errMsg)
		return
	}
	fmt.Fprint(w, "ok")
}

func (h *HttpServer) doAddProxy(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if !h.checkWritePermission() {
		w.Write([]byte("Not Leader"))
		return
	}
	body := &addHttpProxyBody{}
	err := utils.ReadFromRequest(r, body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		logger.Error(err.Error())
		return
	}
	if err := vermouth.addHttpProxy(body); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		logger.Error(err.Error())
		return
	}
	logger.Info("[httpServer] handle add proxy command success")
	w.Write([]byte("success"))
}

func (s *HttpServer) doGetInfo(w http.ResponseWriter, r *http.Request) {
	infos := viper.AllSettings()

	// 获取所有 http 代理信息
	cnt := len(s.Ctx.HttpReverseProxyList)
	infos["http_proxy"] = cnt

	if vermouth.Mode == "cluster" {
		_, leaderId := s.Ctx.Raft.LeaderWithID()
		infos["leaderId"] = leaderId
		infos["role"] = s.Ctx.Raft.State().String()
	}

	data, err := utils.EncodeToJson(infos)
	if err != nil {
		w.Write([]byte(InternalErrorMsg))
		return
	}
	w.Write(data)
}

func (s *HttpServer) doGetHttpProxy(w http.ResponseWriter, r *http.Request) {
	list := s.Ctx.HttpReverseProxyList

	data, err := utils.EncodeToJson(list)
	if err != nil {
		w.Write([]byte(InternalErrorMsg))
		return
	}
	w.Write(data)
}

func (s *HttpServer) doChangeHttpProxyState(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if !s.checkWritePermission() {
		w.Write([]byte("Not Leader"))
		return
	}
	body := &changeHttpProxyStateBody{}
	err := utils.ReadFromRequest(r, body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		logger.Error(err.Error())
		return
	}
	// proxy := s.Ctx.HttpReverseProxyList[body.Port]
	if err := VermouthPeer().changeProxyAvailable(body); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		logger.Error(err.Error())
		return
	}
	// if body.State == 1 {
	// 	proxy.Start()
	// } else if body.State == 2 {
	// 	proxy.Stop()
	// } else {
	// 	proxy.Close()
	// }
	w.Write([]byte("success"))
}

func (s *HttpServer) doAddProxyPrefix(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if !s.checkWritePermission() {
		w.Write([]byte("Not Leader"))
		return
	}
	body := &addHttpProxyPrefixBody{}
	if err := utils.ReadFromRequest(r, body); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		logger.Error(err.Error())
		return
	}
	logger.Debug("[httpServer] receive add prefix command")
	if err := vermouth.addHttpProxyPrefix(body); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		logger.Error(err.Error())
		return
	}
	logger.Info("[httpServer] handle add preifx command success")
	w.Write([]byte("success"))
}

func (s *HttpServer) doRemoveProxyPrefix(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if !s.checkWritePermission() {
		w.Write([]byte("Not Leader"))
		return
	}
	body := &removeHttpProxyPrefixBody{}
	if err := utils.ReadFromRequest(r, body); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		logger.Error(err.Error())
		return
	}
	logger.Debug("[httpServer] receive add prefix command")
	if err := vermouth.removeHttpProxyPrefix(body); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		logger.Error(err.Error())
		return
	}
	logger.Info("[httpServer] handle remove preifx command success")
	w.Write([]byte("success"))
}

func (s *HttpServer) doAddHost(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if !s.checkWritePermission() {
		w.Write([]byte("Not Leader"))
		return
	}
	body := &httpProxyHostBody{}
	if err := utils.ReadFromRequest(r, body); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		logger.Error(err.Error())
		return
	}
	if err := VermouthPeer().addHttpProxyHost(body); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		logger.Error(err.Error())
		return
	}
	w.Write([]byte("success"))
}

func (s *HttpServer) doRemoveHost(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if !s.checkWritePermission() {
		w.Write([]byte("Not Leader"))
		return
	}
	body := &httpProxyHostBody{}
	if err := utils.ReadFromRequest(r, body); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		logger.Error(err.Error())
		return
	}
	logger.Debugf("[httpServer] remove host request data: {port: %d, prefix: %s, host: %s}", body.Port, body.Prefix, body.Host)
	if err := VermouthPeer().removeHttpProxyHost(body); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		logger.Error(err.Error())
		return
	}
	w.Write([]byte("success"))
}

func (s *HttpServer) doAddBlackList(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if !s.checkWritePermission() {
		w.Write([]byte("Not Leader"))
		return
	}
	body := &httpBlackListBody{}
	if err := utils.ReadFromRequest(r, body); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		logger.Error(err.Error())
		return
	}
	logger.Debugf("[httpServer] add black list request data: {port: %d, ips: %v}", body.Port, body.Ips)
	if err := VermouthPeer().addBlackList(body); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		logger.Error(err.Error())
		return
	}
	w.Write([]byte("success"))
}

func (s *HttpServer) doRemoveBlackList(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if !s.checkWritePermission() {
		w.Write([]byte("Not Leader"))
		return
	}
	body := &httpBlackListBody{}
	if err := utils.ReadFromRequest(r, body); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		logger.Error(err.Error())
		return
	}
	logger.Debugf("[httpServer] remove black list request data: {port: %d, ips: %v}", body.Port, body.Ips)
	if err := VermouthPeer().removeBlackList(body); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		logger.Error(err.Error())
		return
	}
	w.Write([]byte("success"))
}

func (h *HttpServer) doChangeStatic(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if !h.checkWritePermission() {
		w.Write([]byte("Not Leader"))
		return
	}
	body := &httpChangeStatic{}
	if err := utils.ReadFromRequest(r, body); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		logger.Error(err.Error())
		return
	}
	logger.Debugf("[httpServer] change static request data: {port: %d, prefix: %s}", body.Port, body.Prefix)
	if err := VermouthPeer().changeStatic(body); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		logger.Error(err.Error())
		return
	}
	w.Write([]byte("success"))
}

func (s *HttpServer) getHttpProxyInfo(w http.ResponseWriter, r *http.Request) {
	variables := r.URL.Query()
	port, err := strconv.Atoi(variables.Get("port"))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		logger.Errorf("[httpServer] get http reverse proxy info has error: %s", err.Error())
		return
	}
	proxy, ok := s.Ctx.HttpReverseProxyList[int64(port)]
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	data, err := utils.EncodeToJson(proxy)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		logger.Errorf("[httpServer] marshal http reverse proxy has error: %s", err.Error())
		return
	}
	logger.Info("[httpServer] handle get http reverse proxy info success")
	w.Write(data)
}

func (s *HttpServer) doSetRateLimiter(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if !s.checkWritePermission() {
		w.Write([]byte("Not Leader"))
		return
	}
	body := &httpRateLimiterBody{}
	if err := utils.ReadFromRequest(r, body); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		logger.Error(err.Error())
		return
	}
	logger.Debugf("[httpServer] set rate limiter request data: {%d %s %s %s %d %d}", body.Port, body.Prefix, body.Url, body.Type, body.Volumn, body.Speed)
	if err := VermouthPeer().setRateLimiter(body); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		logger.Error(err.Error())
		return
	}
	w.Write([]byte("success"))
}

func (s *HttpServer) doGetBalancerType(w http.ResponseWriter, r *http.Request) {
	lb := balancer.GetBalancerType()
	data, err := utils.EncodeToJson(lb)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Write(data)
}

func (s *HttpServer) doChangeLb(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if !s.checkWritePermission() {
		w.Write([]byte("Not Leader"))
		return
	}
	body := &httpChangeLbBody{}
	if err := utils.ReadFromRequest(r, body); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		logger.Error(err.Error())
		return
	}
	logger.Debugf("[httpServer] change load balance request data: {port: %d, prefix: %s, lb: %s}", body.Port, body.Prefix, body.Lb)
	if err := VermouthPeer().changeLb(body); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		logger.Error(err.Error())
		return
	}
	w.Write([]byte(Success))
}

func (s *HttpServer) getMeminfo(w http.ResponseWriter, r *http.Request) {
	host, err := sysinfo.Host()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	mem, err := host.Memory()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	data, err := utils.EncodeToJson(mem)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Write(data)
}

func (s *HttpServer) getSysinfo(w http.ResponseWriter, r *http.Request) {
	host, err := sysinfo.Host()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	info := host.Info()
	data, err := utils.EncodeToJson(info)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Write(data)
}
