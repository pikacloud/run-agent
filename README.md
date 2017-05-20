# Pikacloud run-agent

[![Build Status](https://travis-ci.org/pikacloud/run-agent.svg?branch=master)](https://travis-ci.org/pikacloud/run-agent)
[![GoDoc](https://godoc.org/github.com/pikacloud/run-agent?status.png)](https://godoc.org/github.com/pikacloud/run-agent)

Pikacloud run agent.

## Usage

```
$ go get github.com/pikacloud/run-agent
$ PIKACLOUD_API_TOKEN=cec35315ed6a4ee2a4f62dddae16c75c [PIKACLOUD_AGENT_URL=http://localhost/api/] ./run-agent
```

You can assign labels to your agent with `PIKACLOUD_AGENT_LABELS` environment variable. Then workload can be assigned by selecting agents by their labels. Example:

```
# Separate each label with a comma
PIKACLOUD_AGENT_LABELS="web,database=mysql,region=eu-west-1"
```

## Pikacloud API

[https://pikacloud.com/api/v1/](https://pikacloud.com/api/v1/)
