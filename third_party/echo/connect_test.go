package echo

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestHandlePlainHTTPTunnelRunsResponseHooks(t *testing.T) {
	var requestLog bytes.Buffer
	loader, err := NewPluginLoader([]*Plugin{
		{
			Match:  "**.qq.com",
			Target: &TargetConfig{Protocol: "http", Host: "upstream.test", Port: 80},
			OnRequest: func(ctx *Context) {
				fmt.Fprintf(&requestLog, "%s %s", ctx.Req.Method, ctx.Req.URL.String())
			},
			OnResponse: func(ctx *Context) {
				if ctx.Res == nil || !strings.Contains(ctx.Res.Header.Get("Content-Type"), "text/html") {
					return
				}
				body, err := ctx.GetResponseBody()
				if err != nil {
					t.Errorf("read response body: %v", err)
					return
				}
				ctx.SetResponseBody(strings.Replace(body, "<title>before</title>", "<title>after</title>", 1))
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	httpHandler := NewHTTPHandler(loader)
	httpHandler.Transport.Proxy = nil
	httpHandler.Transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		clientConn, serverConn := net.Pipe()
		go servePipeHTTPResponse(serverConn, `<html><head><title>before</title></head><body>ok</body></html>`)
		return clientConn, nil
	}
	connectHandler := &ConnectHandler{
		PluginLoader: loader,
		HTTPHandler:  httpHandler,
	}

	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	deadline := time.Now().Add(5 * time.Second)
	_ = clientConn.SetDeadline(deadline)
	_ = serverConn.SetDeadline(deadline)

	done := make(chan struct{})
	go func() {
		defer close(done)
		connectHandler.handlePlainHTTPTunnel(serverConn, bufio.NewReader(serverConn), "weixin.qq.com", "80")
	}()

	if _, err := io.WriteString(clientConn, "GET / HTTP/1.1\r\nHost: weixin.qq.com\r\nConnection: close\r\n\r\n"); err != nil {
		t.Fatal(err)
	}

	resp, err := http.ReadResponse(bufio.NewReader(clientConn), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}

	if body := string(bodyBytes); !strings.Contains(body, "<title>after</title>") {
		t.Fatalf("response hook did not rewrite title: %s", body)
	}
	if got := requestLog.String(); got != "GET http://weixin.qq.com/" {
		t.Fatalf("request hook log: got %q", got)
	}

	<-done
}

func servePipeHTTPResponse(conn net.Conn, body string) {
	defer conn.Close()

	req, err := http.ReadRequest(bufio.NewReader(conn))
	if err != nil {
		return
	}
	_, _ = io.Copy(io.Discard, req.Body)
	_ = req.Body.Close()

	resp := &http.Response{
		Status:        "200 OK",
		StatusCode:    http.StatusOK,
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Header:        make(http.Header),
		Body:          io.NopCloser(strings.NewReader(body)),
		ContentLength: int64(len(body)),
		Request:       req,
	}
	resp.Header.Set("Content-Type", "text/html; charset=utf-8")
	resp.Header.Set("Content-Length", fmt.Sprintf("%d", len(body)))
	_ = resp.Write(conn)
}
