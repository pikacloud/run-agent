// +build darwin

package weave

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// PS returns weave ps
func (w *Weave) PS(containerID string) ([]*PSOutput, error) {
	out, err := w.cli(nil, "ps", containerID)
	if err != nil {
		return nil, fmt.Errorf("weave ps returns an error: %s, %s", err, string(out))
	}
	output := []*PSOutput{}
	toScan := bytes.NewReader(out)
	scanner := bufio.NewScanner(toScan)
	for scanner.Scan() {
		line := scanner.Text()
		items := strings.Split(line, " ")
		if len(items) < 2 {
			continue
		}
		p := &PSOutput{
			ContainerID: items[0],
			MAC:         items[1],
			IP:          items[2:],
		}
		output = append(output, p)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("Cannot read weave ps output: %s", err)
	}
	return output, nil
}

// Status returns the weave status
func (w *Weave) Status() (*Status, error) {
	out, err := w.cli(nil, "report")
	if err != nil {
		return nil, fmt.Errorf("weave report returns an error: %s, %s", err, string(out))
	}
	status := &Status{}
	if err := json.Unmarshal(out, &status); err != nil {
		return nil, err
	}
	return status, nil
}
