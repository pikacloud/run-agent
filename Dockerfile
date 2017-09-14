FROM golang:alpine

RUN apk -U add make git gcc

COPY . /go/src/github.com/pikacloud/run-agent
WORKDIR /go/src/github.com/pikacloud/run-agent
RUN make dep install clean && rm -rf /go/pkg /go/src
CMD run-agent


