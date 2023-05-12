package utils

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/json-iterator/go"
)

var ConnectionTimeout = 2 * time.Second

var (
	ErrorFailParseCert = errors.New("failed to parse root certificate")
)

func GetIP(remoteAddr string) string {
	remoteHost, _, _ := net.SplitHostPort(remoteAddr)
	return remoteHost
}

func GetHost(url *url.URL) string {
	if _, _, err := net.SplitHostPort(url.Host); err == nil {
		return url.Host
	}
	if url.Scheme == "http" {
		return fmt.Sprintf("%s:%s", url.Host, "80")
	} else if url.Scheme == "https" {
		return fmt.Sprintf("%s:%s", url.Host, "443")
	}
	return url.Host
}

// 尝试与目标主机建立连接
func IsBackendAlive(host string) bool {
	addr, err := net.ResolveTCPAddr("tcp", host)
	if err != nil {
		return false
	}
	resolve := fmt.Sprintf("%s:%d", addr.IP, addr.Port)
	conn, err := net.DialTimeout("tcp", resolve, ConnectionTimeout)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func GetOutBoundIP()(ip string, err error)  {
    conn, err := net.Dial("udp", "8.8.8.8:53")
    if err != nil {
        fmt.Println(err)
        return
    }
    localAddr := conn.LocalAddr().(*net.UDPAddr)
    // fmt.Println(localAddr.String())
    ip = strings.Split(localAddr.String(), ":")[0]
    return
}

func InetToi(ip string) (uint32, error) {
	// 对 IPv4 地址分段
	ipSegs := strings.Split(ip, ".")
	var ret uint32 = 0
	pos := 24
	for _, ipSeg := range ipSegs {
		tempInt, err := strconv.Atoi(ipSeg)
		if err != nil {
			// logger.Warnf("{inetToi} can't convert ip address: %s", err.Error())
			return 0, err
		}
		// 八位为一组
		tempInt <<= pos
		// |= 无进位加法
		ret |= uint32(tempInt)
		pos -= 8
	}
	return ret, nil
}

func RemoteIp(req *http.Request) string {
	var remoteAddr string
	// RemoteAddr
	remoteAddr = req.RemoteAddr
	if remoteAddr != "" {
		return remoteAddr
	}
	// ipv4
	remoteAddr = req.Header.Get("ipv4")
	if remoteAddr != "" {
		return remoteAddr
	}
	//
	remoteAddr = req.Header.Get("XForwardedFor")
	if remoteAddr != "" {
		return remoteAddr
	}
	// X-Forwarded-For
	remoteAddr = req.Header.Get("X-Forwarded-For")
	if remoteAddr != "" {
		return remoteAddr
	}
	// X-Real-Ip
	remoteAddr = req.Header.Get("X-Real-Ip")
	if remoteAddr != "" {
		return remoteAddr
	} else {
		remoteAddr = "127.0.0.1"
	}
	return remoteAddr
}

func ValidIPAddress(queryIP string) string {
    if sp := strings.Split(queryIP, "."); len(sp) == 4 {
        for _, s := range sp {
            if len(s) > 1 && s[0] == '0' {
                return "Neither"
            }
            if v, err := strconv.Atoi(s); err != nil || v > 255 {
                return "Neither"
            }
        }
        return "IPv4"
    }
    if sp := strings.Split(queryIP, ":"); len(sp) == 8 {
        for _, s := range sp {
            if len(s) > 4 {
                return "Neither"
            }
            if _, err := strconv.ParseUint(s, 16, 64); err != nil {
                return "Neither"
            }
        }
        return "IPv6"
    }
    return "Neither"
}

func CreateListener(port int64) (net.Listener, error) {
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return nil, err
	}
	return listener, nil
}

func EncodeToJson(value interface{}) ([]byte, error) {
	var buf bytes.Buffer
	enc := jsoniter.NewEncoder(&buf)
	if err := enc.Encode(value); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func ReadFromRequest(r *http.Request, dest interface{}) error {
	if err := jsoniter.NewDecoder(r.Body).Decode(dest); err != nil {
		return err
	}
	return nil
}

func ListenTls(ip string, port int, certBytes, keyBytes []byte) (*net.Listener, error) {
	cert, err := tls.X509KeyPair(certBytes, keyBytes)
	if err != nil {
		return nil, err
	}
	clientCertPool := x509.NewCertPool()
	ok := clientCertPool.AppendCertsFromPEM(certBytes)
	if !ok {
		return nil, ErrorFailParseCert
	}
	config := &tls.Config{
		ClientCAs: clientCertPool,
		ServerName: "vermouth",
		Certificates: []tls.Certificate{cert},
		ClientAuth: tls.RequireAndVerifyClientCert,
	}
	ln, err := tls.Listen("tcp", fmt.Sprintf("%s:%d", ip, port), config)
	if err != nil {
		return nil, err
	}
	return &ln, nil
}