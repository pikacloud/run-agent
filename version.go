package main

import (
	"archive/tar"
	"compress/gzip"
	"encoding/hex"
	"fmt"
	"io"
	"runtime"

	update "github.com/inconshreveable/go-update"
)

type versionUpdate struct {
	Version         string `json:"version"`
	Os              string `json:"os"`
	Arch            string `json:"arch"`
	ArchiveURL      string `json:"archive_url"`
	ArchiveChecksum string `json:"archive_checksum"`
	BinaryChecksum  string `json:"binary_checksum"`
	BinarySignature string `json:"binary_signature"`
}

func (agent *Agent) getLatestVersion() (*versionUpdate, error) {
	versionURI := fmt.Sprintf("run/agent-version/latest/?os=%s&arch=%s", runtime.GOOS, runtime.GOARCH)
	v := &versionUpdate{}
	err := pikacloudClient.Get(versionURI, &v)
	if err != nil {
		return nil, err
	}
	return v, nil
}

func (agent *Agent) update() error {
	v, err := agent.getLatestVersion()
	if err != nil {
		return err
	}
	if v.Version == version {
		logger.Infof("Agent version %s is up to date.", version)
		return nil
	}
	logger.Infof("Preparing update from %s to %s", version, v.Version)
	checksum, err := hex.DecodeString(v.BinaryChecksum)
	if err != nil {
		return err
	}
	signature, err := hex.DecodeString(v.BinarySignature)
	if err != nil {
		return err
	}
	logger.Infof("Downloading %s", v.ArchiveURL)
	response, err := httpClient.Get(v.ArchiveURL)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	logger.Infof("Uncompressing run-agent %v update", v.Version)
	gzReader, err := gzip.NewReader(response.Body)
	if err != nil {
		return err
	}
	binaryReader, binaryWriter := io.Pipe()
	defer binaryReader.Close()
	defer binaryWriter.Close()
	tarReader := tar.NewReader(gzReader)
	for {
		header, errTarReader := tarReader.Next()
		if errTarReader == io.EOF {
			break
		}
		if errTarReader != nil {
			return err
		}
		if header.Name == "run-agent" {
			go func() {
				if _, errIoTar := io.Copy(binaryWriter, tarReader); errIoTar != nil {
					logger.Fatalf("ExtractTarGz: Copy() failed: %s", errIoTar.Error())
				}
				defer binaryWriter.Close()
			}()
			break
		}
	}
	logger.Infof("Applying update")
	opts := update.Options{
		Checksum:  checksum,
		Signature: signature,
	}
	publicKey := `
-----BEGIN PUBLIC KEY-----
MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAEA+UzJD+more/0adp0/IKYGl9OgO1
A5t0SQ22qx1j3A6ozKZpNGTQ8JZCudWza3vuZ9RcjsBfbBZVmWZwqDMYbQ==
-----END PUBLIC KEY-----`
	err = opts.SetPublicKeyPEM([]byte(publicKey))
	if err != nil {
		return fmt.Errorf("Could not parse public key: %v", err)
	}
	errUpdate := update.Apply(binaryReader, opts)
	if errUpdate != nil {
		return errUpdate
	}
	reason := fmt.Sprintf("Update done from %s to version %s", version, v.Version)
	shutdown(249, reason)
	return nil
}
