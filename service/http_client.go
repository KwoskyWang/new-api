package service

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/system_setting"

	"golang.org/x/net/proxy"
)

var (
	httpClient      *http.Client
	proxyClientLock sync.Mutex
	proxyClients    = make(map[string]*http.Client)

	// forceHTTP1Lock 保护强制 HTTP/1.1 客户端缓存。
	forceHTTP1Lock sync.Mutex
	// forceHTTP1Clients 以 proxyURL 为键缓存"仅协商 HTTP/1.1"的独立客户端，
	// 空字符串键对应无代理（默认）场景。每个客户端拥有独立、可复用的 Transport 与连接池。
	forceHTTP1Clients = make(map[string]*http.Client)
)

func checkRedirect(req *http.Request, via []*http.Request) error {
	fetchSetting := system_setting.GetFetchSetting()
	urlStr := req.URL.String()
	if err := common.ValidateURLWithFetchSetting(urlStr, fetchSetting.EnableSSRFProtection, fetchSetting.AllowPrivateIp, fetchSetting.DomainFilterMode, fetchSetting.IpFilterMode, fetchSetting.DomainList, fetchSetting.IpList, fetchSetting.AllowedPorts, fetchSetting.ApplyIPFilterForDomain); err != nil {
		return fmt.Errorf("redirect to %s blocked: %v", urlStr, err)
	}
	if len(via) >= 10 {
		return fmt.Errorf("stopped after 10 redirects")
	}
	return nil
}

func InitHttpClient() {
	transport := &http.Transport{
		MaxIdleConns:        common.RelayMaxIdleConns,
		MaxIdleConnsPerHost: common.RelayMaxIdleConnsPerHost,
		ForceAttemptHTTP2:   true,
		Proxy:               http.ProxyFromEnvironment, // Support HTTP_PROXY, HTTPS_PROXY, NO_PROXY env vars
	}
	if common.TLSInsecureSkipVerify {
		transport.TLSClientConfig = common.InsecureTLSConfig
	}

	if common.RelayTimeout == 0 {
		httpClient = &http.Client{
			Transport:     transport,
			CheckRedirect: checkRedirect,
		}
	} else {
		httpClient = &http.Client{
			Transport:     transport,
			Timeout:       time.Duration(common.RelayTimeout) * time.Second,
			CheckRedirect: checkRedirect,
		}
	}
}

func GetHttpClient() *http.Client {
	return httpClient
}

// GetHttpClientWithProxy returns the default client or a proxy-enabled one when proxyURL is provided.
func GetHttpClientWithProxy(proxyURL string) (*http.Client, error) {
	if proxyURL == "" {
		return GetHttpClient(), nil
	}
	return NewProxyHttpClient(proxyURL)
}

// ResetProxyClientCache 清空代理客户端缓存，确保下次使用时重新初始化
func ResetProxyClientCache() {
	proxyClientLock.Lock()
	for _, client := range proxyClients {
		if transport, ok := client.Transport.(*http.Transport); ok && transport != nil {
			transport.CloseIdleConnections()
		}
	}
	proxyClients = make(map[string]*http.Client)
	proxyClientLock.Unlock()

	// 同步清理"仅协商 HTTP/1.1"的独立客户端缓存，释放其连接池。
	forceHTTP1Lock.Lock()
	for _, client := range forceHTTP1Clients {
		if transport, ok := client.Transport.(*http.Transport); ok && transport != nil {
			transport.CloseIdleConnections()
		}
	}
	forceHTTP1Clients = make(map[string]*http.Client)
	forceHTTP1Lock.Unlock()
}

// GetRelayHttpClient 为一次上游转发选择出站 HTTP 客户端。
//   - forceHTTP1 为 false 时：保持现有行为，直接复用默认客户端或对应的代理客户端
//     （含现有 HTTP/2 协商、代理、TLS 校验、超时与重定向配置）。
//   - forceHTTP1 为 true 时：返回一个独立、可复用的客户端，仅向上游协商 HTTP/1.1。
//     该客户端基于对应的默认/代理客户端 Transport 克隆，沿用其代理、TLS、超时与
//     重定向配置，但关闭 HTTP/2 并清理 TLS ALPN 中遗留的 h2，以实际协商结果为准。
//     结果按 proxyURL 缓存，连续请求可复用同一连接池。
func GetRelayHttpClient(proxyURL string, forceHTTP1 bool) (*http.Client, error) {
	baseClient, err := GetHttpClientWithProxy(proxyURL)
	if err != nil {
		return nil, err
	}
	if !forceHTTP1 {
		return baseClient, nil
	}

	forceHTTP1Lock.Lock()
	defer forceHTTP1Lock.Unlock()
	if client, ok := forceHTTP1Clients[proxyURL]; ok {
		return client, nil
	}

	forcedClient, err := newForceHTTP1Client(baseClient)
	if err != nil {
		return nil, err
	}
	forceHTTP1Clients[proxyURL] = forcedClient
	return forcedClient, nil
}

