package main

import (
	"fmt"
	"os/exec"
	"testing"
)

func TestDockerContainer(t *testing.T) {
	opts := &DockerCreateOpts{
		Name:   "foobar",
		Remove: true,
		PullOpts: &DockerPullOpts{
			Image: "nginx:latest",
		},
	}
	err := agent.dockerPull(opts.PullOpts)
	if err != nil {
		t.Fatalf("Cannot pull image nginx:latest: %v", err)
	}
	create, err := agent.dockerCreate(opts)
	if err != nil {
		t.Fatalf("Cannot create container %v: %v", opts, err)
	}
	defer func() {
		command := "docker unpause foobar;docker inspect foobar && docker rm -vf foobar; docker rmi nginx:latest || echo"
		cmd := exec.Command("/bin/sh", "-c", command)
		errRun := cmd.Run()
		if errRun != nil {
			t.Errorf("Cannot remove container foobar: %v", errRun)
		}
	}()
	command := fmt.Sprintf("docker inspect %s", create.ID)
	cmd := exec.Command("/bin/sh", "-c", command)
	errRun := cmd.Run()
	if errRun != nil {
		t.Errorf("Container is not created: %v", err)
	}
	errStart := agent.dockerStart(create.ID, 0, nil, "")
	if errStart != nil {
		t.Errorf("Cannot start container: %v", errStart)
	}
	command = fmt.Sprintf("docker ps -q -f status=running | grep '^%.12s$'", create.ID)
	cmd = exec.Command("/bin/sh", "-c", command)
	errRun = cmd.Run()
	if errRun != nil {
		t.Errorf("Container is not started: %v", errRun)
	}
	errPause := agent.dockerPause(create.ID)
	if errPause != nil {
		t.Errorf("Cannot pause container: %v", errPause)
	}
	command = fmt.Sprintf("docker ps -q -f status=paused | grep '^%.12s$'", create.ID)
	cmd = exec.Command("/bin/sh", "-c", command)
	errRun = cmd.Run()
	if errRun != nil {
		t.Errorf("Container is not paused: %v", errRun)
	}
	errUnpause := agent.dockerUnpause(create.ID)
	if errUnpause != nil {
		t.Errorf("Cannot unpause container: %v", errUnpause)
	}
	command = fmt.Sprintf("docker ps -q -f status=running | grep '^%.12s$'", create.ID)
	cmd = exec.Command("/bin/sh", "-c", command)
	errRun = cmd.Run()
	if errRun != nil {
		t.Errorf("Container is not unpaused: %v", errRun)
	}
}
