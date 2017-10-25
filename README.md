# Pikacloud run-agent

[![Build Status](https://travis-ci.org/pikacloud/run-agent.svg?branch=master)](https://travis-ci.org/pikacloud/run-agent)
[![GoDoc](https://godoc.org/github.com/pikacloud/run-agent?status.png)](https://godoc.org/github.com/pikacloud/run-agent)

On-premise `run-agent` for [Pikacloud](https://pikacloud.com) services.

## Features

- Docker container management
- aggregate multiples hosts from any provider (bare-metal servers, virtual machines, development machines...) to build `run-agent` clusters
- stream and store docker containers logs
- remote terminals
- build Docker images from Git repository
- realtime metrics





## How to run Pikacloud run-agent

Requirements:
- Up to date Docker daemon
- Access to the Docker socket
- a Pikacloud API token in order to connect the Agent to your Pikacloud account: [available here.](https://pikacloud.com/settings/api/tokens/)

Launch run-agent with Docker:

```
docker run -e PIKACLOUD_API_TOKEN=xxxxxxxxx -e PIKACLOUD_AGENT_LABELS="cluster=nginx,dc1" --restart=always --net=host -v /var/run/docker.sock:/var/run/docker.sock --name=run-agent pikacloud/run-agent
```


You can assign labels to your agent with `PIKACLOUD_AGENT_LABELS` environment variable. Containers can be assigned by selecting agents with their labels. Example:

```
# Separate each label with a comma
PIKACLOUD_AGENT_LABELS="web,database=mysql,region=eu-west-1"
```


## For developers

```
$ go get github.com/pikacloud/run-agent
$ cd $HOME/go/src/github.com/run-agent
$ [Env Variables...] ./run-agent
```

### Environment variables

| Name | Default | Role |
|---|---|---|
| PIKACLOUD_API_TOKEN | None |API private token to authenticate with Pikacloud API |
| PIKACLOUD_AGENT_URL | `https://pikacloud.com/api/` |Pikacloud API URL |
| PIKACLOUD_AGENT_HOSTNAME | None | Override hostname reported by the agent  |
| PIKACLOUD_AGENT_LABELS | None |List of strings separated by a comma `"aws,eu-west-1a,nginx,magento"` |
| PIKACLOUD_WS_URL | `wss://ws.pikacloud.com`| Pikacloud Websocket server |
| PIKACLOUD_XHYVE  | false | Darwin specifics for garbase collecting interrupted docker exec |
| PIKACLOUD_XHYVE_TTY  | `~/Library/Containers/com.docker.docker/Data/com.docker.driver.amd64-linux/tty`  | Darwin specific path of xhyve VM tty |
| LOG_LEVEL | info |Defines log level (`debug` for the most verbose logging level) |


## Pikacloud API

[https://pikacloud.com/api/v1/](https://pikacloud.com/api/v1/)
