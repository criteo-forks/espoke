IMG ?= blackbox-prober:latest

.PHONY: build build_linux


build:
		go build -o build/espoke .

build_linux:
		CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o build/espoke_linux_amd64 .
