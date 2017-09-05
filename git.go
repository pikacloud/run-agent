package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"

	git "gopkg.in/src-d/go-git.v4"
)

// GitCloneOpts describes options for git clone
type GitCloneOpts struct {
	Path string `json:"path"`
	URL  string `json:"repository_url"`
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
		repository, errClone := git.PlainClone(cloneOpts.Path, false, &git.CloneOptions{
			URL:   cloneOpts.URL,
			Depth: 1,
		})
		if errClone != nil {
			os.RemoveAll(cloneOpts.Path)
			return errClone
		}
		log.Printf("%s cloned in %s", cloneOpts.URL, cloneOpts.Path)
		fmt.Println(repository)
		return nil
	default:
		return fmt.Errorf("Unknown step method %s", step.Method)
	}
}
