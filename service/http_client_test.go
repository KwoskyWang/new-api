package service

import (
	"crypto/tls"
	"crypto/x509"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"net/url"
	"sync/atomic"
	"testing"
	"time"
)

func newRelayProtocolTestUpstream(t *testing.T) (*httptest.Server, *http.Client) {
	t.Helper()
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Test-Remote-Addr", r.RemoteAddr)
		_, _ = io.WriteString(w, "ok")
	}))
	server.EnableHTTP2 = true
	server.StartTLS()
	t.Cleanup(server.Close)

	roots := x509.NewCertPool()
	roots.AddCert(server.Certificate())
	client := &http.Client{
		Transport: &http.Transport{
			ForceAttemptHTTP2: true,
			TLSClientConfig: &tls.Config{
				RootCAs:    roots,
				MinVersion: tls.VersionTLS12,
			},
		},
		Timeout: 5 * time.Second,
	}
	t.Cleanup(client.CloseIdleConnections)
	return server, client
}

func installRelayProtocolTestClients(t *testing.T, defaultClient *http.Client, proxyURL string, proxyClient *http.Client) {
	t.Helper()
	previousClient := httpClient
	proxyClientLock.Lock()
	previousProxyClients := proxyClients
	proxyClients = make(map[string]*http.Client)
	proxyClientLock.Unlock()
	ResetProxyClientCache()
	httpClient = defaultClient
	if proxyURL != "" {
		proxyClientLock.Lock()
		proxyClients[proxyURL] = proxyClient
		proxyClientLock.Unlock()
	}
	t.Cleanup(func() {
		ResetProxyClientCache()
		httpClient = previousClient
		proxyClientLock.Lock()
		proxyClients = previousProxyClients
		proxyClientLock.Unlock()
	})
}

func relayProtocolTestRequest(t *testing.T, client *http.Client, requestURL string) (int, string, bool) {
	t.Helper()
	var reused atomic.Bool
	request, err := http.NewRequest(http.MethodGet, requestURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	request = request.WithContext(httptrace.WithClientTrace(request.Context(), &httptrace.ClientTrace{
		GotConn: func(info httptrace.GotConnInfo) { reused.Store(info.Reused) },
	}))
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if _, err := io.Copy(io.Discard, response.Body); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("unexpected response status: %s", response.Status)
	}
	return response.ProtoMajor, response.Header.Get("X-Test-Remote-Addr"), reused.Load()
}

func TestGetRelayHttpClientForceHTTP1(t *testing.T) {
	server, originalClient := newRelayProtocolTestUpstream(t)
	installRelayProtocolTestClients(t, originalClient, "", nil)

	defaultClient, err := GetRelayHttpClient("", false)
	if err != nil {
		t.Fatal(err)
	}
	if defaultClient != originalClient {
		t.Fatal("disabled option must preserve the existing default client")
	}
	if protocol, _, _ := relayProtocolTestRequest(t, defaultClient, server.URL); protocol != 2 {
		t.Fatalf("default client negotiated HTTP/%d, want HTTP/2", protocol)
	}

	forcedClient, err := GetRelayHttpClient("", true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(forcedClient.CloseIdleConnections)
	if forcedClient == originalClient || forcedClient.Transport == originalClient.Transport {
		t.Fatal("forced HTTP/1 must use a separate client and transport")
	}
	if forcedClient.Timeout != originalClient.Timeout {
		t.Fatal("forced client must retain the existing timeout")
	}
	protocol, firstAddress, _ := relayProtocolTestRequest(t, forcedClient, server.URL)
	if protocol != 1 {
		t.Fatalf("forced client negotiated HTTP/%d, want HTTP/1.1", protocol)
	}
	cachedClient, err := GetRelayHttpClient("", true)
	if err != nil {
		t.Fatal(err)
	}
	if cachedClient != forcedClient {
		t.Fatal("repeated forced selection must reuse the client and connection pool")
	}
	protocol, secondAddress, reused := relayProtocolTestRequest(t, cachedClient, server.URL)
	if protocol != 1 || !reused || firstAddress != secondAddress {
		t.Fatalf("forced connection was not reused: protocol=%d reused=%t addresses=%q/%q", protocol, reused, firstAddress, secondAddress)
	}

	if protocol, _, _ := relayProtocolTestRequest(t, originalClient, server.URL); protocol != 2 {
		t.Fatalf("forcing a different client changed the original HTTP/2 transport: HTTP/%d", protocol)
	}
	unchangedClient, err := GetRelayHttpClient("", false)
	if err != nil || unchangedClient != originalClient {
		t.Fatalf("default selection changed after forcing HTTP/1: client=%p err=%v", unchangedClient, err)
	}
}

func TestGetRelayHttpClientForceHTTP1ThroughHTTPProxy(t *testing.T) {
	server, directClient := newRelayProtocolTestUpstream(t)
	var tunnels atomic.Int64
	proxyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodConnect {
			http.Error(w, "CONNECT required", http.StatusMethodNotAllowed)
			return
		}
		upstream, err := net.DialTimeout("tcp", r.Host, 3*time.Second)
		if err != nil {
			http.Error(w, "upstream unavailable", http.StatusBadGateway)
			return
		}
		defer upstream.Close()
		connection, buffered, err := w.(http.Hijacker).Hijack()
		if err != nil {
			return
		}
		defer connection.Close()
		tunnels.Add(1)
		if _, err := buffered.WriteString("HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
			return
		}
		if err := buffered.Flush(); err != nil {
			return
		}
		go func() {
			_, _ = io.Copy(upstream, buffered)
			_ = upstream.Close()
		}()
		_, _ = io.Copy(connection, upstream)
	}))
	t.Cleanup(proxyServer.Close)
	proxyURL, err := url.Parse(proxyServer.URL)
	if err != nil {
		t.Fatal(err)
	}
	proxyTransport := directClient.Transport.(*http.Transport).Clone()
	proxyTransport.Proxy = http.ProxyURL(proxyURL)
	originalProxyClient := &http.Client{Transport: proxyTransport, Timeout: directClient.Timeout}
	t.Cleanup(originalProxyClient.CloseIdleConnections)
	installRelayProtocolTestClients(t, directClient, proxyServer.URL, originalProxyClient)

	defaultClient, err := GetRelayHttpClient(proxyServer.URL, false)
	if err != nil || defaultClient != originalProxyClient {
		t.Fatalf("disabled option must preserve the existing proxy client: client=%p err=%v", defaultClient, err)
	}
	if protocol, _, _ := relayProtocolTestRequest(t, defaultClient, server.URL); protocol != 2 {
		t.Fatalf("default proxied client negotiated HTTP/%d, want HTTP/2", protocol)
	}

	forcedClient, err := GetRelayHttpClient(proxyServer.URL, true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(forcedClient.CloseIdleConnections)
	if forcedClient == originalProxyClient || forcedClient.Transport == originalProxyClient.Transport {
		t.Fatal("forced proxied client must have a separate transport")
	}
	if protocol, _, _ := relayProtocolTestRequest(t, forcedClient, server.URL); protocol != 1 {
		t.Fatalf("forced proxied client negotiated HTTP/%d, want HTTP/1.1", protocol)
	}
	if tunnels.Load() != 2 {
		t.Fatalf("expected separate default and forced CONNECT tunnels, got %d", tunnels.Load())
	}
	if protocol, _, _ := relayProtocolTestRequest(t, originalProxyClient, server.URL); protocol != 2 {
		t.Fatalf("forcing HTTP/1 changed the original proxy client: HTTP/%d", protocol)
	}
	if tunnels.Load() != 2 {
		t.Fatal("original proxy client lost its reusable connection")
	}
}

