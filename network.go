package main

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"os/exec"
	"reflect"
	"strings"
	"time"

	docker_types "github.com/docker/docker/api/types"
	fernet "github.com/fernet/fernet-go"
)

func (agent *Agent) infiniteSyncAgentInterfaces() {
	for {
		newInt, err := agent.getNetInterfaces()
		if err != nil {
			time.Sleep(3 * time.Second)
			continue
		}

		if !reflect.DeepEqual(interfaces, newInt) {
			interfaces = newInt
			err := agent.syncAgentInterfaces()
			if err != nil {
				logger.Infof("Cannot sync agent interfaces: %+v", err)
			} else {
				logger.Debug("Sync agent interfaces OK")
			}
		}
		time.Sleep(3 * time.Second)
	}
}

func (agent *Agent) syncAgentInterfaces() error {
	opt := CreateAgentOptions{
		Interfaces: interfaces,
	}
	uri := fmt.Sprintf("run/agents/%s/", agent.ID)
	_, err := pikacloudClient.Put(uri, opt, &agent)
	if err != nil {
		return err
	}
	return nil
}

// getNetInterfaces describes the function who pushes all active interfaces from host to connect to network
func (agent *Agent) getNetInterfaces() ([]string, error) {
	var SysInt []string
	var DockInt []string
	var Ret []string
	Cards, err := net.InterfaceAddrs()
	if err != nil {
		return Ret, err
	}
	for _, card := range Cards {
		if ipnet, ok := card.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				SysInt = append(SysInt, card.String())
			}
		}
	}

	ctx := context.Background()
	nl, err := agent.DockerClient.NetworkList(ctx, docker_types.NetworkListOptions{})
	if err != nil {
		return Ret, err
	}
	for _, net := range nl {
		if len(net.IPAM.Config) > 0 {
			DockInt = append(DockInt, net.IPAM.Config[0].Subnet)
		}
	}
	for _, scard := range SysInt {
		_, ipv4Net, err2 := net.ParseCIDR(scard)
		if err2 != nil {
			return Ret, err2
		}
		test := true
		for _, dcard := range DockInt {
			if ipv4Net.String() == dcard {
				test = false
			}
		}
		if test == true {
			Ret = append(Ret, scard)
		}
	}
	return Ret, nil
}

// detachNetwork describes available methods of the Network plugin
func (agent *Agent) detachNetwork(containerID string, Networks map[string]string) error {
	ctx := context.Background()

	for network, domain := range Networks {
		// nets
		command := fmt.Sprintf("%s detach net:%s %s",
			"/usr/local/bin/weave", string(network), containerID)
		cmd2, err2 := parseCommandLine(command)
		if err2 != nil {
			return fmt.Errorf("Error parsing command line (detach): %s", err2)
		}
		cmd := exec.CommandContext(ctx, cmd2[0], cmd2[1:]...)
		IP, _ := cmd.Output()
		//domains
		if domain != "" {
			command = fmt.Sprintf("%s dns-remove %s %s",
				"/usr/local/bin/weave", string(IP), containerID)
			cmd2, err2 = parseCommandLine(command)
			if err2 != nil {
				return fmt.Errorf("Error parsing command line (dns detach): %s", err2)
			}
			cmd = exec.CommandContext(ctx, cmd2[0], cmd2[1:]...)
			cmd.Run()
		}
	}

	return nil
}

func (agent *Agent) checkSuperNetwork(MasterIP []string) error {
	ctx := context.Background()
	command, err := parseCommandLine("docker ps")
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
		sn, err3 := pikacloudClient.SuperNetwork(agent.ID)
		if err3 != nil {
			return err3
		}
		key := base64.StdEncoding.EncodeToString([]byte(agent.ID))
		k := fernet.MustDecodeKeys(key)
		password := fernet.VerifyAndDecrypt([]byte(sn.Key), 60*time.Second, k)
		command2 := fmt.Sprintf("%s launch --password=%s --ipalloc-range %s --dns-domain=%s %s",
			"/usr/local/bin/weave", string(password), "10.42.0.0/16", "pikacloud.local", "--plugin=false --proxy=false")
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
func getNewNets(nets map[string]string, containerID string) (map[string]string, error) {
	var ret map[string]string
	var delete map[string]string
	var tnets []string
	var tnets2 []string

	ret = make(map[string]string)
	delete = make(map[string]string)
	for net, _ := range nets {
		tnets = append(tnets, net)
	}

	for _, network := range networks[containerID] {
		tnets2 = append(tnets2, strings.Split(network, "-")[0])
	}
	diff := difference(tnets, tnets2)
	fmt.Println(diff)

	for _, net := range diff {
		if stringInSlice(net, tnets2) {
			delete[net] = nets[net]
		} else {
			ret[net] = nets[net]
		}
	}
	if err := agent.detachNetwork(containerID, delete); err != nil {
		return nil, fmt.Errorf("Error detaching Container from Network: %s", err)
	}
	return ret, nil
}

// attachNetwork describes available methods of the Network plugin
func (agent *Agent) attachNetwork(containerID string, Networks map[string]string, MasterIP []string, Name string, NetPasswd string) error {
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
		fmt.Println(newNets)
		for network, domain := range newNets {
			//nets
			command := fmt.Sprintf("%s attach net:%s %s",
				"/usr/local/bin/weave", string(network), containerID)
			cmd2, err2 := parseCommandLine(command)
			if err2 != nil {
				return fmt.Errorf("Error parsing command line (attach): %s", err2)
			}
			cmd := exec.CommandContext(ctx, cmd2[0], cmd2[1:]...)
			IP, err := cmd.Output()
			if err != nil {
				return fmt.Errorf("Error attaching container to network: %s", err)
			}
			// domains
			if domain != "" {
				command = fmt.Sprintf("%s dns-add %s %s -h %s.%s",
					"/usr/local/bin/weave", string(IP), containerID, Name, domain)
				cmd2, err2 = parseCommandLine(command)
				if err2 != nil {
					return fmt.Errorf("Error parsing command line (dns add): %s", err2)
				}
				cmd = exec.CommandContext(ctx, cmd2[0], cmd2[1:]...)
				err = cmd.Run()
				if err != nil {
					return fmt.Errorf("Error creating dns entry: %s", err)
				}
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
