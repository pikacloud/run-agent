package main

import (
	"fmt"
	"net/http"
	"reflect"
	"testing"

	"github.com/pikacloud/gopikacloud"
)

func createTestAgent() error {
	version = "undefined"
	safeLocaltime := localtime()
	mux.HandleFunc("/v1/run/agents/", func(w http.ResponseWriter, r *http.Request) {
		testMethod(nil, r, "POST")
		fmt.Fprintf(w, "{\"aid\": \"toto\", \"hostname\": \"tata\", \"localtime\": %d}", safeLocaltime)
	})
	agent = NewAgent("foobar", "tata", nil)
	agent.Client = client
	pikacloudClient = gopikacloud.NewClient("tata")
	pikacloudClient.BaseURL = "http://localhost:28002/api/"
	err := agent.Register()
	if err != nil {
		return err
	}
	metrics = &Metrics{}
	streamer = NewStreamer("foobar", false)
	return nil
}

func TestAgentCreate(t *testing.T) {
	setup()
	defer teardown()
	safeLocaltime := localtime()
	mux.HandleFunc("/v1/run/agents/", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "POST")
		fmt.Fprintf(w, "{\"aid\": \"toto\", \"hostname\": \"tata\", \"localtime\": %d}", safeLocaltime)
	})
	agent = NewAgent("", "tata", nil)
	pikacloudClient = gopikacloud.NewClient("tata")
	pikacloudClient.BaseURL = "http://localhost:28002/api/"
	agent.Client = client
	err := agent.Register()
	if err != nil {
		t.Errorf("Cannot create agent: %v", err)
	}
	if agent.Localtime == 0 || agent.Localtime < (localtime()-60) {
		t.Errorf("Agent Localtime not set correctly: %v", agent.Localtime)
	}
	if agent.ID != "toto" {
		t.Errorf("Agent ID is %v, want %v", agent.ID, "toto")
	}
	if agent.Hostname != "tata" {
		t.Errorf("Agent Hostname=%v, want %v", agent.Hostname, "tata")
	}
}

func TestMakeLabels(t *testing.T) {
	labelsStr := "foo,bar        , cluster=redis,test a,   a     c"
	res := makeLabels(labelsStr)
	if len(res) != 5 {
		t.Errorf("We should have 5 labels, got %d: %+v", len(res), res)
	}
	want := []string{"foo", "bar", "cluster=redis", "test_a", "a_c"}
	if !reflect.DeepEqual(want, res) {
		t.Errorf("Labels array is %v, want %v", res, want)
	}
	labelsStr = ""
	res = makeLabels(labelsStr)
	if len(res) != 0 {
		t.Errorf("We should have 0 labels, got %d: %+v", len(res), res)
	}
}

func TestAgentPing(t *testing.T) {
	setup()
	defer teardown()

	mux.HandleFunc("/v1/run/agents/toto/ping/", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "POST")
		fmt.Fprint(w, `{"status": "OK"}`)
	})
	err := createTestAgent()
	if err != nil {
		t.Errorf("Cannot create agent: %v", err)
	}

	errPing := agent.Ping()

	if errPing != nil {
		t.Errorf("Ping test fails: %v", errPing)
	}
}

func TestAgentLatestVersion(t *testing.T) {
	setup()
	defer teardown()
	// testURI := fmt.Sprintf("/v1/run/agent_version/latest/?from=%s&os=%s&arch=%s", version, runtime.GOOS, runtime.GOARCH)
	mux.HandleFunc("/v1/run/agent-version/latest/", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "GET")
		fmt.Fprint(w, `{"version": "1.0.1", "archive_url":"http://foo.bar/agent.tar.gz"}`)
	})
	err := createTestAgent()
	if err != nil {
		t.Errorf("Cannot create agent: %v", err)
	}
	v, err := agent.getLatestVersion()
	if err != nil {
		t.Errorf("Cannot fetch latest version %+v", err)
	}
	if v.Version != "1.0.1" {
		t.Errorf("Version is %v, want %v", v.Version, "1.0.1")
	}
}
