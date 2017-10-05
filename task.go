package main

import (
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/gorilla/websocket"
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
	ResultMessage       string
}

// Task descibres a task
type Task struct {
	ID            string      `json:"tid"`
	Steps         []*TaskStep `json:"payload"`
	NeedACK       bool        `json:"need_ack"`
	Stream        bool        `json:"stream"`
	streamMsg     chan []byte
	websocketConn *websocket.Conn
	LogWriter     *io.PipeWriter
	LogReader     *io.PipeReader
	cancelCh      chan bool
	streamer      *Streamer
}

// TaskACKStep represent a task step ACK
type TaskACKStep struct {
	Success        bool   `json:"success"`
	Message        string `json:"message"`
	StartTimestamp int64  `json:"start_timestamp"`
	EndTimestamp   int64  `json:"end_timestamp"`
}

// TaskACK reprensent a tack ACK
type TaskACK struct {
	TaskACKStep []*TaskACKStep `json:"results"`
}

func deleteRunningTasks(tid string) {
	delete(runningTasksList, tid)
}

func (task *Task) cancel() error {
	logger.Debugf("1/2 Trying to cancel task %s", task.ID)

	task.cancelCh <- true
	logger.Debugf("2/2 Trying to cancel task %s", task.ID)
	return nil
}

// Do a step
func (step *TaskStep) Do() error {
	switch step.Plugin {
	case "system":
		return step.System()
	case "docker":
		return step.Docker()
	case "git":
		return step.Git()
	default:
		return fmt.Errorf("Unknown step plugin %s", step.Plugin)
	}
}

func (step *TaskStep) streamErr(msg string) {
	step.stream([]byte(fmt.Sprintf("\033[0;31m%s\033[0m", msg)))
}

func (step *TaskStep) stream(msg []byte) {
	step.Task.streamer.writeRawMsg(msg)
}

// Do a task
func (task *Task) Do() error {
	defer close(task.cancelCh)
	defer deleteRunningTasks(task.ID)
	ack := TaskACK{}
	alreadyAcked := false
	if task.Stream {
		task.streamer = NewStreamer(fmt.Sprintf("tid:%s", task.ID), true)
		defer task.streamer.destroy()
		go task.streamer.run()
	}
	for _, step := range task.Steps {
		ackStep := TaskACKStep{}
		ackStep.StartTimestamp = time.Now().Unix()
		step.Task = task
		if step.AckBeforeCompletion && task.NeedACK {
			err := agent.ackTask(task, &ack)
			if err != nil {
				logger.Errorf("Unable to ack task %s before running step: %s", task.ID, err)
			} else {
				logger.Infof("task %s ACKed before running step", task.ID)
			}
			alreadyAcked = true
		}
		err := step.Do()
		if err != nil {
			logger.Errorf("%s Step %s failed with config %s (%s)", step.Plugin, step.Method, step.PluginConfig, err)
			ackStep.Message = err.Error()
			ackStep.Success = false
			if step.Task.Stream {
				step.streamErr(ackStep.Message + "\n")
			}
			ackStep.EndTimestamp = time.Now().Unix()
			if step.ExitOnFailure {
				ack.TaskACKStep = append(ack.TaskACKStep, &ackStep)
				break
			}
		} else {
			ackStep.Success = true
			if step.ResultMessage != "" {
				ackStep.Message = step.ResultMessage

			} else {
				ackStep.Message = "OK"
			}

		}
		ackStep.EndTimestamp = time.Now().Unix()
		ack.TaskACKStep = append(ack.TaskACKStep, &ackStep)
	}
	if task.NeedACK && alreadyAcked == false {
		err := agent.ackTask(task, &ack)
		if err != nil {
			logger.Errorf("Unable to ack task %s: %s", task.ID, err)
		} else {
			logger.Infof("task %s ACKed", task.ID)
		}
	}
	return nil
}
