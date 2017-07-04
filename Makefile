VERSION := $(shell cat VERSION)
BUILDFLAGS := -ldflags "-X main.version=$(VERSION)"

test:
	go test -v

dep:
	go get -u github.com/golang/dep/cmd/dep
	dep ensure -v

build:
	go build -v $(BUILDFLAGS)

install:
	go install -v $(BUILDFLAGS)

latest:
	docker build -t run-agent .
	docker tag run-agent pikacloud/run-agent:latest
	docker push pikacloud/run-agent:latest

clean:
	rm -rf vendor
