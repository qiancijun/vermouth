package vermouth

import (
	"fmt"
	"net"
	"runtime/debug"

	"github.com/qiancijun/vermouth/logger"
)

// 暂时用不上
type ServerChannel struct {
	ip          string
	port        int
	Listener    *net.Listener
	UDPListener *net.UDPConn
	errHandler  func(err error)
}

func NewServerChannel(ip string, port int) ServerChannel {
	return ServerChannel{
		ip: ip,
		port: port,
		errHandler: func(err error) {
			logger.Error(err.Error())
		},
	}
}

func (sc *ServerChannel) SetErrorHandler(fn func(err error)) {
	sc.errHandler = fn
}

func (sc *ServerChannel) ListenerTCP(fn func(conn net.Conn)) error {
	l, err := net.Listen("tcp", fmt.Sprintf("%s:%d", sc.ip, sc.port))
	if err != nil {
		return err
	}
	sc.Listener = &l
	go func() {
		defer func() {
			if err := recover(); err != nil {
				logger.Errorf("listen tcp crushed, %s, trace: %s", err, string(debug.Stack()))
			}
		}()
		for {
			conn, err := (*sc.Listener).Accept()
			if err != nil {
				sc.errHandler(err)
				break
			}
			go func() {
				defer func() {
					if err := recover(); err != nil {
						logger.Errorf("connection handler crushed, err: %s, trace: %s", err, string(debug.Stack()))
					}
				}()
				fn(conn)
			}()
		}
	}()
	return nil
}

func (sc *ServerChannel) ListenerUDP(fn func(packet []byte, localAddr, srcAddr *net.UDPAddr)) error {
	addr := &net.UDPAddr{
		IP: net.ParseIP(sc.ip),
		Port: sc.port,
	}
	l, err := net.Listen("udp", fmt.Sprintf("%s:%d", sc.ip, sc.port))
	if err != nil {
		return err
	}
	sc.Listener = &l
	go func() {
		defer func() {
			if err := recover(); err != nil {
				logger.Errorf("listen tcp crushed, %s, trace: %s", err, string(debug.Stack()))
			}
		}()
		for {
			buf := make([]byte, 2048)
			n, srcAddr, err := (*sc.UDPListener).ReadFromUDP(buf)
			if err != nil {
				sc.errHandler(err)
				break
			}
			go func() {
				defer func() {
					if err := recover(); err != nil {
						logger.Errorf("connection handler crushed, err: %s, trace: %s", err, string(debug.Stack()))
					}
				}()
				packet := buf[0:n]
				fn(packet, addr, srcAddr)
			}()
		}
	}()
	return nil
}

func (sc *ServerChannel) ListenTls(certBytes, keyBytes []byte, fn func(conn net.Conn)) error {
	return nil
}