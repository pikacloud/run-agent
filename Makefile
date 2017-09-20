VERSION := $(shell cat VERSION)
GIT_REF := $(shell git rev-parse --short HEAD || echo unsupported)
BUILDFLAGS := -ldflags "-X main.version=$(VERSION) -X main.gitRef=$(GIT_REF)"

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
	docker tag run-agent pikacloud/run-agent:$(VERSION)
	docker push pikacloud/run-agent:latest
	docker push pikacloud/run-agent:$(VERSION)

clean:
	rm -rf vendor
