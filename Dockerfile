FROM golang:alpine

RUN apk -U add make git gcc dev86 linux-headers musl-dev docker
COPY . /go/src/github.com/pikacloud/run-agent
WORKDIR /go/src/github.com/pikacloud/run-agent
RUN mv scripts/weave /usr/local/bin && chmod +x /usr/local/bin/weave
RUN make dep install clean && rm -rf /go/pkg /go/src
CMD run-agent -auto

