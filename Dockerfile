FROM golang:1.9.2

ADD . /go/src/github.com/cofyc/xhttproxy

RUN set -eux \
    && cd /go/src/github.com/cofyc/xhttproxy \
    && go get github.com/golang/dep/cmd/dep \
    && dep ensure -v -vendor-only \
    && CGO_ENABLED=0 go install github.com/cofyc/xhttproxy/cmd/xhttproxy

FROM alpine:3.7

RUN set -eux \
    && apk --no-cache add ca-certificates

ADD example/server.pem /etc/xhttproxy/server.pem
ADD example/server.key /etc/xhttproxy/server.key
COPY --from=0 /go/bin/xhttproxy /usr/local/bin
ENTRYPOINT ["/usr/local/bin/xhttproxy"]
CMD ["-pem", "/etc/xhttproxy/server.pem", "-key", "/etc/xhttproxy/server.key"]
