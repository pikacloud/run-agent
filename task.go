package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/url"
	"strings"
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
	websocketConn *websocket.Conn
	LogWriter     *io.PipeWriter
	LogReader     *io.PipeReader
	cancelCh      chan bool
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
	// for idx, t := range runningTasksList {
	// 	if t == tid {
	// 		runningTasksList = runningTasksList[:idx+copy(runningTasksList[idx:], runningTasksList[idx+1:])]
	// 	}
	// }
}

func (task *Task) cancel() error {
	log.Printf("1/2 Trying to cancel task %s", task.ID)

	task.cancelCh <- true
	log.Printf("2/2 Trying to cancel task %s", task.ID)
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

func (step *TaskStep) stream(msg string) {
	if step.Task.Stream {
		step.Task.LogWriter.Write([]byte(msg))
	}
}

func (task *Task) streaming() error {
	r, w := io.Pipe()
	task.LogReader = r
	task.LogWriter = w

	wsURLParams := strings.Split(wsURL, "://")
	scheme := wsURLParams[0]
	addr := wsURLParams[1]
	path := fmt.Sprintf("/_ws/hub/log-writer/tid:%s/", task.ID)
	u := url.URL{Scheme: scheme, Host: addr, Path: path}
	dialer := websocket.DefaultDialer
	dialer.HandshakeTimeout = 3 * time.Second
	c, _, err := dialer.Dial(u.String(), nil)
	task.websocketConn = c
	if err != nil {
		return fmt.Errorf("dial:%s", err)
	}
	log.Printf("Task %s connected to %s", task.ID, u.String())
	go func() {
		defer func() {
			r.Close()
			w.Close()
			c.Close()
			log.Printf("Connection closed with %s", c.RemoteAddr())
		}()
		for {
			_, _, err := c.NextReader()
			if err != nil {
				return
			}
		}
	}()
	go func() {
		for {
			buf := make([]byte, 1024)
			read, err := task.LogReader.Read(buf)
			if err != nil {
				break
			}
			msg := buf[:read]
			if read > 0 {
				c.WriteMessage(websocket.BinaryMessage, []byte(fmt.Sprintf("%s", msg)))
			}
		}
	}()
	return nil
}

// Do a task
func (task *Task) Do() error {
	defer close(task.cancelCh)
	ack := TaskACK{}
	alreadyAcked := false
	if task.Stream {
		err := task.streaming()
		if err != nil {
			log.Printf("Unable to start stream for task %s: %s", task.ID, err)
		}
	}
	defer func() {
		if task.Stream {
			task.websocketConn.Close()
		}
	}()
	for _, step := range task.Steps {
		ackStep := TaskACKStep{}
		ackStep.StartTimestamp = time.Now().Unix()
		step.Task = task
		if step.AckBeforeCompletion && task.NeedACK {
			err := agent.ackTask(task, &ack)
			if err != nil {
				log.Printf("Unable to ack task %s before running step: %s", task.ID, err)
			} else {
				log.Printf("task %s ACKed before running step", task.ID)
			}
			alreadyAcked = true
		}
		err := step.Do()
		if err != nil {
			log.Printf("%s Step %s failed with config %s (%s)", step.Plugin, step.Method, step.PluginConfig, err)
			ackStep.Message = err.Error()
			ackStep.Success = false
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
			log.Printf("Unable to ack task %s: %s", task.ID, err)
		} else {
			log.Printf("task %s ACKed", task.ID)
		}
	}
	deleteRunningTasks(task.ID)
	return nil
}
