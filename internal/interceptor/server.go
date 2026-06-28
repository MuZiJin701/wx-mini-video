package interceptor

import (
	"context"
	"fmt"
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
	if err := s.Interceptor.Start(); err != nil {
		return fmt.Errorf("failed to start interceptor: %v", err)
	}
	go func() {
		err := s.server.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			s.errCh <- err
		}
	}()
	select {
	case err := <-s.errCh:
		return err
	case <-time.After(200 * time.Millisecond):
	}
	return nil
}

func (s *InterceptorServer) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.Interceptor.Stop(); err != nil {
		return fmt.Errorf("failed to stop interceptor: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return s.server.Shutdown(ctx)
}
