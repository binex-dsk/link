BIN=linkserv
CC=go

prefix=/usr/local
confdir=/etc
systemd_dir=${DESTDIR}${confdir}/systemd/system
nginx_dir=${DESTDIR}${confdir}/nginx
bindir=${DESTDIR}${prefix}/bin

all: vendor gen build

vendor: go.mod go.sum
	@$(CC) mod tidy
	@$(CC) mod vendor

build:
	@$(CC) build -ldflags='-s -w' -o $(BIN)

gen:
	@$(CC) generate ./...

test:
	@env $(ENV) $(CC) test ./... -cover -count 1

run: lint build
	@clear
	@env $(ENV) ./$(BIN) -v -demo -copy "2021 swurl@swurl.xyz" -url https://short.swurl.xyz -port 8080 -db /tmp/link.db -seed "secret"

dev:
	@find . -type f | grep -E '(.*)\.(go|html)' | entr -cr make run

lint:
	@golangci-lint run ./...

install-nginx:
	@install -Dm644 doc/link.nginx.conf ${nginx_dir}/sites-available/link

install-systemd:
	@install -Dm644 doc/link.service ${systemd_dir}/link.service
	@install -Dm644 doc/link.conf ${DESTDIR}/${confdir}/link.conf

install-bin:
	@install -Dm755 ${BIN} ${bindir}/${BIN}

install: build install-bin install-nginx install-systemd
