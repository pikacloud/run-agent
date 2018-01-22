package main

import (
	"encoding/base64"
	"fmt"
	"time"

	fernet "github.com/fernet/fernet-go"
)

func (e *ExternalAuthPullOpts) registryAuthString(aid string) string {
	key := base64.StdEncoding.EncodeToString([]byte(aid))
	k := fernet.MustDecodeKeys(key)
	password := fernet.VerifyAndDecrypt([]byte(e.Password), 60*time.Second, k)
	data := []byte(fmt.Sprintf("{\"username\": \"%s\", \"password\": \"%s\"}", e.Login, string(password)))
	str := base64.StdEncoding.EncodeToString(data)
	return str
}

func (agent *Agent) internalRegistryAuthString() string {
	data := []byte(fmt.Sprintf("{\"username\": \"%s\", \"password\": \"%s\"}", agent.User.Email, apiToken))
	str := base64.StdEncoding.EncodeToString(data)
	return str
}
