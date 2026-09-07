package jira

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os/exec"
	"runtime"
	"sync"
	"time"
)

var ErrAuthorizationTimeout = errors.New("Atlassian authorization timed out")

type authorizationResult struct {
	Code  string
	State string
	Iss   string
}

type loopbackCallback struct {
	listener net.Listener
	server   *http.Server
	result   chan callbackOutcome
	once     sync.Once
	out      io.Writer
	opener   func(string) error
	onURL    func(string)
	timeout  time.Duration
}

type callbackOutcome struct {
	result *authorizationResult
	err    error
}

func newLoopbackCallback(out io.Writer, opener func(string) error, onURL func(string), timeout time.Duration) (*loopbackCallback, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("start OAuth callback: %w", err)
	}
	c := &loopbackCallback{listener: listener, result: make(chan callbackOutcome, 1), out: out, opener: opener, onURL: onURL, timeout: timeout}
	c.server = &http.Server{Handler: http.HandlerFunc(c.serveHTTP)}
	go func() { _ = c.server.Serve(listener) }()
	return c, nil
}

func (c *loopbackCallback) URL() string { return "http://" + c.listener.Addr().String() + "/callback" }

func (c *loopbackCallback) serveHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/callback" {
		http.NotFound(w, r)
		return
	}
	if r.URL.Query().Get("error") != "" || r.URL.Query().Get("code") == "" {
		c.finish(callbackOutcome{err: errors.New("Atlassian authorization was denied")})
		http.Error(w, "Corum could not complete authorization. You may close this tab.", http.StatusBadRequest)
		return
	}
	c.finish(callbackOutcome{result: &authorizationResult{Code: r.URL.Query().Get("code"), State: r.URL.Query().Get("state"), Iss: r.URL.Query().Get("iss")}})
	_, _ = io.WriteString(w, "Corum is connected to Atlassian. You may close this tab.")
}

func (c *loopbackCallback) finish(outcome callbackOutcome) { c.once.Do(func() { c.result <- outcome }) }

func (c *loopbackCallback) Fetch(ctx context.Context, authorizationURL string) (*authorizationResult, error) {
	fmt.Fprintf(c.out, "Open this URL to connect Atlassian:\n%s\n", authorizationURL)
	if c.onURL != nil {
		c.onURL(authorizationURL)
	}
	if c.opener != nil {
		_ = c.opener(authorizationURL)
	}
	timer := time.NewTimer(c.timeout)
	defer timer.Stop()
	select {
	case outcome := <-c.result:
		return outcome.result, outcome.err
	case <-timer.C:
		return nil, ErrAuthorizationTimeout
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (c *loopbackCallback) Close() error { return c.server.Close() }

func defaultBrowserOpen(url string) error {
	command := "xdg-open"
	if runtime.GOOS == "darwin" {
		command = "open"
	}
	return exec.Command(command, url).Start()
}
