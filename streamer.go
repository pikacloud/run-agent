package main

import (
	"bytes"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

const (
	maxMessageSize = 1024
)

// Streamer describes a streamer
type Streamer struct {
	ioReader  *io.PipeReader
	ioWriter  *io.PipeWriter
	msg       chan []byte
	conn      *websocket.Conn
	done      chan bool
	retryConn chan bool
}

func (s *Streamer) run(id string, autoReconnect bool) {
	s.done = make(chan bool, 1)
	s.retryConn = make(chan bool, 1)
	s.msg = make(chan []byte, 1024)
	r, w := io.Pipe()
	s.ioReader = r
	s.ioWriter = w
	defer func() {
		if s.conn != nil {
			s.conn.Close()
		}
		s.ioReader.Close()
		s.ioWriter.Close()
		r.Close()
		w.Close()
		close(s.msg)
		close(s.done)
		close(s.retryConn)
		logger.Debug("Streamer exited")
	}()
	go s.ioPipePump()
	wsURLParams := strings.Split(wsURL, "://")
	scheme := wsURLParams[0]
	addr := wsURLParams[1]
	path := fmt.Sprintf("/_ws/hub/log-writer/%s/", id)
	u := url.URL{Scheme: scheme, Host: addr, Path: path}
	s.retryConn <- true
	for {
		select {
		case <-s.done:
			logger.Debug("Done received")
			return
		case <-s.retryConn:
			if s.conn != nil {
				s.conn.Close()
			}
			err := s.connectWS(u)
			if err != nil {
				logger.Errorf("Unable to connect to %s: %+v", u.String(), err)
				if !autoReconnect {
					return
				}
				time.Sleep(3 * time.Second)
				s.retryConn <- true
				continue
			}
			logger.Infof("Streamer connected to %s", u.String())
			go s.readPump()
			s.writePump()
		}
	}

}

func (s *Streamer) destroy() {
	s.done <- true
}

func (s *Streamer) wait() {
	<-s.done
}

func (s *Streamer) connectWS(u url.URL) error {
	dialer := websocket.DefaultDialer
	dialer.HandshakeTimeout = 3 * time.Second
	var err error
	s.conn, _, err = dialer.Dial(u.String(), nil)
	if err != nil {
		return err
	}
	return nil
}

// ioPipePump pipe io from ioReader to msg channel
func (s *Streamer) ioPipePump() {
	for {
		buf := make([]byte, 1024)
		read, err := s.ioReader.Read(buf)
		if err != nil {
			return
		}
		msg := buf[:read]
		if read > 0 {
			select {
			case s.msg <- msg:
			default:
			}
		}
	}
}

// writePump reads message from msg channel and push to the websocket connection
func (s *Streamer) writePump() {
	for {
		select {
		case <-s.done:
			s.done <- true
			return
		case message, ok := <-s.msg:
			if !ok {
				s.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			messageBuffer := bytes.NewBuffer(message)
			for {
				buf := make([]byte, maxMessageSize)
				read, err := messageBuffer.Read(buf)
				if err != nil {
					break
				}
				s.conn.WriteMessage(websocket.BinaryMessage, buf[:read])
			}
		}
	}
}

// readPump keep the websocket connection alive
func (s *Streamer) readPump() {
	s.conn.SetReadLimit(maxMessageSize)
	for {
		_, _, err := s.conn.NextReader()
		if err != nil {
			logger.Errorf("Streamer readPump error: %+v", err)
			return
		}
	}
}
