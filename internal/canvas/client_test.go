package canvas

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

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
