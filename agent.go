package main

import "strings"

// CreateAgentOptions represents the agent Create() options
type CreateAgentOptions struct {
	Hostname string   `json:"hostname"`
	Labels   []string `json:"labels,omitempty"`
}

func makeLabels(labels string) []string {
	return strings.Split(labels, ",")
}
