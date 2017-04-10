package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

var (
	mux    *http.ServeMux
	client *Client
	server *httptest.Server
)

// This method of testing http client APIs is borrowed from
// Will Norris's work in go-github @ https://github.com/google/go-github
func setup() {
	mux = http.NewServeMux()
	server = httptest.NewServer(mux)

	client = NewClient("mytoken")
	client.BaseURL = server.URL + "/"
}

func teardown() {
	server.Close()
}

func testMethod(t *testing.T, r *http.Request, want string) {
	if want != r.Method {
		t.Errorf("Request method = %v, want %v", r.Method, want)
	}
}

func TestMakeRequest(t *testing.T) {
	c := NewClient("mytoken")
	c.BaseURL = defaultBaseURL

	inURL, outURL := "foo", "https://pikacloud.com/api/v1/foo"
	req, _ := c.makeRequest("GET", inURL, nil)

	// test that relative URL was expanded with the proper BaseURL
	if req.URL.String() != outURL {
		t.Errorf("NewRequest(%v) URL = %v, want %v", inURL, req.URL, outURL)
	}
}
