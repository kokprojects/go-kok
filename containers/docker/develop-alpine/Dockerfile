FROM alpine:3.5

RUN \
  apk add --update go git make gcc musl-dev linux-headers ca-certificates && \
  git clone --depth 1 https://github.com/kokereum/go-kokereum && \
  (cd go-kokereum && make gkok) && \
  cp go-kokereum/build/bin/gkok /gkok && \
  apk del go git make gcc musl-dev linux-headers && \
  rm -rf /go-kokereum && rm -rf /var/cache/apk/*

EXPOSE 8545
EXPOSE 30303

ENTRYPOINT ["/gkok"]
