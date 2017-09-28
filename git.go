package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	git "gopkg.in/src-d/go-git.v4"
)

// GitCloneOpts describes options for git clone
type GitCloneOpts struct {
	Path   string `json:"path"`
	URL    string `json:"repository_url"`
	GitRef string `json:"git_ref"`
}

// Git handles git functions
func (step *TaskStep) Git() error {
	switch step.Method {
	case "clone":
		var cloneOpts = GitCloneOpts{}
		err := json.Unmarshal([]byte(step.PluginConfig), &cloneOpts)
		if err != nil {
			return fmt.Errorf("Bad config for git clone: %s (%v)", err, step.PluginConfig)
		}
		errMkdir := os.MkdirAll(cloneOpts.Path, 0700)
		if errMkdir != nil {
			return errMkdir
		}
		step.stream([]byte(fmt.Sprintf("git clone %s\n", cloneOpts.URL)))
		cloneOptions := &git.CloneOptions{
			URL:   cloneOpts.URL,
			Depth: 1,
		}
		if step.Task.Stream {
			cloneOptions.Progress = step.Task.streamer.ioWriter
		}
		repository, errClone := git.PlainClone(cloneOpts.Path, false, cloneOptions)
		if errClone != nil {
			os.RemoveAll(cloneOpts.Path)
			return errClone
		}
		//log.Printf("%s cloned in %s", cloneOpts.URL, cloneOpts.Path)
		head, _ := repository.Head()
		commit, _ := repository.CommitObject(head.Hash())
		msg := fmt.Sprintf("\n\n%s\n\n", strings.Replace(commit.String(), "\n", "\n\r", -1))
		step.stream([]byte(msg))
		return nil
	default:
		return fmt.Errorf("Unknown step method %s", step.Method)
	}
}
