package vermouth

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"testing"
	"time"

	jsoniter "github.com/json-iterator/go"
	"github.com/panjf2000/ants"
	"github.com/qiancijun/vermouth/config"
	rate "github.com/qiancijun/vermouth/rate_limiter"
	"github.com/stretchr/testify/assert"
)

func TestHttpProxy(t *testing.T) {
	args := config.HttpProxyArgs{
		PrefixerType: "hash-prefixer",
		Port:         7181,
	}
	client := NewHttpReverseProxy(args)
	assert.NotNil(t, client)
	go client.Run()

	client.AddPrefix("/api/service", "round-robin", []string{
		"127.0.0.1:8080",
	}, false)
	cases := []struct {
		name   string
		args   []string
		expect []string
	}{
		{
			"test-success-proxy",
			[]string{
				"http://127.0.0.1:7181/api/service/hello",
				"http://127.0.0.1:7181/api/service/index",
				"http://127.0.0.1:7181/api/service/person?name=cheryl&age=18",
			},
			[]string{
				"hello world, i'm 8080",
				"index, i'm 8080",
				"hi, i'm cheryl, i'm 18 years old",
			},
		},
		{
			"test-error-proxy",
			[]string{
				"http://127.0.0.1:7181/api/hello",
				"http://127.0.0.1:7181/index",
				"http://127.0.0.1:7181/person?name=cheryl&age=18",
			},
			[]string{
				"Path not found, please check prefix",
				"Path not found, please check prefix",
				"Path not found, please check prefix",
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			for i, arg := range c.args {
				res, err := http.Get(arg)
				assert.NoError(t, err)
				content := getContentFromResponse(res)
				assert.Equal(t, c.expect[i], content)
			}
		})
	}
}

func TestPostQuery(t *testing.T) {
	args := config.HttpProxyArgs{
		PrefixerType: "hash-prefixer",
		Port:         7181,
	}
	client := NewHttpReverseProxy(args)
	assert.NotNil(t, client)
	go client.Start()

	client.AddPrefix("/api/service", "round-robin", []string{
		"127.0.0.1:8080",
	}, false)

	jsonStr := []byte(`{ "Name": "cheryl", "Age": "18" }`)
	req, err := http.NewRequest("POST", "http://127.0.0.1:7181/api/service/people", bytes.NewBuffer(jsonStr))
	assert.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	h := &http.Client{}
	resp, err := h.Do(req)
	assert.NoError(t, err)
	defer resp.Body.Close()

	content := getContentFromResponse(resp)
	assert.Equal(t, "hi, i'm cheryl, i'm 18 years old", content)
}

func TestAccessControl(t *testing.T) {
	args := config.HttpProxyArgs{
		PrefixerType: "hash-prefixer",
		Port:         7181,
	}
	client := NewHttpReverseProxy(args)
	assert.NotNil(t, client)
	go client.Start()

	client.AddPrefix("/api/service", "round-robin", []string{
		"127.0.0.1:8080",
	}, false)

	client.AddBlackList([]string{
		"127.0.0.1/32",
		"0.0.0.0/32",
	})
	res, err := http.Get("http://127.0.0.1:7181/api/service/hello")
	assert.NoError(t, err)
	content := getContentFromResponse(res)
	assert.Equal(t, ForbiddenAccessSystem, content)
}

func TestRateLimiter(t *testing.T) {
	args := config.HttpProxyArgs{
		PrefixerType: "hash-prefixer",
		Port:         7181,
	}
	client := NewHttpReverseProxy(args)
	assert.NotNil(t, client)
	go client.Run()

	client.AddPrefix("/api/service", "round-robin", []string{
		"127.0.0.1:8080",
	}, false)
	limiter, err := rate.Build("qps")
	assert.NoError(t, err)
	limiter.SetRate(1, 1)
	client.rateLimiters["/api/service"] = limiter

	req, err := http.NewRequest("GET", "http://127.0.0.1:7181/api/service/hello", nil)
	assert.NoError(t, err)
	httpClient := &http.Client{}
	res, err := httpClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, "hello wolrd, i'm 8080", getContentFromResponse(res))

	res, err = httpClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, rate.ErrNoReaminToken.Error(), getContentFromResponse(res))
}

