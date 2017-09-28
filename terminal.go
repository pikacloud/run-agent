package main

import (
	"context"
	"io"

	"github.com/apex/log"
	docker_types "github.com/docker/docker/api/types"
	"github.com/gorilla/websocket"
	"github.com/moby/moby/pkg/stdcopy"
)

// WSReaderWriter uses
type WSReaderWriter struct {
	*websocket.Conn
}

func (c WSReaderWriter) Write(p []byte) (n int, err error) {

	// output, err := iconv.ConvertString(string(p), "ISO-8859-1", "utf-8")

	if err != nil {
		log.Errorf("WSReaderWriter: %+v", err)
		return 0, err
	}

	err = c.WriteMessage(websocket.TextMessage, []byte(p))

	if err != nil {
		log.Errorf("WSReaderWriter: %+v", err)
	}

	return len(p), err
}

// ContainerExecResize resizes container terminal
func (agent *Agent) ContainerExecResize(id string, height, width uint) error {
	size := docker_types.ResizeOptions{
		Height: height,
		Width:  width,
	}
	return agent.DockerClient.ContainerExecResize(context.Background(), id, size)
}

func execPipe(resp docker_types.HijackedResponse, inStream io.Reader, outStream, errorStream io.Writer) error {
	var err error
	receiveStdout := make(chan error, 1)
	if outStream != nil || errorStream != nil {
		go func() {
			// always do this because we are never tty
			_, err = stdcopy.StdCopy(outStream, errorStream, resp.Reader)
			log.Debug("[hijack] End of stdout")
			receiveStdout <- err
		}()
	}

	stdinDone := make(chan struct{})
	go func() {
		if inStream != nil {
			io.Copy(resp.Conn, inStream)
			log.Debug("[hijack] End of stdin")
		}

		if err := resp.CloseWrite(); err != nil {
			log.Errorf("Could not send EOF: %s", err.Error())
		}
		close(stdinDone)
	}()

	select {
	case err := <-receiveStdout:
		if err != nil {
			log.Errorf("Error receiveStdout: %s", err)
			return err
		}
	case <-stdinDone:
		if outStream != nil || errorStream != nil {
			if err := <-receiveStdout; err != nil {
				log.Errorf("Error receiveStdout: %s", err)
				return err
			}
		}
	}
	return nil
}