// newForceHTTP1Client 基于给定的基础客户端克隆出一个仅协商 HTTP/1.1 的新客户端。
// 克隆 Transport 以复用其代理、TLS 校验、拨号、超时等配置，同时：
//   - 置 ForceAttemptHTTP2 = false；
//   - 清空 TLSNextProto（显式禁用 HTTP/2 升级路径）；
//   - 处理 TLS ALPN：移除遗留的 "h2"，仅保留/声明 "http/1.1"，以实际协商结果为准。
//
// 若基础客户端的 Transport 不是 *http.Transport（理论上不会发生），则回退到按现有
// 配置新建一个 HTTP/1.1 Transport，保证开关始终可用。
func newForceHTTP1Client(baseClient *http.Client) (*http.Client, error) {
	if baseClient == nil {
		baseClient = http.DefaultClient
	}

	var transport *http.Transport
	if base, ok := baseClient.Transport.(*http.Transport); ok && base != nil {
		transport = base.Clone()
	} else {
		transport = &http.Transport{
			MaxIdleConns:        common.RelayMaxIdleConns,
			MaxIdleConnsPerHost: common.RelayMaxIdleConnsPerHost,
			Proxy:               http.ProxyFromEnvironment,
		}
		if common.TLSInsecureSkipVerify {
			transport.TLSClientConfig = common.InsecureTLSConfig.Clone()
		}
	}

	disableHTTP2OnTransport(transport)

	return &http.Client{
		Transport:     transport,
		Timeout:       baseClient.Timeout,
		CheckRedirect: baseClient.CheckRedirect,
		Jar:           baseClient.Jar,
	}, nil
}

// disableHTTP2OnTransport 就地改造 Transport，使其仅协商 HTTP/1.1。
func disableHTTP2OnTransport(transport *http.Transport) {
	if transport == nil {
		return
	}
	transport.ForceAttemptHTTP2 = false
	// 显式清空 HTTP/2 升级映射，避免克隆自已初始化过 HTTP/2 的 Transport 时残留升级路径。
	transport.TLSNextProto = make(map[string]func(string, *tls.Conn) http.RoundTripper)

	// 处理 TLS ALPN：克隆已初始化的 Transport 时，TLSClientConfig.NextProtos 可能残留 "h2"。
	// 这里复制一份配置并把 ALPN 收敛为仅 "http/1.1"，以实际协商结果为准，且不影响原客户端。
	var tlsConfig *tls.Config
	if transport.TLSClientConfig != nil {
		tlsConfig = transport.TLSClientConfig.Clone()
	} else {
		tlsConfig = &tls.Config{}
	}
	filtered := tlsConfig.NextProtos[:0:0]
	for _, proto := range tlsConfig.NextProtos {
		if proto == "h2" {
			continue
		}
		filtered = append(filtered, proto)
	}
	if len(filtered) == 0 {
		filtered = []string{"http/1.1"}
	}
	tlsConfig.NextProtos = filtered
	transport.TLSClientConfig = tlsConfig
}

// NewProxyHttpClient 创建支持代理的 HTTP 客户端
func NewProxyHttpClient(proxyURL string) (*http.Client, error) {
	if proxyURL == "" {
		if client := GetHttpClient(); client != nil {
			return client, nil
		}
		return http.DefaultClient, nil
	}

	proxyClientLock.Lock()
	if client, ok := proxyClients[proxyURL]; ok {
		proxyClientLock.Unlock()
		return client, nil
	}
	proxyClientLock.Unlock()

	parsedURL, err := url.Parse(proxyURL)
	if err != nil {
		return nil, err
	}

	switch parsedURL.Scheme {
	case "http", "https":
		transport := &http.Transport{
			MaxIdleConns:        common.RelayMaxIdleConns,
			MaxIdleConnsPerHost: common.RelayMaxIdleConnsPerHost,
			ForceAttemptHTTP2:   true,
			Proxy:               http.ProxyURL(parsedURL),
		}
		if common.TLSInsecureSkipVerify {
			transport.TLSClientConfig = common.InsecureTLSConfig
		}
		client := &http.Client{
			Transport:     transport,
			CheckRedirect: checkRedirect,
		}
		client.Timeout = time.Duration(common.RelayTimeout) * time.Second
		proxyClientLock.Lock()
		proxyClients[proxyURL] = client
		proxyClientLock.Unlock()
		return client, nil

	case "socks5", "socks5h":
		// 获取认证信息
		var auth *proxy.Auth
		if parsedURL.User != nil {
			auth = &proxy.Auth{
				User:     parsedURL.User.Username(),
				Password: "",
			}
			if password, ok := parsedURL.User.Password(); ok {
				auth.Password = password
			}
		}

		// 创建 SOCKS5 代理拨号器
		// proxy.SOCKS5 使用 tcp 参数，所有 TCP 连接包括 DNS 查询都将通过代理进行。行为与 socks5h 相同
		dialer, err := proxy.SOCKS5("tcp", parsedURL.Host, auth, proxy.Direct)
		if err != nil {
			return nil, err
		}

		transport := &http.Transport{
			MaxIdleConns:        common.RelayMaxIdleConns,
			MaxIdleConnsPerHost: common.RelayMaxIdleConnsPerHost,
			ForceAttemptHTTP2:   true,
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return dialer.Dial(network, addr)
			},
		}
		if common.TLSInsecureSkipVerify {
			transport.TLSClientConfig = common.InsecureTLSConfig
		}

		client := &http.Client{Transport: transport, CheckRedirect: checkRedirect}
		client.Timeout = time.Duration(common.RelayTimeout) * time.Second
		proxyClientLock.Lock()
		proxyClients[proxyURL] = client
		proxyClientLock.Unlock()
		return client, nil

	default:
		return nil, fmt.Errorf("unsupported proxy scheme: %s, must be http, https, socks5 or socks5h", parsedURL.Scheme)
	}
}
