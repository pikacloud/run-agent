package main

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// Network describe the network view
type Network struct {
	containersID []string `json:"containersid,omitempty"`
}

// NetworkCreateOpts describes the network create struct
type NetworkCreateOpts struct {
	Domain    string `json:"domain"`
	Command   string `json:"command"`
	CIDR      string `json:"cidr"`
	ExtraOpts string `json:"extraopts"`
	Password  string `json:"password"`
}

// NetworkConnectOpts describes the network connect struct
type NetworkConnectOpts struct {
	Command string `json:"command"`
	IP      string `json:"ip"`
}

// NetworkAttachOpts describes the network attach struct
type NetworkAttachOpts struct {
	Domain    string `json:"domain"`
	CIDR      string `json:"cidr"`
	Command   string `json:"command"`
	Container string `json:"container"`
}

// NetworkDetachOpts describes the network detach struct
type NetworkDetachOpts struct {
	Domain    string `json:"domain"`
	CIDR      string `json:"cidr"`
	Command   string `json:"command"`
	Container string `json:"container"`
}

func (agent *Agent) checkSuperNetwork(MasterIP string) error {
	ctx := context.Background()
	command, err := parseCommandLine("/bin/ps aux")
	if err != nil {
		return fmt.Errorf("Error creating Network: %s", err)
	}
	output, err2 := exec.CommandContext(ctx, command[0], command[1:]...).Output()
	if err2 != nil {
		return fmt.Errorf("Error creating Network: %s", err)
	}
	test := string(output)
	process := strings.Contains(test, "weave")
	if process != true {
		command2 := fmt.Sprintf("%s launch --password=%s --ipalloc-range %s --dns-domain=%s %s",
			"/usr/local/bin/weave", "toto", "10.42.0.0/16", "pikacloud.local", "--plugin=false --proxy=false --dns-ttl=10")
		//createOpts.Command, createOpts.Password, createOpts.CIDR, createOpts.Domain, createOpts.ExtraOpts)
		command, err = parseCommandLine(command2)
		if err != nil {
			return fmt.Errorf("Error creating Network: %s", err)
		}
		err = exec.CommandContext(ctx, command[0], command[1:]...).Run()
		if err != nil {
			return fmt.Errorf("Error creating Network: %s", err)
		}
		if len(MasterIP) > 0 {
			command3 := fmt.Sprintf("%s connect %s",
				"/usr/local/bin/weave", MasterIP)
			//createOpts.Command, createOpts.Password, createOpts.CIDR, createOpts.Domain, createOpts.ExtraOpts)
			command, err = parseCommandLine(command3)
			if err != nil {
				return fmt.Errorf("Error creating Network: %s", err)
			}
			err = exec.CommandContext(ctx, command[0], command[1:]...).Run()
			if err != nil {
				return fmt.Errorf("Error creating Network: %s", err)
			}
		}
	}
	return nil
}

//func removeDuplicates(elements []string) []string {
//	encountered := map[string]bool{}

// Create a map of all unique elements.
//	for v := range elements {
//		encountered[elements[v]] = true
//	}

// Place all keys from the map into a slice.
//	result := []string{}
//	for key, _ := range encountered {
//		result = append(result, key)
//	}
//	return result
//}

//func addNewCIDR(net *Network, label string) {
//	var check = true
//	for key, _ := range networks {
//		if key == label {
//			check = false
///		list := append(networks[key].containersID, net.containersID...)
//		networks[key].containersID = removeDuplicates(list)
//		}
//	}
//	if check == true {
//		networks[label] = net
//	}
//}

