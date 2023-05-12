package discovery

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/go-zookeeper/zk"
	jsoniter "github.com/json-iterator/go"
	"github.com/qiancijun/vermouth/config"
	"github.com/qiancijun/vermouth/logger"
	"github.com/qiancijun/vermouth/utils"
)

/**
* 用来与 zookeeper 进行连接并且获取数据
* 抓取逻辑：
* 	监听 zk 的 /services 路径，该路径下保存服务名称
*	服务名称下的节点是每一台机器的注册信息
 */
type ZookeeperDiscovery struct {
	log       *logger.Logger
	conn      *zk.Conn
	EventChan chan Event
	Services  map[string]struct{} // 存上一次获取的所有服务名称和其下所有节点
	Shutdown  map[string]contextWithCancel
	// httpProxy *proxy.HttpReverseProxy
}

var _ Discovery = (*ZookeeperDiscovery)(nil)

type ZookeeperServiceInfo struct {
	Name                string                 `json:"name"`
	Id                  string                 `json:"id"`
	Address             string                 `json:"address"`
	Port                int64                  `json:"port"`
	SslPort             int64                  `json:"sslPort"`
	Payload             map[string]interface{} `json:"payload"`
	RegistrationTimeUTC uint64                 `json:"registrationTimeUTC"`
	ServiceType         string                 `json:"serviceType"`
	UriSpec             map[string]interface{} `json:"uriSpec"`
}

type contextWithCancel struct {
	cancel context.CancelFunc
	ctx    context.Context
}

func NewContextWithCancel(c context.Context) contextWithCancel {
	ctx, cancel := context.WithCancel(c)
	return contextWithCancel{
		cancel: cancel,
		ctx:    ctx,
	}
}

const (
	SERVICES_PATH = "/services"
)

var (
	ErrPathNotExsists = errors.New("zookeeper can't get children, path noe exsists")
)

func init() {
	factories["zookeeper"] = NewZookeeperDiscovery
}

func NewZookeeperDiscovery(notify chan Event, args config.Discovery) Discovery {
	zk := &ZookeeperDiscovery{
		Services:  make(map[string]struct{}),
		Shutdown:  make(map[string]contextWithCancel),
		EventChan: notify,
	}

	conn, err := zookeeperClient(args.Address, args.ConnectTimeout)
	if err != nil {
		logger.Errorf("can't create zookeeper client, %s", err.Error())
		return nil
	}
	if zkLog := logger.Logs["zk"]; zkLog != nil {
		zk.log = zkLog
	} else {
		zk.log = logger.Log
	}
	zk.conn = conn
	return zk
}

func zookeeperClient(addrs []string, timeout int64) (*zk.Conn, error) {
	c, _, err := zk.Connect(addrs, time.Duration(timeout)*time.Second)
	if err != nil {
		return nil, err
	}
	if zkLog, ok := logger.Logs["zk"]; ok {
		c.SetLogger(zkLog)
	}
	return c, nil
}

// ========== 对 Discovery 接口实现 ==========

func (zk *ZookeeperDiscovery) Class() string {
	return "zk"
}

func (dis *ZookeeperDiscovery) Run() {
	go dis.watchServices()
}

/**
* 监听逻辑：监听 /services 路径下的所有节点，该路径下存放所有的服务名称
* 监听每一个服务名称
 */
func (zk *ZookeeperDiscovery) watchServices() {
	zk.log.Debug("start watch services")
	for {
		servicesChildren, _, notify, err := zk.conn.ChildrenW(SERVICES_PATH)
		if err != nil {
			zk.log.Errorf("path %s not exists, %s", SERVICES_PATH, err.Error())
			time.Sleep(5 * time.Second)
			continue
		}
		curServices := make(map[string]struct{}) // 本次读取的所有服务名称
		for _, service := range servicesChildren {
			curServices[service] = struct{}{}
		}
		zk.log.Debugf("zookeeper find new service: %+v", servicesChildren)
		zk.log.Debugf("curServices: %+v\t newServices: %+v", zk.Services, curServices)
		// 检查哪些服务全部下线，取消监听，删除 prefix
		for lastService := range zk.Services {
			if _, ok := curServices[lastService]; !ok {
				zk.Shutdown[lastService].cancel() // 阻塞了
				zk.EventChan <- Event{
					Opt:         REMOVE_SERVICE,
					ServiceName: lastService,
				}
			}
		}

		for service := range curServices {
			// 没有监听再启动携程监听
			if _, ok := zk.Shutdown[service]; !ok {
				zk.Shutdown[service] = NewContextWithCancel(context.Background())
				zk.EventChan <- Event{
					Opt:         ADD_SERVICE,
					ServiceName: service,
				}
				go zk.watchNode(service)
			}
		}
		zk.Services = curServices
		e := <-notify
		zk.log.Debugf("zk has new event: %+v", e)
	}

}

func (zk *ZookeeperDiscovery) watchNode(service string) {
	path := SERVICES_PATH + "/" + service
	for {
		// nodes 是最新服务下的所有节点的id，不是地址
		nodes, _, notify, err := zk.conn.ChildrenW(path)
		if err != nil {
			zk.log.Errorf("client can't get %s children, %s", path, err.Error())
		}
		// 获取所有节点的数据
		infos := make(map[string]ZookeeperServiceInfo)
		for _, node := range nodes {
			path := path + "/" + node
			data, _, err := zk.conn.Get(path)
			if err != nil {
				zk.log.Errorf("client can't get [%s] data, %s", path, err.Error())
				continue
			}
			info := ZookeeperServiceInfo{}
			if err = jsoniter.Unmarshal(data, &info); err != nil {
				zk.log.Errorf("client can't unmarshal [%s] data, %s", path, err.Error())
				continue
			}
			infos[node] = info
		}

		// 移除下线的节点
		curAddress := make([]string, 0)
		for _, v := range infos {
			curAddress = append(curAddress, fmt.Sprintf("%s:%d", v.Address, v.Port))
		}

		zk.EventChan <- Event{
			Opt:         REMOVE_HOSTS,
			ServiceName: service,
			Children:    curAddress,
		}

		// 向状态机注册, nodes 是所有该服务的节点
		for _, node := range nodes {
			ip, port := infos[node].Address, infos[node].Port
			switch utils.ValidIPAddress(ip) {
			case "IPv4":
				// zk.httpProxy.AddHost(service, ip)
				zk.EventChan <- Event{
					Opt:         ADD_HOST,
					ServiceName: service,
					Host:        fmt.Sprintf("%s:%d", ip, port),
				}
			case "IPv6":
				zk.log.Info("IPv6 is unsupport")
			default:
				// host解析
				addrs, err := net.LookupHost(ip)
				if err != nil {
					zk.log.Errorf("can't parse hostname: %s address", ip)
					break
				}
				for _, addr := range addrs {
					// zk.httpProxy.AddHost(service, addr)
					zk.EventChan <- Event{
						Opt:         ADD_HOST,
						ServiceName: service,
						Host:        fmt.Sprintf("%s:%d", addr, port),
					}
				}
			}
		}

		select {
		case e := <-notify:
			zk.log.Debugf("%s has new event: %+v", path, e)
		case <-zk.Shutdown[service].ctx.Done():
			zk.log.Debugf("%s closed", path)
			delete(zk.Shutdown, service)
			return
		}
	}
}
