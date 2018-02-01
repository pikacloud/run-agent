#!/bin/bash

agent_labels=""
WELCOME_TEXT="Run-agent interactive installation\n##################################\n"
TOKEN_HELP_TEXT="1. Run-agent need your Pikacloud API Token to start:\nyou can generate and retrieve one here: https://pikacloud.com/settings/api/tokens/\n"
LABELS_HELP_TEXT="2. You can configure here some labels for your Run-agent (or none if you prefer).\nAgents labels are conditions used by Pikacloud to select specific hosts or group of hosts to run workload or schedule resources.\n"

function askToken() {
	echo -n "Enter your Pikacloud API token: "
	read api_token
	if [[ -z "$api_token" ]];
	then
		echo "Please specify your token!"
		askToken
	fi
}

function askLabels() {
	echo -n "Agent label (leave empty and press Enter to finish labels configuration:"
	read label
	if [[ ! -z "$label" ]];
	then
		if [[ -z "$agent_labels" ]];
		then
			agent_labels="$label"
		else
			agent_labels="$label,$agent_labels"
		fi
		askLabels
	fi
}


EXISTS=$(docker ps -q -f "ancestor=pikacloud/run-agent" -f "status=running" | xargs)
if [[ ! -z "$EXISTS" ]];
then
	docker ps -f "id=$EXISTS"
	echo -e "/!\ You already have a run-agent running in container $EXISTS\nPress Enter to destroy this run-agent and create a new one. Cancel with ctrl-c."
	read override_agent
	echo -ne "Removing container... "
  docker rm -fv $EXISTS
fi

echo -e $WELCOME_TEXT

echo -e $TOKEN_HELP_TEXT
askToken

echo -e $LABELS_HELP_TEXT
askLabels

docker pull pikacloud/run-agent
docker run -e PIKACLOUD_API_TOKEN="$api_token" \
           -e PIKACLOUD_AGENT_LABELS="$agent_labels" \
           --net=host -v /var/run/docker.sock:/var/run/docker.sock \
           --restart=always \
           --name=run-agent \
           -d pikacloud/run-agent
sleep 2
docker logs --tail=10 run-agent
