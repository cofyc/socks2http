
GIT_VERSION = $(shell ./hack/version.sh | awk -F': ' '/^GIT_VERSION:/ {print $$2}')

# Only set Version if building a tag or VERSION is set
ifneq ($(VERSION),)
	LDFLAGS += -X github.com/cofyc/xhttproxy/pkg/version.VERSION=${VERSION}
else
	LDFLAGS += -X github.com/cofyc/xhttproxy/pkg/version.VERSION=${GIT_VERSION}
endif

all:
	CGO_ENABLED=0 go install -ldflags "${LDFLAGS}" github.com/cofyc/xhttproxy/cmd/xhttproxy

test:
	go test -timeout 5m github.com/cofyc/xhttproxy/...
