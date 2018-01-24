// +build !darwin
package weave

import (
	"fmt"
)

// PS returns weave ps
func (w *Weave) PS(containerID string) ([]*PSOutput, error) {
	type mapping struct {
		ContainerID string   `json:"containerid"`
		Addrs       []string `json:"addrs"`
	}

	type mappings struct {
		Owned []mapping `json:"owned"`
	}
	out := &mappings{}
	err := w.Get("/ip", out)
	if err != nil {
		return nil, fmt.Errorf("weave api /ip returns an error: %s", err)
	}
	output := []*PSOutput{}
	for _, mapping := range out.Owned {
		if mapping.ContainerID != containerID {
			continue
		}
		p := &PSOutput{
			ContainerID: mapping.ContainerID,
			IP:          mapping.Addrs,
		}
		output = append(output, p)
	}
	return output, nil
}

// Status returns weave status
func (w *Weave) Status() (*Status, error) {
	status := &Status{}
	if err := w.Get("/report", status); err != nil {
		return nil, err
	}
	return status, nil
}
