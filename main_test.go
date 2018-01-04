package main

import (
	"testing"
)

func TestDeleteRunningTasks(t *testing.T) {
	runningTasksList = make(map[string]*Task)
	runningTasksList["t1"] = &Task{}
	runningTasksList["t2"] = &Task{}
	deleteRunningTasks("t1")
	if runningTasksList["t1"] != nil {
		t.Errorf("t1 found in %v", runningTasksList)
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
