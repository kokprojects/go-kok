FROM docker.io/golang

RUN apt-get update
RUN apt-get upgrade -y
RUN apt-get install vim-gtk -y
RUN mkdir /usr/local/go/go_workpace
RUN mkdir /usr/local/go/go_workpace/src
RUN mkdir /usr/local/go/go_workpace/pkg
RUN mkdir /usr/local/go/go_workpace/bin

ENV GOROOT /usr/local/go
ENV GOBIN $GOROOT/bin
ENV GOPATH /usr/local/go/go_workpace
ENV PATH $PATH:$GOPATH:$GOBIN:$GOROOT

WORKDIR /usr/local/go/go_workpace/src/github.com/kokprojects/go-kok
ADD . /usr/local/go/go_workpace/src/github.com/kokprojects/go-kok

RUN go run build/ci.go install ./cmd/gkok
EXPOSE 8545 8546 30303 30303/udp 30304/udp

