package main

import (
	"fmt"
)

// System describes available methods of the system plugin
func (step *TaskStep) System() error {
	switch step.Method {
	case "shutdown":
		shutdown()
		return nil
	default:
		return fmt.Errorf("Unknown step method %s", step.Method)
	}
}
