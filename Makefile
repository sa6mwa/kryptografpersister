.PHONY: scan test build clean newkey

all: scan build

scan: capslock.json
	go run golang.org/x/vuln/cmd/govulncheck@latest ./...

test:
	go test -count 1 -shuffle on -v -coverprofile cover.out ./...

build: test bin/kryptografpersister

clean:
	rm -f capslock.json cover.out bin/kryptografpersister bin/newkey
	rmdir bin

newkey: bin/newkey
	bin/newkey

cover.lcov:
	go test -v -coverprofile cover.out ./...
	go run github.com/jandelgado/gcov2lcov@latest -infile cover.out -outfile cover.lcov -use-absolute-source-path

README.md:
	go run github.com/princjef/gomarkdoc/cmd/gomarkdoc@latest ./... > README.md

capslock.json:
	go run github.com/google/capslock/cmd/capslock@latest -output json > capslock.json

bin:
	mkdir bin

bin/kryptografpersister: bin
	go build -o bin/kryptografpersister ./

bin/newkey: bin
	GOBIN=${PWD}/bin go install github.com/sa6mwa/kryptograf/cmd/newkey@latest

