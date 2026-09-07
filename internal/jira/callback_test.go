package jira

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestCallbackOnlyAcceptsExactPathAndReturnsCodeFields(t *testing.T) {
	callback, err := newLoopbackCallback(io.Discard, func(string) error { return nil }, nil, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer callback.Close()
	bad, err := http.Get(callback.URL() + "elsewhere")
	if err != nil || bad.StatusCode != http.StatusNotFound {
		t.Fatalf("other path = %v, %v", bad, err)
	}
	result := make(chan *authorizationResult, 1)
	go func() {
		value, err := callback.Fetch(context.Background(), "https://auth.example/authorize")
		if err != nil {
			t.Errorf("Fetch: %v", err)
			return
		}
		result <- value
	}()
	resp, err := http.Get(callback.URL() + "?code=code-value&state=state-value&iss=https%3A%2F%2Fissuer.example")
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("callback = %v, %v", resp, err)
	}
	got := <-result
	if got.Code != "code-value" || got.State != "state-value" || got.Iss != "https://issuer.example" {
		t.Fatalf("result = %+v", got)
	}
}

func TestCallbackProviderErrorAndTimeoutAreRedacted(t *testing.T) {
	callback, err := newLoopbackCallback(io.Discard, func(string) error { return nil }, nil, 20*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	defer callback.Close()
	_, err = callback.Fetch(context.Background(), "https://auth.example/authorize")
	if !errors.Is(err, ErrAuthorizationTimeout) {
		t.Fatalf("timeout = %v", err)
	}
}

func TestCallbackProviderErrorIsRedacted(t *testing.T) {
	callback, err := newLoopbackCallback(io.Discard, func(string) error { return nil }, nil, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer callback.Close()
	result := make(chan error, 1)
	go func() {
		_, err := callback.Fetch(context.Background(), "https://auth.example/authorize")
		result <- err
	}()
	_, _ = http.Get(callback.URL() + "?error=access_denied&error_description=raw-sentinel")
	err = <-result
	if err == nil || strings.Contains(err.Error(), "raw-sentinel") {
		t.Fatalf("provider error = %v", err)
	}
}

func TestCallbackFirstResultWins(t *testing.T) {
	callback, err := newLoopbackCallback(io.Discard, func(string) error { return nil }, nil, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer callback.Close()
	got := make(chan *authorizationResult, 1)
	go func() {
		result, err := callback.Fetch(context.Background(), "https://auth.example/authorize")
		if err != nil {
			t.Errorf("Fetch: %v", err)
			return
		}
		got <- result
	}()
	for _, code := range []string{"first", "second"} {
		_, _ = http.Get(callback.URL() + "?code=" + code)
	}
	if value := <-got; value.Code != "first" {
		t.Fatalf("code = %q, want first", value.Code)
	}
}
