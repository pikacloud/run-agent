package main

import (
	"encoding/json"
	"fmt"
	"log"
)

// TaskStep describes a step of a task
type TaskStep struct {
	Plugin            string          `json:"plugin"`
	PluginConfig      json.RawMessage `json:"plugin_options"`
	Method            string          `json:"method"`
	ExitOnFailure     bool            `json:"exit_on_failure"`
	WaitForCompletion bool            `json:"wait_for_completion"`
	Task              *Task
}

// Task descibres a task
type Task struct {
	ID      string      `json:"tid"`
	Steps   []*TaskStep `json:"payload"`
	NeedACK bool        `json:"need_ack"`
}

// TaskACKStep represent a task step ACK
type TaskACKStep struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// TaskACK reprensent a tack ACK
type TaskACK struct {
	TaskACKStep []*TaskACKStep `json:"results"`
}

// Do a step
func (step *TaskStep) Do() error {
	switch step.Plugin {
	case "docker":
		return step.Docker()
	default:
		return fmt.Errorf("Unknown step plugin %s", step.Plugin)
	}
}

// Do a task
func (task *Task) Do() (TaskACK, error) {
	ack := TaskACK{}
	for _, step := range task.Steps {
		ackStep := TaskACKStep{}
		step.Task = task
		err := step.Do()
		if err != nil {
			log.Printf("%s Step %s failed with config %s (%s)", step.Plugin, step.Method, step.PluginConfig, err)
			ackStep.Message = err.Error()
			ackStep.Success = false
		} else {
			ackStep.Success = true
			ackStep.Message = "OK"
		}
		ack.TaskACKStep = append(ack.TaskACKStep, &ackStep)
	}
	return ack, nil
}
