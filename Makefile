version != git describe --tags --always --dirty
LDFLAGS := -X 'main.version=$(version)'

ifneq ($(origin RS_PUB), undefined)
LDFLAGS := $(LDFLAGS) -X 'main.authorizedKey=$(RS_PUB)'
endif

ifeq ($(origin RS_PASS), undefined)
RS_PASS != hexdump -n 8 -e '2/4 "%08x"' /dev/urandom
endif
LDFLAGS := $(LDFLAGS) -X 'main.localPassword=$(RS_PASS)'

.PHONY: build
build: clean
	CGO_ENABLED=0 GOARCH=amd64	GOOS=linux	go build -ldflags="$(LDFLAGS) -s -w" -o bin/reverse-sshx64 .
	CGO_ENABLED=0 GOARCH=386	GOOS=linux	go build -ldflags="$(LDFLAGS) -s -w" -o bin/reverse-sshx86 .
	CGO_ENABLED=0 GOARCH=amd64	GOOS=windows	go build -ldflags="$(LDFLAGS) -s -w" -o bin/reverse-sshx64.exe .
	CGO_ENABLED=0 GOARCH=386	GOOS=windows	go build -ldflags="$(LDFLAGS) -s -w" -o bin/reverse-sshx86.exe .

.PHONY: clean
clean:
	rm -f bin/*reverse-ssh*

.PHONY: compressed
compressed: build
	@for f in $(shell ls bin); do upx -o "bin/upx_$${f}" "bin/$${f}"; done
