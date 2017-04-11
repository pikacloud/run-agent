package main

import (
	"fmt"
	"net/http"
	"testing"
)

func TestNewClient(t *testing.T) {
	c := NewClient("mytoken")

	if c.BaseURL != defaultBaseURL {
		t.Errorf("NewClient BaseURL = %v, want %v", c.BaseURL, defaultBaseURL)
	}
}

func TestAgentCreate(t *testing.T) {
	setup()
	defer teardown()

	mux.HandleFunc("/v1/run/agents/", func(w http.ResponseWriter, r *http.Request) {
		want := make(map[string]interface{})
		want = map[string]interface{}{"hostname": "tata"}

		testMethod(t, r, "POST")
		fmt.Fprint(w, `{"aid": "toto", "hostname": "tata"}`)

		testRequestJSON(t, r, want)

	})
	a := Agent{
		Client: client,
	}
	newAgentOpts := CreateAgentOptions{
		Hostname: "tata",
	}
	a.Create(&newAgentOpts)
	if a.ID != "toto" {
		t.Errorf("Agent ID=%v, want %v", a.ID, "toto")
	}
	if a.Hostname != "tata" {
		t.Errorf("Agent Hostname=%v, want %v", a.Hostname, "tata")
	}
}

func TestAgentPing(t *testing.T) {
	setup()
	defer teardown()

	mux.HandleFunc("/v1/run/agents/toto/ping/", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "POST")
		fmt.Fprint(w, `{"status": "OK"}`)
	})
	a := Agent{
		Client: client,
		ID:     "toto",
	}
	err := a.Ping()
	if err != nil {
		t.Errorf("Ping test fails: %v", err)
	}
}