// attachNetwork describes available methods of the Network plugin
func (agent *Agent) attachNetwork(containerID string, Networks []string, MasterIP string) error {
	ctx := context.Background()

	if len(Networks) == 0 {
		return nil
	}
	fmt.Println("step 2")
	test := agent.checkSuperNetwork(MasterIP)
	fmt.Println("step 3")
	if test == nil {
		fmt.Println("Step 4")
		for _, network := range Networks {
			command := fmt.Sprintf("%s attach net:%s %s",
				"/usr/local/bin/weave", string(network), containerID)
			cmd2, err2 := parseCommandLine(command)
			if err2 != nil {
				return fmt.Errorf("Error creating Network: %s", err2)
			}
			fmt.Println("Step 5")
			cmd := exec.CommandContext(ctx, cmd2[0], cmd2[1:]...)
			err := cmd.Run()
			if err != nil {
				return fmt.Errorf("Error attaching container to network: %s", err)
			}
			fmt.Println("Step 3")
			//net := new(Network)
			//net.containersID = append(net.containersID, containerID)
			//		addNewCIDR(net, network)
			fmt.Println("Step 4")
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

/*
	switch step.Method {
	case "create":
		var createOpts = NetworkCreateOpts{}
		err := json.Unmarshal([]byte(step.PluginConfig), &createOpts)
		if err != nil {
			return fmt.Errorf("Bad config for create_task: %s (%v)", err, step.PluginConfig)
		}
		command := fmt.Sprintf("%s launch --password=%s --ipalloc-range %s --dns-domain=%s %s",
			createOpts.Command, createOpts.Password, createOpts.CIDR, createOpts.Domain, createOpts.ExtraOpts)
		cmd := exec.Command(command)
		err = cmd.Run()
		if err != nil {
			return fmt.Errorf("Error creating Network: %s", err)
		}
		return nil
	case "connect":
		var connectOpts = NetworkConnectOpts{}
		err := json.Unmarshal([]byte(step.PluginConfig), &connectOpts)
		if err != nil {
			return fmt.Errorf("Bad config for connect_task: %s (%v)", err, step.PluginConfig)
		}
		command := fmt.Sprintf("%s connect %s", connectOpts.Command, connectOpts.IP)
		cmd := exec.Command(command)
		err = cmd.Run()
		if err != nil {
			fmt.Printf("Error connecting Network: %s", err)
		}
		return nil
	case "disconnect":
		command := "/usr/local/bin/weave stop"
		cmd := exec.Command(command)
		err := cmd.Run()
		if err != nil {
			return fmt.Errorf("Error stopping Peer: %s", err)
		}
		command = "/usr/local/bin/weave reset --force"
		cmd = exec.Command(command)
		err = cmd.Run()
		if err != nil {
			return fmt.Errorf("Error deleting Peer: %s", err)
		}
		return nil
	case "attach":
		var attachOpts = NetworkAttachOpts{}
		network := new(Network)
		err := json.Unmarshal([]byte(step.PluginConfig), &attachOpts)
		if err != nil {
			return fmt.Errorf("Bad config for attach_task: %s (%v)", err, step.PluginConfig)
		}
		command := fmt.Sprintf("%s attach net:%s %s", attachOpts.Command, attachOpts.CIDR, attachOpts.Container)
		cmd := execmsg.Action.Command(command)
		err = cmd.Run()
		if err != nil {
			return fmt.Errorf("Error attaching container to network: %s", err)
		}
		network.CIDR = attachOpts.CIDR
		network.Domain = attachOpts.Domain
		//if !networks[network.CIDR] {
		//	networks[network.CIDR] = &network
		//}
		//command = fmt.Sprintf("%s dns-add %s -h %s.%s", attachOpts.Command, attachOpts.Container, attachOpts.Container, attachOpts.Domain)
		//cmd = exec.Command(command)
		//err = cmd.Run()
		//if err != nil {
		//	fmt.Printf("Error adding container to DNS: %s", err)
		//}
		return nil
	case "detach":
		var detachOpts = NetworkDetachOpts{}
		err := json.Unmarshal([]byte(step.PluginConfig), &detachOpts)
		if err != nil {
			return fmt.Errorf("Bad config for detach_task: %s (%v)", err, step.PluginConfig)
		}
		command := fmt.Sprintf("%s detach %s", detachOpts.Command, detachOpts.Container)
		cmd := exec.Command(command)
		err = cmd.Run()
		if err != nil {
			return fmt.Errorf("Error detaching container from network: %s", err)
		}
		//command = fmt.Sprintf("%s dns-remove %s -h %s.%s", detachOpts.Command, detachOpts.Container, detachOpts.Container, detachOpts.Domain)
		//cmd = exec.Command(command)
		//err = cmd.Run()
		//if err != nil {
		//		return fmt.Errorf("Error deleting container from DNS: %s", err)
		//}
		return nil
	default:
		return fmt.Errorf("Unknown step method %s", step.Method)
	}
}
*/
