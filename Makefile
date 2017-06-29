VERSION := `cat VERSION`

test:
	go test -v

dep:
	go get -u github.com/golang/dep/cmd/dep
	dep ensure -v

build:
	go build -v -ldflags "-X main.version=$(VERSION)"

install:
	go install

latest:
	docker build -t run-agent .
	docker tag run-agent pikacloud/run-agent:latest
	docker tag run-agent pikacloud/run-agent:$(VERSION)
	docker push pikacloud/run-agent:latest
	docker push pikacloud/run-agent:$(VERSION)

clean:
	rm -rf vendor
