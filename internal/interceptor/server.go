package interceptor

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"wx_channel/pkg/certificate"
)

type InterceptorServer struct {
	Interceptor *Interceptor
	server      *http.Server
	mu          sync.Mutex
	errCh       chan error
}

func NewInterceptorServer(settings *InterceptorConfig, cert *certificate.CertFileAndKeyFile) *InterceptorServer {
	interceptor := NewInterceptor(settings, cert)
	addr := settings.ProxyServerHostname + ":" + strconv.Itoa(settings.ProxyServerPort)
	return &InterceptorServer{
		Interceptor: interceptor,
		server: &http.Server{
			Addr:              addr,
			Handler:           interceptor,
			ReadHeaderTimeout: 10 * time.Second,
		},
		errCh: make(chan error, 1),
	}
}

func (s *InterceptorServer) Start() error {
	listener, err := net.Listen("tcp", s.server.Addr)
	if err != nil {
		return fmt.Errorf("代理端口不可用 %s: %w", s.server.Addr, err)
	}
	if err := s.Interceptor.Start(); err != nil {
		return errors.Join(fmt.Errorf("failed to start interceptor: %w", err), listener.Close())
	}
	go func() {
		err := s.server.Serve(listener)
		if err != nil && err != http.ErrServerClosed {
			s.errCh <- err
		}
	}()
	select {
	case err := <-s.errCh:
		return errors.Join(err, s.Stop())
	case <-time.After(200 * time.Millisecond):
	}
	return nil
}

func (s *InterceptorServer) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return errors.Join(s.Interceptor.Stop(), s.server.Shutdown(ctx))
}
