.PHONY: build cluster test

default: build

build:
	go build -o bin/swarm    github.com/kokereum/go-kokereum/cmd/swarm
	go build -o bin/gkok     github.com/kokereum/go-kokereum/cmd/gkok
	go build -o bin/bootnode github.com/kokereum/go-kokereum/cmd/bootnode

cluster: build
	scripts/boot-cluster.sh

test:
	go test -v github.com/kokereum/go-kokereum/swarm/...
