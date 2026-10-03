package record

import (
	"context"
	"errors"
	"io"
	"log"
	"mime"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"time"
)

const (
	maxActiveRequests = 64
	maxHeaderBytes    = 64 << 10
	exchangeTimeout   = 30 * time.Second
	shutdownTimeout   = 5 * time.Second
)

// Serve forwards to the validated target until ctx is canceled. It does not
// capture traffic or create recording files.
func Serve(ctx context.Context, listen string, target *url.URL) (result error) {
	transport := &http.Transport{
		DialContext:            (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		TLSHandshakeTimeout:    5 * time.Second,
		ResponseHeaderTimeout:  10 * time.Second,
		ExpectContinueTimeout:  time.Second,
		IdleConnTimeout:        30 * time.Second,
		MaxIdleConns:           maxActiveRequests,
		MaxIdleConnsPerHost:    maxActiveRequests,
		MaxConnsPerHost:        maxActiveRequests,
		MaxResponseHeaderBytes: maxHeaderBytes,
		DisableCompression:     true,
	}
	quietLog := log.New(io.Discard, "", 0)
	errorLog := log.New(os.Stderr, "prolane record: ", 0)
	proxy := &httputil.ReverseProxy{
		Rewrite:   func(r *httputil.ProxyRequest) { r.SetURL(target) },
		Transport: transport,
		ErrorLog:  quietLog,
		ModifyResponse: func(r *http.Response) error {
			mediaType, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
			if r.StatusCode == http.StatusSwitchingProtocols || mediaType == "text/event-stream" {
				return errors.New("unsupported upstream protocol")
			}
			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			status := http.StatusBadGateway
			var networkError net.Error
			if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &networkError) && networkError.Timeout()) {
				status = http.StatusGatewayTimeout
			}
			if r.Context().Err() == nil {
				errorLog.Print("upstream exchange failed")
			}
			http.Error(w, http.StatusText(status), status)
		},
	}
	active := make(chan struct{}, maxActiveRequests)
	// Keep active requests alive during the grace period; cancel them only when
	// shutdown expires or the server fails.
	requests, cancelRequests := context.WithCancel(context.Background())
	server := &http.Server{
		DisableGeneralOptionsHandler: true,
		ReadHeaderTimeout:            5 * time.Second,
		ReadTimeout:                  exchangeTimeout,
		WriteTimeout:                 exchangeTimeout,
		IdleTimeout:                  30 * time.Second,
		MaxHeaderBytes:               maxHeaderBytes,
		ErrorLog:                     quietLog,
		BaseContext:                  func(net.Listener) context.Context { return requests },
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodConnect || r.RequestURI == "*" || len(r.Header.Values("Upgrade")) != 0 {
				http.Error(w, "tunnels, upgrades, and asterisk-form requests are not supported", http.StatusNotImplemented)
				return
			}
			if r.URL.User != nil || r.URL.Opaque != "" {
				http.Error(w, "unsupported request target", http.StatusBadRequest)
				return
			}
			if _, err := url.ParseQuery(r.URL.RawQuery); err != nil {
				http.Error(w, "invalid query parameters", http.StatusBadRequest)
				return
			}
			select {
			case active <- struct{}{}:
				defer func() { <-active }()
			default:
				http.Error(w, "too many active requests", http.StatusServiceUnavailable)
				return
			}
			requestCtx, cancel := context.WithTimeout(r.Context(), exchangeTimeout)
			defer cancel()
			proxy.ServeHTTP(w, r.WithContext(requestCtx))
		}),
	}
	defer func() {
		cancelRequests()
		if err := server.Close(); err != nil {
			result = errors.Join(result, errors.New("failed to close HTTP server"))
		}
		transport.CloseIdleConnections()
	}()

	listener, err := net.Listen("tcp", listen)
	if err != nil {
		return errors.New("failed to bind HTTP listener")
	}
	served := make(chan error, 1)
	go func() { served <- server.Serve(listener) }()
	select {
	case err := <-served:
		if !errors.Is(err, http.ErrServerClosed) {
			return errors.New("HTTP server stopped unexpectedly")
		}
		return nil
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	shutdownErr := server.Shutdown(shutdownCtx)
	serveErr := <-served
	if shutdownErr != nil {
		return errors.New("graceful shutdown failed; remaining exchanges canceled")
	}
	if !errors.Is(serveErr, http.ErrServerClosed) {
		return errors.New("HTTP server stopped unexpectedly")
	}
	return nil
}
