package main

import "strings"

// CreateAgentOptions represents the agent Create() options
type CreateAgentOptions struct {
	Hostname string   `json:"hostname"`
	Labels   []string `json:"labels,omitempty"`
}

// PingAgentOptions represents the agent Ping() options
type PingAgentOptions struct {
	RunningTasks []string `json:"running_tasks,omitempty"`
}

func makeLabels(labels string) []string {
	return strings.Split(labels, ",")
}
