package canvas

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

func TestClientPaginatesOnlyAtConfiguredOrigin(t *testing.T) {
	requests := 0
	var server *httptest.Server
	server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Header.Get("Authorization") != "Bearer token" {
			t.Fatalf("authorization = %q", r.Header.Get("Authorization"))
		}
		if r.URL.Query().Get("page") == "2" {
			_, _ = w.Write([]byte(`[{"id":2}]`))
			return
		}
		w.Header().Set("Link", fmt.Sprintf("<%s/items?page=2>; rel=\"next\"", server.URL))
		_, _ = w.Write([]byte(`[{"id":1}]`))
	}))
	defer server.Close()
	client, err := NewClient(server.URL, "token")
	if err != nil {
		t.Fatal(err)
	}
	client.http = server.Client()
	items, err := client.GetAll(context.Background(), "/items", url.Values{})
	if err != nil || len(items) != 2 || requests != 2 {
		t.Fatalf("GetAll() = %#v, %v; requests=%d", items, err, requests)
	}
}

func TestClientRejectsForeignPaginationLink(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Link", `<https://elsewhere.invalid/items>; rel="next"`)
		_, _ = w.Write([]byte(`[]`))
	}))
	defer server.Close()
	client, err := NewClient(server.URL, "token")
	if err != nil {
		t.Fatal(err)
	}
	client.http = server.Client()
	if _, err := client.GetAll(context.Background(), "/items", nil); err == nil {
		t.Fatal("GetAll accepted foreign pagination URL")
	}
}

func TestClientRedactsVerifierFromTransportErrors(t *testing.T) {
	client, err := NewClient("https://canvas.example.edu", "token")
	if err != nil {
		t.Fatal(err)
	}
	client.http = &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		return nil, fmt.Errorf("dial failed for %s", request.URL.String())
	})}
	err = client.Download(context.Background(), "https://assets.example.edu/file.pdf?verifier=super-secret&download=1", filepath.Join(t.TempDir(), "file.pdf"))
	if err == nil {
		t.Fatal("Download() succeeded")
	}
	if strings.Contains(err.Error(), "super-secret") || strings.Contains(err.Error(), "verifier") {
		t.Fatalf("transport error exposed attachment secret: %v", err)
	}
}

func TestClientRejectsRepeatedPaginationURL(t *testing.T) {
	var requests atomic.Int32
	var server *httptest.Server
	server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.Header().Set("Link", fmt.Sprintf("<%s/items>; rel=\"next\"", server.URL))
		_, _ = w.Write([]byte(`[{"id":1}]`))
	}))
	defer server.Close()
	client, err := NewClient(server.URL, "token")
	if err != nil {
		t.Fatal(err)
	}
	client.http = server.Client()
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	_, err = client.GetAll(ctx, "/items", nil)
	if err == nil || !strings.Contains(err.Error(), "pagination") {
		t.Fatalf("GetAll() error = %v", err)
	}
	if got := requests.Load(); got > 2 {
		t.Fatalf("repeated pagination made %d requests", got)
	}
}

func TestClientRejectsEmptyPaginationPageWithNextLink(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("page") == "2" {
			_, _ = w.Write([]byte(`[]`))
			return
		}
		w.Header().Set("Link", fmt.Sprintf("<%s/items?page=2>; rel=\"next\"", server.URL))
		_, _ = w.Write([]byte(`[]`))
	}))
	defer server.Close()
	client, err := NewClient(server.URL, "token")
	if err != nil {
		t.Fatal(err)
	}
	client.http = server.Client()
	if _, err := client.GetAll(context.Background(), "/items", nil); err == nil || !strings.Contains(err.Error(), "pagination") {
		t.Fatalf("GetAll() error = %v", err)
	}
}

func TestClientBoundsPaginationPages(t *testing.T) {
	var requests atomic.Int32
	var server *httptest.Server
	server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page := requests.Add(1)
		w.Header().Set("Link", fmt.Sprintf("<%s/items?page=%d>; rel=\"next\"", server.URL, page+1))
		_, _ = w.Write([]byte(fmt.Sprintf(`[{"id":%d}]`, page)))
	}))
	defer server.Close()
	client, err := NewClient(server.URL, "token")
	if err != nil {
		t.Fatal(err)
	}
	client.http = server.Client()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err = client.GetAll(ctx, "/items", nil)
	if err == nil || !strings.Contains(err.Error(), "pagination") || errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("GetAll() error = %v", err)
	}
	if got := requests.Load(); got > 1000 {
		t.Fatalf("pagination was not bounded: %d requests", got)
	}
}
