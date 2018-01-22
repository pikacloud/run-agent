package main

import (
	"encoding/json"
	"fmt"
	"os"
)

// CancelTaskOpts describes the cancel task options
type CancelTaskOpts struct {
	Tid string `json:"tid"`
}

// ShutdownOpts describes agent shutdown options
type ShutdownOpts struct {
	Reason   string `json:"reason"`
	WithCode int    `json:"with_code"`
}

func shutdown(exitCode int, reason string) {
	logger.Infof("Shutting down run-agent (%s)...", reason)
	os.Exit(exitCode)
}

// System describes available methods of the system plugin
func (step *TaskStep) System() error {
	switch step.Method {
	case "cancel_task":
		var cancelOpts = CancelTaskOpts{}
		err := json.Unmarshal([]byte(step.PluginConfig), &cancelOpts)
		if err != nil {
			return fmt.Errorf("Bad config for cancel_task: %s (%v)", err, step.PluginConfig)
		}
		task := runningTasksList[cancelOpts.Tid]
		if task == nil {
			return fmt.Errorf("Cannot cancel task %s, not found as running", task.ID)

		}
		errCancel := task.cancel()
		if errCancel != nil {
			return fmt.Errorf("Cannot cancel task %s: %s", task.ID, err)
		}
		return nil
	case "shutdown":
		var shutdownOpts = ShutdownOpts{}
		err := json.Unmarshal([]byte(step.PluginConfig), &shutdownOpts)
		if err != nil {
			return fmt.Errorf("Bad config for shutdown: %s (%v)", err, step.PluginConfig)
		}
		shutdown(shutdownOpts.WithCode, shutdownOpts.Reason)
		return nil
	default:
		return fmt.Errorf("Unknown step method %s", step.Method)
	}
}
