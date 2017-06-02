test: dep
	go test -v

dep:
	go get -u github.com/golang/dep/cmd/dep
	dep ensure -v

build: dep test
	go build -v
	go install

latest:
	docker build -t run-agent .
	docker tag run-agent pikacloud/run-agent:latest
	docker push pikacloud/run-agent:latest

clean:
	rm -rf vendor
