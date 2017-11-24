package main

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// detachNetwork describes available methods of the Network plugin
func (agent *Agent) detachNetwork(containerID string, Networks []string) error {
	ctx := context.Background()

	for _, network := range Networks {
		command := fmt.Sprintf("%s detach net:%s %s",
			"/usr/local/bin/weave", string(network), containerID)
		cmd2, err2 := parseCommandLine(command)
		if err2 != nil {
			return fmt.Errorf("Error parsing command line (detach): %s", err2)
		}
		cmd := exec.CommandContext(ctx, cmd2[0], cmd2[1:]...)
		cmd.Run()
	}

	return nil
}

func (agent *Agent) checkSuperNetwork(MasterIP string) error {
	ctx := context.Background()
	command, err := parseCommandLine("/bin/ps aux")
	if err != nil {
		return fmt.Errorf("Error parsing command line (checking): %s", err)
	}
	output, err2 := exec.CommandContext(ctx, command[0], command[1:]...).Output()
	if err2 != nil {
		return fmt.Errorf("Error checking Network: %s", err)
	}
	test := string(output)
	process := strings.Contains(test, "weave")

	if process != true {
		command2 := fmt.Sprintf("%s launch --password=%s --ipalloc-range %s --dns-domain=%s %s",
			"/usr/local/bin/weave", "e29f169168f64368a32920e3ce041826", "10.42.0.0/16", "pikacloud.local", "--plugin=false --proxy=false --dns-ttl=10")
		command, err = parseCommandLine(command2)
		if err != nil {
			return fmt.Errorf("Error parsing command line (create): %s", err)
		}
		err = exec.CommandContext(ctx, command[0], command[1:]...).Run()
		if err != nil {
			return fmt.Errorf("Error creating Network: %s", err)
		}
	}

	if len(MasterIP) > 0 {
		command2 := "/usr/local/bin/weave status peers"
		command, err = parseCommandLine(command2)
		if err != nil {
			return fmt.Errorf("Error parsing command line (check peers): %s", err)
		}
		output, err = exec.CommandContext(ctx, command[0], command[1:]...).Output()
		if err != nil {
			return fmt.Errorf("Error checking connect state: %s", err)
		}
		if strings.Count(string(output), "\n") <= 2 {
			command2 = fmt.Sprintf("%s connect %s",
				"/usr/local/bin/weave", MasterIP)
			command, err = parseCommandLine(command2)
			if err != nil {
				return fmt.Errorf("Error parsing command line (connect): %s", err)
			}
			err = exec.CommandContext(ctx, command[0], command[1:]...).Run()
			if err != nil {
				return fmt.Errorf("Error connecting Peer: %s", err)
			}
		}
	}
	return nil
}

func difference(slice1 []string, slice2 []string) []string {
	var diff []string
	for i := 0; i < 2; i++ {
		for _, s1 := range slice1 {
			found := false
			for _, s2 := range slice2 {
				if s1 == s2 {
					found = true
					break
				}
			}
			if !found {
				diff = append(diff, s1)
			}
		}
		if i == 0 {
			slice1, slice2 = slice2, slice1
		}
	}
	return diff
}

func stringInSlice(a string, list []string) bool {
	for _, b := range list {
		if b == a {
			return true
		}
	}
	return false
}

// getNewNets disconnect from old networks and prepare the new list of Nets
func getNewNets(nets []string, containerID string) ([]string, error) {
	var ret []string
	var delete []string
	diff := difference(nets, networks[containerID])

	for i, net := range diff {
		if stringInSlice(net, networks[containerID]) {
			delete = append(delete, net)
			ret = append(diff[:i], diff[i+1:]...)
		}
	}
	if err := agent.detachNetwork(containerID, delete); err != nil {
		return nil, fmt.Errorf("Error detaching Container from Network: %s", err)
	}
	return ret, nil
}

// attachNetwork describes available methods of the Network plugin
func (agent *Agent) attachNetwork(containerID string, Networks []string, MasterIP string) error {
	ctx := context.Background()

	test := agent.checkSuperNetwork(MasterIP)
	if test == nil {
		newNets := Networks
		if _, ok := networks[containerID]; ok {
			var erro error
			newNets, erro = getNewNets(Networks, containerID)
			if erro != nil {
				return erro
			}
		}
		if len(newNets) == 0 {
			newNets = Networks
		}
		for _, network := range newNets {
			command := fmt.Sprintf("%s attach net:%s %s",
				"/usr/local/bin/weave", string(network), containerID)
			cmd2, err2 := parseCommandLine(command)
			if err2 != nil {
				return fmt.Errorf("Error creating Network: %s", err2)
			}
			cmd := exec.CommandContext(ctx, cmd2[0], cmd2[1:]...)
			err := cmd.Run()
			if err != nil {
				return fmt.Errorf("Error attaching container to network: %s", err)
			}
		}
	}

	return nil
}

func parseCommandLine(command string) ([]string, error) {
	var args []string
	state := "start"
	current := ""
	quote := "\""
	escapeNext := true
	for i := 0; i < len(command); i++ {
		c := command[i]
		if state == "quotes" {
			if string(c) != quote {
				current += string(c)
			} else {
				args = append(args, current)
				current = ""
				state = "start"
			}
			continue
		}
		if escapeNext {
			current += string(c)
			escapeNext = false
			continue
		}
		if c == '\\' {
			escapeNext = true
			continue
		}
		if c == '"' || c == '\'' {
			state = "quotes"
			quote = string(c)
			continue
		}
		if state == "arg" {
			if c == ' ' || c == '\t' {
				args = append(args, current)
				current = ""
				state = "start"
			} else {
				current += string(c)
			}
			continue
		}
		if c != ' ' && c != '\t' {
			state = "arg"
			current += string(c)
		}
	}
	if state == "quotes" {
		return []string{}, errors.New(fmt.Sprintf("Unclosed quote in command line: %s", command))
	}
	if current != "" {
		args = append(args, current)
	}
	return args, nil
}
