package main

import (
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/gorilla/websocket"
)

const (
	maxMessageSize = 1024
)

// Streamer describes a streamer
type Streamer struct {
	ioReader      *io.PipeReader
	ioWriter      *io.PipeWriter
	msg           chan *StreamEntry
	conn          *websocket.Conn
	endWritePump  chan bool
	done          chan bool
	id            string
	autoReconnect bool
	authRequest   chan *StreamAuthRequest
}

//StreamEntry descibe a stream entry
type StreamEntry struct {
	Aid               string `json:"aid"`
	Cid               string `json:"cid"`
	ContainerConfigID string `json:"container_config_id"`
	Msg               []byte `json:"msg"`
	Timestamp         int64  `json:"time"`
}

// StreamAuthRequest defines a stream auth request
type StreamAuthRequest struct {
	Token string `json:"token"`
}

// NewStreamer initializes a new streamer
func NewStreamer(id string, autoReconnect bool) *Streamer {
	r, w := io.Pipe()
	return &Streamer{
		id:            id,
		autoReconnect: autoReconnect,
		endWritePump:  make(chan bool, 1),
		done:          make(chan bool, 1),
		msg:           make(chan *StreamEntry, 1024),
		authRequest:   make(chan *StreamAuthRequest, 1),
		ioReader:      r,
		ioWriter:      w,
	}
}

func (s *Streamer) run() {
	defer func() {
		if s.conn != nil {
			s.conn.Close()
		}
		s.ioReader.Close()
		s.ioWriter.Close()
		close(s.msg)
		close(s.endWritePump)
		close(s.done)
		logger.Debug("Streamer exited")
	}()
	go s.ioPipePump()
	wsURLParams := strings.Split(wsURL, "://")
	scheme := wsURLParams[0]
	addr := wsURLParams[1]
	path := fmt.Sprintf("/_ws/hub/log-writer/%s/", s.id)
	u := url.URL{Scheme: scheme, Host: addr, Path: path}
	for {
		// select {
		// case <-s.done:
		// 	logger.Debug("Done received")
		// 	return
		// case <-s.retryConn:
		// 	if s.conn != nil {
		// 		s.conn.Close()
		// 		time.Sleep(3 * time.Second)
		// 	}
		err := s.connectWS(u)
		if err != nil {
			logger.Errorf("Unable to connect to %s: %+v", u.String(), err)
			select {
			case <-s.done:
				return
			default:
			}
			if !s.autoReconnect {
				return
			}
			time.Sleep(3 * time.Second)
			continue
		}
		logger.Infof("Streamer connected to %s", u.String())
		s.authRequest <- &StreamAuthRequest{
			Token: apiToken,
		}
		go s.writePump()
		s.readPump()
		s.endWritePump <- true
		select {
		case <-s.done:
			return
		default:
		}

	}
}

func (s *Streamer) destroy() {
	s.done <- true
	if s.conn != nil {
		s.conn.Close()
	}
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
	logger.Debugf("ioPipePump %s started", s.id)
	defer logger.Debugf("ioPipePump %s exited", s.id)
	for {
		buf := make([]byte, 1024)
		read, err := s.ioReader.Read(buf)
		if err != nil {
			return
		}
		msg := buf[:read]
		if read > 0 {
			entry := &StreamEntry{
				Msg: msg,
			}
			s.writeEntry(entry)
		}
	}
}

// writePump reads message from msg channel and push to the websocket connection
func (s *Streamer) writePump() {
	logger.Debugf("writePump %s started", s.id)
	defer func() {
		logger.Debugf("writePump %s exited", s.id)
	}()
	for {
		select {
		case <-s.endWritePump:
			return
		case authRequest, ok := <-s.authRequest:
			if !ok {
				s.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			s.conn.WriteJSON(authRequest)
			logger.Debugf("Auth send for streamer %s", s.id)
		case entry, ok := <-s.msg:
			if !ok {
				s.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if len(entry.Msg) > 700 {
				entry.Msg = entry.Msg[:700]
			}
			logger.WithFields(logrus.Fields{
				"aid":       entry.Aid,
				"cid":       entry.Cid,
				"msg":       string(entry.Msg),
				"timestamp": entry.Timestamp,
			}).Debug("Sending logs")
			s.conn.WriteJSON(entry)
		}
	}
}

// readPump keep the websocket connection alive
func (s *Streamer) readPump() {
	logger.Debugf("readPump %s started", s.id)
	defer func() {
		s.conn.Close()
		// if s.autoReconnect {
		// 	select {
		// 	case s.retryConn <- true:
		// 	default:
		// 	}
		// }
		logger.Debugf("readPump %s exited", s.id)
	}()
	for {
		_, _, err := s.conn.NextReader()
		if err != nil {
			logger.Errorf("Streamer readPump error: %+v", err)
			return
		}
	}
}

func (s *Streamer) writeEntry(e *StreamEntry) {
	if e.Timestamp == 0 {
		e.Timestamp = time.Now().UnixNano()
	}
	select {
	case s.msg <- e:
	default:
		logger.WithFields(logrus.Fields{
			"buffer":    fmt.Sprintf("%d/%d", len(s.msg), cap(s.msg)),
			"msg":       string(e.Msg),
			"cid":       e.Cid,
			"aid":       e.Aid,
			"timestamp": e.Timestamp,
		}).Debug("Streamer buffer is full, cannot send log message")
	}
}

func (s *Streamer) writeRawMsg(msg []byte) {
	entry := &StreamEntry{
		Msg: msg,
	}
	s.writeEntry(entry)
}
