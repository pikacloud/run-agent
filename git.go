package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	fernet "github.com/fernet/fernet-go"

	git "gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/plumbing"

	"golang.org/x/crypto/ssh"
	"gopkg.in/src-d/go-git.v4/plumbing/transport"
	gitSSH "gopkg.in/src-d/go-git.v4/plumbing/transport/ssh"

	gitConfig "gopkg.in/src-d/go-git.v4/config"
	gitClient "gopkg.in/src-d/go-git.v4/plumbing/transport/client"
	"gopkg.in/src-d/go-git.v4/storage/memory"
	"gopkg.in/src-d/go-git.v4/utils/ioutil"
)

// GitCloneOpts describes options for git clone
type GitCloneOpts struct {
	Path    string `json:"path"`
	URL     string `json:"repository_url"`
	GitRef  string `json:"git_ref"`
	SSHKey  string `json:"sshkey"`
	SSHUser string `json:"sshuser"`
}

func (o *GitCloneOpts) searchSSHUser() string {
	s := strings.Split(o.URL, "@")
	if len(s) > 1 {
		return s[0]
	}
	return ""
}

func (o *GitCloneOpts) auth(key []byte) (*gitSSH.PublicKeys, error) {
	if o.SSHKey == "" {
		return nil, nil
	}
	bytesKey := make([]byte, 32)
	copy(bytesKey, key)
	fKey := base64.StdEncoding.EncodeToString(bytesKey)
	k := fernet.MustDecodeKeys(fKey)
	sshkey := fernet.VerifyAndDecrypt([]byte(o.SSHKey), 60*time.Second, k)
	auth, err := gitSSH.NewPublicKeys(o.SSHUser, sshkey, "")
	auth.HostKeyCallback = func(hostname string, remote net.Addr, key ssh.PublicKey) error { return nil }
	if err != nil {
		return nil, fmt.Errorf("error on parsing private key: %+v", err)
	}
	return auth, nil
}

// Git handles git functions
func (step *TaskStep) Git() error {
	switch step.Method {
	case "clone":
		var cloneOpts = GitCloneOpts{}
		err := json.Unmarshal([]byte(step.PluginConfig), &cloneOpts)
		if err != nil {
			return fmt.Errorf("Bad config for git clone: %s (%v)", err, step.PluginConfig)
		}
		// check that gitRef exists
		if cloneOpts.GitRef == "" {
			cloneOpts.GitRef = "master"
		}
		if cloneOpts.SSHUser == "" && cloneOpts.SSHKey != "" {
			// try to extract the user name from the repository URL
			user := cloneOpts.searchSSHUser()
			if user == "" {
				user = "git"
			}
			cloneOpts.SSHUser = user
		}
		auth, err := cloneOpts.auth([]byte(agent.ID))
		if err != nil {
			return err
		}
		references, errAuth := inMemorylsRemote(cloneOpts.URL, auth)
		if errAuth != nil {
			return fmt.Errorf("Unable to list remote reference for %s: %+v", cloneOpts.URL, errAuth)
		}
		reference, err := resolveRawReference(cloneOpts.GitRef, references)
		if err != nil {
			return fmt.Errorf("Reference %s not found in %s: %+v", cloneOpts.GitRef, cloneOpts.URL, err)
		}
		errMkdir := os.MkdirAll(cloneOpts.Path, 0700)
		if errMkdir != nil {
			return errMkdir
		}
		step.stream([]byte(fmt.Sprintf("\033[33m[GIT]\033[0m cloning %s, using %s %s\r\n", cloneOpts.URL, reference.Type(), reference.String())))
		cloneOptions := &git.CloneOptions{
			URL:           cloneOpts.URL,
			Depth:         1,
			ReferenceName: reference.Name(),
		}
		if auth != nil {
			cloneOptions.Auth = auth
		}
		if step.Task.Stream {
			cloneOptions.Progress = step.Task.streamer.ioWriter
		}
		repository, errClone := git.PlainClone(cloneOpts.Path, false, cloneOptions)
		if errClone != nil {
			os.RemoveAll(cloneOpts.Path)
			return fmt.Errorf("Unable to clone %s", errClone)
		}

		logger.Debugf("%s cloned in %s", cloneOpts.URL, cloneOpts.Path)
		head, _ := repository.Head()
		commit, _ := repository.CommitObject(head.Hash())
		msg := fmt.Sprintf("\n\n%s\n\n", strings.Replace(commit.String(), "\n", "\n\r", -1))
		step.stream([]byte(msg))
		return nil
	default:
		return fmt.Errorf("Unknown step method %s", step.Method)
	}
}

func resolveRawReference(raw string, references memory.ReferenceStorage) (*plumbing.Reference, error) {
	for _, reference := range references {
		if strings.HasSuffix(string(reference.Name()), fmt.Sprintf("/%s", raw)) {
			return reference, nil
		}
	}
	return nil, fmt.Errorf("Git reference %s not found", raw)
}

func inMemorylsRemote(repoURL string, auth transport.AuthMethod) (memory.ReferenceStorage, error) {
	remoteName := "origin"
	s := memory.NewStorage()
	repo, err := git.Init(s, nil)
	if err != nil {
		return nil, err
	}
	remoteConfig := &gitConfig.RemoteConfig{
		Name: remoteName,
		URLs: []string{repoURL},
	}
	remote, err := repo.CreateRemote(remoteConfig)
	if err != nil {
		return nil, err
	}
	remotes, err := lsRemote(remote, auth)
	if err != nil {
		return nil, err
	}
	return remotes, nil
}

func lsRemote(remote *git.Remote, auth transport.AuthMethod) (memory.ReferenceStorage, error) {
	url := remote.Config().URLs[0]
	s, err := newUploadPackSession(url, auth)
	if err != nil {
		return nil, err
	}
	defer ioutil.CheckClose(s, &err)

	ar, err := s.AdvertisedReferences()
	if err != nil {
		return nil, err
	}

	return ar.AllReferences()
}

func newUploadPackSession(url string, auth transport.AuthMethod) (transport.UploadPackSession, error) {
	c, ep, err := newGitClient(url)
	if err != nil {
		return nil, err
	}
	return c.NewUploadPackSession(ep, auth)
}

func newGitClient(url string) (transport.Transport, *transport.Endpoint, error) {
	ep, err := transport.NewEndpoint(url)
	if err != nil {
		return nil, nil, err
	}
	c, err := gitClient.NewClient(ep)
	if err != nil {
		return nil, nil, err
	}

	return c, ep, err
}
