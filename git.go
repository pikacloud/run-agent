package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	fernet "github.com/fernet/fernet-go"

	git "gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/plumbing"

	"golang.org/x/crypto/ssh"
	"gopkg.in/src-d/go-git.v4/plumbing/transport"
	ssh2 "gopkg.in/src-d/go-git.v4/plumbing/transport/ssh"

	gitConfig "gopkg.in/src-d/go-git.v4/config"
	gitClient "gopkg.in/src-d/go-git.v4/plumbing/transport/client"
	"gopkg.in/src-d/go-git.v4/storage/memory"
	"gopkg.in/src-d/go-git.v4/utils/ioutil"
)

// GitCloneOpts describes options for git clone
type GitCloneOpts struct {
	Path   string `json:"path"`
	URL    string `json:"repository_url"`
	GitRef string `json:"git_ref"`
	SSHKey string `json:"sshkey"`
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
		// check if a ssh key is given
		var auth *ssh2.PublicKeys
		hasAuth := false
		if cloneOpts.SSHKey != "" {
			bytesKey := make([]byte, 32)
			copy(bytesKey, []byte(agent.ID))
			fKey := base64.StdEncoding.EncodeToString(bytesKey)
			k := fernet.MustDecodeKeys(fKey)
			sshkey := fernet.VerifyAndDecrypt([]byte(cloneOpts.SSHKey), 60*time.Second, k)
			signer, _ := ssh.ParsePrivateKey([]byte(sshkey))
			auth = &ssh2.PublicKeys{User: "git", Signer: signer}
			hasAuth = true
		}

		references := make(memory.ReferenceStorage)
		var err2 error
		if hasAuth {
			references, err2 = inMemorylsRemote(cloneOpts.URL, auth)
			if err2 != nil {
				return fmt.Errorf("Unable to list remote reference for %s: %+v", cloneOpts.URL, err)
			}
		} else {
			references, err2 = inMemorylsRemote(cloneOpts.URL, nil)
			if err2 != nil {
				return fmt.Errorf("Unable to list remote reference for %s: %+v", cloneOpts.URL, err)
			}
		}
		reference, err := resolveRawReference(cloneOpts.GitRef, references)
		if err != nil {
			return fmt.Errorf("Reference %s not found in %s: %+v", cloneOpts.GitRef, cloneOpts.URL, err)
		}
		errMkdir := os.MkdirAll(cloneOpts.Path, 0700)
		if errMkdir != nil {
			return errMkdir
		}
		step.stream([]byte(fmt.Sprintf("\033[33m[GIT]\033[0m cloning %s, using %s %s\n", cloneOpts.URL, reference.Type(), reference.String())))
		cloneOptions := &git.CloneOptions{}
		if cloneOpts.SSHKey != "" {
			cloneOptions = &git.CloneOptions{
				URL:           cloneOpts.URL,
				Depth:         1,
				ReferenceName: reference.Name(),
				Auth:          auth,
			}
		} else {
			cloneOptions = &git.CloneOptions{
				URL:           cloneOpts.URL,
				Depth:         1,
				ReferenceName: reference.Name(),
			}
		}
		if step.Task.Stream {
			cloneOptions.Progress = step.Task.streamer.ioWriter
		}
		repository, errClone := git.PlainClone(cloneOpts.Path, false, cloneOptions)
		if errClone != nil {
			os.RemoveAll(cloneOpts.Path)
			return errClone
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

func newGitClient(url string) (transport.Transport, transport.Endpoint, error) {
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
