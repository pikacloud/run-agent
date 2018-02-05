package main

import (
	"crypto"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"strings"

	"github.com/blang/semver"
)

func sign(parsePrivKey func([]byte) (crypto.Signer, error), privatePEM string, source []byte) ([]byte, error) {
	block, _ := pem.Decode([]byte(privatePEM))
	if block == nil {
		return nil, fmt.Errorf("Failed to parse private key PEM")
	}

	priv, err := parsePrivKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("Failed to parse private key DER: %v", err)
	}

	checksum := sha256.Sum256(source)
	sig, err := priv.Sign(rand.Reader, checksum[:], crypto.SHA256)
	if err != nil {
		return nil, fmt.Errorf("Failed to sign: %v", sig)
	}

	return sig, nil
}

func signec(privatePEM string, source []byte) ([]byte, error) {
	parseFn := func(p []byte) (crypto.Signer, error) { return x509.ParseECPrivateKey(p) }
	return sign(parseFn, privatePEM, source)
}

func semverParse(v string) (semver.Version, error) {
	s := strings.Split(v, ".")
	if len(s) < 3 {
		v = fmt.Sprintf("%s.0", v)
		return semverParse(v)
	}
	semVer, err := semver.Parse(v)
	if err != nil {
		return semver.Version{}, err
	}
	return semVer, nil
}
