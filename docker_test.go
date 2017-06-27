package main

import (
	"encoding/base64"
	"reflect"
	"testing"

	fernet "github.com/fernet/fernet-go"
)

func TestRegistryAuthString(t *testing.T) {
	aid := "9ee55b88a2a74a59adc48a8e425c61d7"
	key := base64.StdEncoding.EncodeToString([]byte(aid))
	k := fernet.MustDecodeKeys(key)
	password, err := fernet.EncryptAndSign([]byte("bar"), k[0])
	if err != nil {
		t.Errorf("Cannot create test encrypted password: %v", err)
	}
	e := ExternalAuthPullOpts{
		Login:    "foo",
		Password: string(password),
	}
	encodedAuth := e.registryAuthString(aid)
	plainAuth, err := base64.StdEncoding.DecodeString(encodedAuth)
	if err != nil {
		t.Errorf("Cannot decode base64 string %s", plainAuth)
	}
	want := "{\"username\": \"foo\", \"password\": \"bar\"}"
	if !reflect.DeepEqual(string(plainAuth), want) {
		t.Errorf("auth %s, want %v", plainAuth, want)
	}
}
