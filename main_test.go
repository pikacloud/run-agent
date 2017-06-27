package main

import (
	"testing"
)

func TestNewClient(t *testing.T) {
	c := NewClient("mytoken")

	if c.BaseURL != defaultBaseURL {
		t.Errorf("NewClient BaseURL = %v, want %v", c.BaseURL, defaultBaseURL)
	}
}

func TestDeleteRunningTasks(t *testing.T) {
	runningTasksList = append(runningTasksList, "t1", "t2")
	deleteRunningTasks("t1")
	for _, task := range runningTasksList {
		if task == "t1" {
			t.Errorf("t1 found in %v", runningTasksList)
		}
	}

}

func TestPluralize(t *testing.T) {
	s := pluralize(3)
	if s != "s" {
		t.Errorf("3 should give a plural, got %v", s)
	}
	s = pluralize(1)
	if s != "" {
		t.Errorf("1 do not give a plural, got %v", s)
	}
	s = pluralize(0)
	if s != "s" {
		t.Errorf("0 should give a plural, got %v", s)
	}
}