func TestStart(t *testing.T) {
	args := config.HttpProxyArgs{
		PrefixerType: "hash-prefixer",
		Port:         7181,
	}
	client := NewHttpReverseProxy(args)
	assert.NotNil(t, client)
	go client.Start()
	client.Close()
}

func TestUnMarshal(t *testing.T) {
	filePath := "../data/test.json"
	file, err := os.OpenFile(filePath, os.O_RDONLY, 0744)
	assert.NoError(t, err)
	list := []*HttpReverseProxy{}
	err = jsoniter.NewDecoder(file).Decode(&list)
	assert.NoError(t, err)
}

func getContentFromResponse(r *http.Response) string {
	bytes, _ := ioutil.ReadAll(r.Body)
	r.Body.Close()
	return string(bytes)
}

func TestBuildHttpReverseProxyFromConfig(t *testing.T) {
	c := config.NewVermouthConfig()
	err := c.ReadConfig("../config/test_config.toml")
	assert.NoError(t, err)

	list := BuildHttpReverseProxyFromConfig(c.HttpProxy)
	m := make(map[int64]*HttpReverseProxy)
	for _, p := range list {
		m[p.Port] = p
	}

	for _, p := range c.HttpProxy {
		t.Run(fmt.Sprintf("create %d", p.Port), func(t *testing.T) {
			proxy := m[p.Port]
			assert.NotNil(t, proxy)
			for _, path := range p.Paths {
				assert.Equal(t, path.Hosts, proxy.GetHosts(path.Pattern))
			}
		})
	}
}

func TestHTTPs(t *testing.T) {
	pool := x509.NewCertPool()
	caCertPath := "../cert/ca/ca.crt"

	caCrt, err := ioutil.ReadFile(caCertPath)
	if err != nil {

		fmt.Println("ReadFile err:", err)
		return
	}
	pool.AppendCertsFromPEM(caCrt)

	tr := &http.Transport{
		TLSClientConfig: &tls.Config{
			RootCAs: pool,
		},
		DisableCompression: true,
	}

	client := &http.Client{Transport: tr}

	resp, err := client.Get("https://127.0.0.1:9001/api/hello")
	assert.NoError(t, err)
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	assert.NoError(t, err)
	assert.Equal(t, "hello wolrd, i'm 8080", string(body))
}

// go test -benchmem -bench BenchmarkHttpReverseProxy -run =^$ -cpu 4,8,12 -count=2
func BenchmarkHttpReverseProxy(b *testing.B) {
	proxy := NewHttpReverseProxy(config.HttpProxyArgs{
		Port:         9000,
		PrefixerType: "hash-prefixer",
	})
	proxy.AddPrefix("/api", "round-robin", []string{"localhost:8080"}, false)
	go proxy.Run()
	httpClient := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			Dial: (&net.Dialer{
				Timeout:   30 * time.Second, //限制建立TCP连接的时间
				KeepAlive: 30 * time.Second,
			}).Dial,
			ForceAttemptHTTP2:     true,
			MaxIdleConnsPerHost:   1024,
			MaxConnsPerHost:       1024,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		},
	}
	p, _ := ants.NewPool(12)
	req, err := http.NewRequest("GET", "http://localhost:9000/api/hello", nil)
	assert.NoError(b, err)
	assert.NoError(b, err)
	b.ResetTimer() // 重置定时器
	for n := 0; n < b.N; n++ {
		p.Submit(func() {
			res, err := httpClient.Do(req)
			assert.NoError(b, err)
			assert.Equal(b, "hello world, i'm 8080", getContentFromResponse(res))
		})
	}
}
