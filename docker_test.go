package main

import (
	"encoding/base64"
	"fmt"
	"os/exec"
	"reflect"
	"testing"

	fernet "github.com/fernet/fernet-go"
)

func TestRegistryAuthString(t *testing.T) {
	aid := "9ee55b88a2a74a59adc48a8e425c61d7"
	key := base64.StdEncoding.EncodeToString([]byte(aid))
	k := fernet.MustDecodeKeys(key)
	password, err := fernet.EncryptAndSign([]byte("bar"), k[0])
	if err != nil {
		t.Errorf("Cannot create test encrypted password: %v", err)
	}
	e := ExternalAuthPullOpts{
		Login:    "foo",
		Password: string(password),
	}
	encodedAuth := e.registryAuthString(aid)
	plainAuth, err := base64.StdEncoding.DecodeString(encodedAuth)
	if err != nil {
		t.Errorf("Cannot decode base64 string %s", plainAuth)
	}
	want := "{\"username\": \"foo\", \"password\": \"bar\"}"
	if !reflect.DeepEqual(string(plainAuth), want) {
		t.Errorf("auth %s, want %v", plainAuth, want)
	}
}

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
		t.Errorf("Cannot pull image nginx:latest: %v", err)
	}
	create, err := agent.dockerCreate(opts)
	defer func() {
		command := "docker unpause foobar;docker inspect foobar && docker rm -vf foobar; docker rmi nginx:latest || echo"
		cmd := exec.Command("/bin/sh", "-c", command)
		errRun := cmd.Run()
		if errRun != nil {
			t.Errorf("Cannot remove container foobar: %v", errRun)
		}
	}()
	if err != nil {
		t.Errorf("Cannot create container %v: %v", opts, err)
	}
	command := fmt.Sprintf("docker inspect %s", create.ID)
	cmd := exec.Command("/bin/sh", "-c", command)
	errRun := cmd.Run()
	if errRun != nil {
		t.Errorf("Container is not created: %v", err)
	}
	errStart := agent.dockerStart(create.ID, 0)
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
