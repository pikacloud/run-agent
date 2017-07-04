# Pikacloud run-agent

[![Build Status](https://travis-ci.org/pikacloud/run-agent.svg?branch=master)](https://travis-ci.org/pikacloud/run-agent)
[![GoDoc](https://godoc.org/github.com/pikacloud/run-agent?status.png)](https://godoc.org/github.com/pikacloud/run-agent)

Pikacloud run agent.

## How to run Pikacloud run-agent

Requirements:
- Docker
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
$ PIKACLOUD_API_TOKEN=cec35315ed6a4ee2a4f62dddae16c75c [PIKACLOUD_AGENT_URL=http://localhost/api/] ./run-agent
```



## Pikacloud API

[https://pikacloud.com/api/v1/](https://pikacloud.com/api/v1/)