// TestForceHTTP1ClonedTransportDropsLeftoverH2 直接验证：当基于一个已初始化过 HTTP/2
// （ALPN NextProtos 含 "h2"）的 Transport 克隆出强制 HTTP/1.1 客户端时，遗留的 "h2"
// 被清理、补齐 "http/1.1"，HTTP/2 升级路径被关闭，且原始 Transport 不受影响。
func TestForceHTTP1ClonedTransportDropsLeftoverH2(t *testing.T) {
	baseTransport := &http.Transport{
		ForceAttemptHTTP2: true,
		TLSClientConfig: &tls.Config{
			NextProtos: []string{"h2", "http/1.1"},
			MinVersion: tls.VersionTLS12,
		},
		TLSNextProto: map[string]func(string, *tls.Conn) http.RoundTripper{
			"h2": func(string, *tls.Conn) http.RoundTripper { return nil },
		},
	}
	baseClient := &http.Client{Transport: baseTransport, Timeout: 7 * time.Second}

	forcedClient, err := newForceHTTP1Client(baseClient)
	if err != nil {
		t.Fatal(err)
	}
	forcedTransport, ok := forcedClient.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("forced transport has unexpected type %T", forcedClient.Transport)
	}
	if forcedTransport == baseTransport {
		t.Fatal("forced client must not reuse the base transport instance")
	}
	if forcedTransport.ForceAttemptHTTP2 {
		t.Fatal("forced transport must disable ForceAttemptHTTP2")
	}
	if len(forcedTransport.TLSNextProto) != 0 {
		t.Fatalf("forced transport must clear HTTP/2 upgrade map, got %d entries", len(forcedTransport.TLSNextProto))
	}
	for _, proto := range forcedTransport.TLSClientConfig.NextProtos {
		if proto == "h2" {
			t.Fatal("forced transport ALPN still advertises h2")
		}
	}
	if got := forcedTransport.TLSClientConfig.NextProtos; len(got) != 1 || got[0] != "http/1.1" {
		t.Fatalf("forced transport ALPN = %v, want [http/1.1]", got)
	}
	if forcedClient.Timeout != baseClient.Timeout {
		t.Fatalf("forced client timeout = %s, want %s", forcedClient.Timeout, baseClient.Timeout)
	}

	// 原始 Transport 不应被就地修改。
	if !baseTransport.ForceAttemptHTTP2 {
		t.Fatal("base transport ForceAttemptHTTP2 was mutated")
	}
	if len(baseTransport.TLSNextProto) != 1 {
		t.Fatal("base transport TLSNextProto was mutated")
	}
	if got := baseTransport.TLSClientConfig.NextProtos; len(got) != 2 || got[0] != "h2" {
		t.Fatalf("base transport ALPN was mutated: %v", got)
	}
}
