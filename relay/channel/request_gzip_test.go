package channel

import (
	"bytes"
	"compress/gzip"
	"io"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

func gzipRequestTestJSON(size int) []byte {
	prefix := []byte("{\n  \"model\": \"claude-opus-5\", \"messages\": [{\"role\": \"user\", \"content\": \"原文")
	suffix := []byte("\"}]\n}\n")
	body := append(prefix, bytes.Repeat([]byte("x"), size-len(prefix)-len(suffix))...)
	return append(body, suffix...)
}

func TestGzipLargeClaudeRequestWireRoundTripAndReplay(t *testing.T) {
	// Exercise the inclusive 1 MiB boundary, JSON content-type parameters,
	// and preservation of UTF-8 and whitespace in the original request.
	original := gzipRequestTestJSON(1 << 20)
	type receivedRequest struct {
		body     []byte
		encoding string
		length   int64
		err      error
	}
	received := make(chan receivedRequest, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		received <- receivedRequest{body: body, encoding: r.Header.Get("Content-Encoding"), length: r.ContentLength, err: err}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	request, err := http.NewRequest(http.MethodPost, server.URL, bytes.NewReader(original))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json; charset=utf-8")
	if err := gzipLargeClaudeRequest(request); err != nil {
		t.Fatal(err)
	}
	defer request.Body.Close()
	if request.Header.Get("Content-Encoding") != "gzip" || request.GetBody == nil {
		t.Fatal("compressed request must declare gzip and remain replayable")
	}
	if request.ContentLength <= 0 || request.ContentLength*5 > int64(len(original))*4 {
		t.Fatalf("unexpected compressed content length: %d", request.ContentLength)
	}
	if request.Header.Get("Content-Type") != "application/json; charset=utf-8" {
		t.Fatal("compression changed the original content type")
	}

	for attempt := 0; attempt < 2; attempt++ {
		replay := request.Clone(request.Context())
		if attempt > 0 {
			replay.Body, err = request.GetBody()
			if err != nil {
				t.Fatal(err)
			}
		}
		response, err := server.Client().Do(replay)
		if err != nil {
			t.Fatal(err)
		}
		_ = response.Body.Close()
		wire := <-received
		if wire.err != nil {
			t.Fatal(wire.err)
		}
		if wire.encoding != "gzip" || wire.length != request.ContentLength || int64(len(wire.body)) != request.ContentLength {
			t.Fatalf("attempt %d has incorrect wire encoding or length: encoding=%q declared=%d actual=%d", attempt, wire.encoding, wire.length, len(wire.body))
		}
		reader, err := gzip.NewReader(bytes.NewReader(wire.body))
		if err != nil {
			t.Fatal(err)
		}
		decoded, readErr := io.ReadAll(reader)
		closeErr := reader.Close()
		if readErr != nil || closeErr != nil {
			t.Fatalf("cannot decode attempt %d: read=%v close=%v", attempt, readErr, closeErr)
		}
		if !bytes.Equal(decoded, original) {
			t.Fatalf("attempt %d did not preserve the exact original request bytes", attempt)
		}
	}
}

func gzipRequestTestIncompressibleJSON() []byte {
	// Uniform printable JSON-safe ASCII has more than 6.4 bits of entropy
	// per byte, so gzip cannot reach the required 20% reduction.
	random := rand.New(rand.NewSource(7))
	body := []byte(`{"content":"`)
	for len(body) < 1<<20 {
		character := byte(32 + random.Intn(95))
		if character != '"' && character != '\\' {
			body = append(body, character)
		}
	}
	return append(body, '"', '}')
}

func TestGzipLargeClaudeRequestNoOp(t *testing.T) {
	large := gzipRequestTestJSON(1 << 20)
	cases := []struct {
		name        string
		body        []byte
		method      string
		contentType string
		encoding    string
		noReplay    bool
		unknownSize bool
	}{
		{name: "small", body: []byte(`{ "content": "keep these bytes" }`), contentType: "application/json"},
		{name: "below threshold", body: large[:len(large)-1], contentType: "application/json"},
		{name: "pre-encoded", body: large, contentType: "application/json", encoding: "br"},
		{name: "non-JSON", body: large, contentType: "text/plain"},
		{name: "non-POST", body: large, contentType: "application/json", method: http.MethodPut},
		{name: "not replayable", body: large, contentType: "application/json", noReplay: true},
		{name: "unknown length", body: large, contentType: "application/json", unknownSize: true},
		{name: "insufficient savings", body: gzipRequestTestIncompressibleJSON(), contentType: "application/json"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			method := testCase.method
			if method == "" {
				method = http.MethodPost
			}
			request, err := http.NewRequest(method, "https://example.invalid/v1/messages", bytes.NewReader(testCase.body))
			if err != nil {
				t.Fatal(err)
			}
			defer request.Body.Close()
			request.Header.Set("Content-Type", testCase.contentType)
			if testCase.encoding != "" {
				request.Header.Set("Content-Encoding", testCase.encoding)
			}
			if testCase.noReplay {
				request.GetBody = nil
			}
			if testCase.unknownSize {
				request.ContentLength = -1
			}
			originalBody := request.Body
			originalLength := request.ContentLength
			if err := gzipLargeClaudeRequest(request); err != nil {
				t.Fatal(err)
			}
			if request.Body != originalBody || request.ContentLength != originalLength || request.Header.Get("Content-Encoding") != testCase.encoding {
				t.Fatal("ineligible request was changed")
			}
			body, err := io.ReadAll(request.Body)
			if err != nil || !bytes.Equal(body, testCase.body) {
				t.Fatalf("original body was consumed or modified: %v", err)
			}
			if (request.GetBody == nil) != testCase.noReplay {
				t.Fatal("replay availability changed")
			}
		})
	}
}

type gzipRequestTestAdaptor struct {
	Adaptor
	endpoint string
}

func (a gzipRequestTestAdaptor) GetRequestURL(*relaycommon.RelayInfo) (string, error) {
	return a.endpoint, nil
}

func (gzipRequestTestAdaptor) SetupRequestHeader(c *gin.Context, headers *http.Header, info *relaycommon.RelayInfo) error {
	SetupApiRequestHeader(info, c, headers)
	return nil
}

func TestDoApiRequestGzipLargeClaudeRequestsOptIn(t *testing.T) {
	if service.GetHttpClient() == nil {
		service.InitHttpClient()
	}
	t.Cleanup(service.GetHttpClient().CloseIdleConnections)
	original := gzipRequestTestJSON(1 << 20)
	type receivedRequest struct {
		body     []byte
		encoding string
		err      error
	}
	received := make(chan receivedRequest, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		received <- receivedRequest{body: body, encoding: r.Header.Get("Content-Encoding"), err: err}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	cases := []struct {
		name        string
		enabled     bool
		format      types.RelayFormat
		channelType int
		compressed  bool
	}{
		{name: "default unchanged", format: types.RelayFormatClaude, channelType: constant.ChannelTypeAnthropic},
		{name: "native Claude opt-in", enabled: true, format: types.RelayFormatClaude, channelType: constant.ChannelTypeAnthropic, compressed: true},
		{name: "different request format", enabled: true, format: types.RelayFormatOpenAI, channelType: constant.ChannelTypeAnthropic},
		{name: "different channel type", enabled: true, format: types.RelayFormatClaude, channelType: constant.ChannelTypeOpenAI},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(original))
			ctx.Request.Header.Set("Content-Type", "application/json")
			info := &relaycommon.RelayInfo{
				RelayFormat: testCase.format,
				ChannelMeta: &relaycommon.ChannelMeta{
					ChannelType: testCase.channelType,
					ChannelSetting: dto.ChannelSettings{
						GzipLargeClaudeRequests: testCase.enabled,
					},
				},
			}
			response, err := DoApiRequest(gzipRequestTestAdaptor{endpoint: server.URL}, ctx, info, bytes.NewReader(original))
			if err != nil {
				t.Fatal(err)
			}
			_ = response.Body.Close()
			wire := <-received
			if wire.err != nil {
				t.Fatal(wire.err)
			}
			body := wire.body
			if testCase.compressed {
				if wire.encoding != "gzip" || len(body)*5 > len(original)*4 {
					t.Fatalf("native opt-in request was not compressed: encoding=%q length=%d", wire.encoding, len(body))
				}
				reader, err := gzip.NewReader(bytes.NewReader(body))
				if err != nil {
					t.Fatal(err)
				}
				body, err = io.ReadAll(reader)
				_ = reader.Close()
				if err != nil {
					t.Fatal(err)
				}
			} else if wire.encoding != "" {
				t.Fatalf("ineligible request acquired Content-Encoding %q", wire.encoding)
			}
			if !bytes.Equal(body, original) {
				t.Fatal("relay did not preserve the original JSON bytes")
			}
		})
	}
}
