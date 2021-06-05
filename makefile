BIN=out
CC=go1.16

all: setup vendor gen build

setup:
	@env GOOD=off go get golang.org/dl/go1.16
	@env GOOD=off $(CC) download

vendor: go.mod go.sum
	@$(CC) mod tidy
	@$(CC) mod vendor

build:
	@$(CC) build -ldflags='-s -w' -o $(BIN)

gen: 
	@$(CC) generate ./...

test: 
	@env $(ENV) $(CC) test ./... -cover -count 1

run: gen build
	@clear
	@env $(ENV) ./$(BIN) -v -demo -copy "2021 i@fsh.ee" -url https://dev.fsh.ee -port 8080 -db /tmp/link_test_db_1.sql -seed secret

dev:
	@find . -type f | grep -E '(.*)\.(go|html)' | entr -cr make run
