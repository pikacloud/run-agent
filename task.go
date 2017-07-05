package main

import (
	"encoding/json"
	"fmt"
	"log"
)

// TaskStep describes a step of a task
type TaskStep struct {
	Plugin              string          `json:"plugin"`
	PluginConfig        json.RawMessage `json:"plugin_options"`
	Method              string          `json:"method"`
	ExitOnFailure       bool            `json:"exit_on_failure"`
	WaitForCompletion   bool            `json:"wait_for_completion"`
	AckBeforeCompletion bool            `json:"ack_before_completion"`
	Task                *Task
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
	case "system":
		return step.System()
	case "docker":
		return step.Docker()
	default:
		return fmt.Errorf("Unknown step plugin %s", step.Plugin)
	}
}

// Do a task
func (task *Task) Do() error {
	ack := TaskACK{}
	alreayAcked := false
	for _, step := range task.Steps {
		ackStep := TaskACKStep{}
		step.Task = task
		if step.AckBeforeCompletion && task.NeedACK {
			err := agent.ackTask(task, &ack)
			if err != nil {
				log.Printf("Unable to ack task %s before running step: %s", task.ID, err)
			} else {
				alreayAcked = true
				deleteRunningTasks(task.ID)
				log.Printf("task %s ACKed before running step", task.ID)
			}
		}
		err := step.Do()
		if err != nil {
			log.Printf("%s Step %s failed with config %s (%s)", step.Plugin, step.Method, step.PluginConfig, err)
			ackStep.Message = err.Error()
			ackStep.Success = false
			if step.ExitOnFailure {
				ack.TaskACKStep = append(ack.TaskACKStep, &ackStep)
				break
			}
		} else {
			ackStep.Success = true
			ackStep.Message = "OK"
		}
		ack.TaskACKStep = append(ack.TaskACKStep, &ackStep)
	}
	if task.NeedACK && alreayAcked == false {
		err := agent.ackTask(task, &ack)
		if err != nil {
			log.Printf("Unable to ack task %s: %s", task.ID, err)
		} else {
			log.Printf("task %s ACKed", task.ID)
		}
		deleteRunningTasks(task.ID)
	}
	return nil
}
