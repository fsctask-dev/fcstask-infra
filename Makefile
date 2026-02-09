MODULE_NAME := fcstask-infra

.PHONY: init tidy gen test

init:
	@if [ ! -f go.mod ]; then \
		echo "init module: $(MODULE_NAME)"; \
		go mod init $(MODULE_NAME); \
	else \
		echo "good. already exists"; \
	fi

tidy:
	go mod tidy

gen:
	go generate ./...

test:
	go test ./... -v

lint:
	golangci-lint run ./... --fix
