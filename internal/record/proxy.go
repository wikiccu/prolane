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

type exchangeContextKey struct{}

// Serve forwards to the validated target and writes metadata to a new output
// file until ctx is canceled or recording fails. Headers and bodies are omitted.
func Serve(ctx context.Context, listen string, target *url.URL, output string) (result error) {
	listener, err := net.Listen("tcp", listen)
	if err != nil {
		return errors.New("failed to bind HTTP listener")
	}
	defer func() {
		if err := listener.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
			result = errors.Join(result, errors.New("failed to close HTTP listener"))
		}
	}()
	file, err := os.OpenFile(output, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return errors.New("failed to create recording file; use a new path in an existing writable directory")
	}
	recording := &recording{file: file, failed: make(chan struct{})}
	defer func() { result = errors.Join(result, recording.close()) }()
	if info, err := file.Stat(); err != nil || !info.Mode().IsRegular() {
		return errors.New("recording output must be a regular file")
	}

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
			exchange := r.Request.Context().Value(exchangeContextKey{}).(*Exchange)
			exchange.Response = &ResponseMetadata{
				StatusCode: r.StatusCode, HeadersOmitted: true, BodyOmitted: true,
			}
			mediaType, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
			if r.StatusCode == http.StatusSwitchingProtocols || mediaType == "text/event-stream" {
				return errors.New("unsupported upstream protocol")
			}
			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			exchange := r.Context().Value(exchangeContextKey{}).(*Exchange)
			exchange.Failure = classifyFailure(errors.Join(err, r.Context().Err()))
			status := http.StatusBadGateway
			if exchange.Failure == FailureTimeout {
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
			path := r.URL.EscapedPath()
			if len(r.Method)+len(path) > maxMetadataBytes {
				http.Error(w, "request metadata exceeds recording limit", http.StatusRequestURITooLong)
				return
			}
			select {
			case active <- struct{}{}:
				defer func() { <-active }()
			default:
				http.Error(w, "too many active requests", http.StatusServiceUnavailable)
				return
			}
			id, admitted := recording.admit()
			if !admitted {
				http.Error(w, "recording is stopping", http.StatusServiceUnavailable)
				return
			}
			defer recording.handlers.Done()
			started := time.Now()
			exchange := Exchange{
				SchemaVersion: SchemaVersion,
				ID:            id,
				StartedAt:     started.UTC(),
				Request: RequestMetadata{
					Method: r.Method, Path: path,
					QueryOmitted:   r.URL.RawQuery != "" || r.URL.ForceQuery,
					HeadersOmitted: true, BodyOmitted: true,
				},
			}
			requestCtx, cancel := context.WithTimeout(r.Context(), exchangeTimeout)
			defer cancel()
			completed := false
			defer func() {
				exchange.DurationNS = time.Since(started).Nanoseconds()
				if err := requestCtx.Err(); err != nil {
					exchange.Failure = classifyFailure(err)
				} else if !completed {
					// ReverseProxy aborts interrupted transfers with ErrAbortHandler.
					// Persist the failure while allowing that panic to propagate.
					exchange.Failure = FailureIncompleteResponse
				} else if exchange.Response == nil && exchange.Failure == "" {
					exchange.Failure = FailureUpstream
				}
				recording.write(&exchange)
			}()
			requestCtx = context.WithValue(requestCtx, exchangeContextKey{}, &exchange)
			proxy.ServeHTTP(w, r.WithContext(requestCtx))
			completed = true
		}),
	}
	defer func() {
		cancelRequests()
		if err := server.Close(); err != nil {
			result = errors.Join(result, errors.New("failed to close HTTP server"))
		}
		transport.CloseIdleConnections()
	}()

	served := make(chan error, 1)
	go func() { served <- server.Serve(listener) }()
	select {
	case err := <-served:
		if !errors.Is(err, http.ErrServerClosed) {
			return errors.New("HTTP server stopped unexpectedly")
		}
		return nil
	case <-ctx.Done():
	case <-recording.failed:
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

func classifyFailure(err error) FailureCode {
	if errors.Is(err, context.DeadlineExceeded) {
		return FailureTimeout
	}
	if errors.Is(err, context.Canceled) {
		return FailureCanceled
	}
	var networkError net.Error
	if errors.As(err, &networkError) && networkError.Timeout() {
		return FailureTimeout
	}
	return FailureUpstream
}
