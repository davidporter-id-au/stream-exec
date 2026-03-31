default: build

test:
	@go test ./... -timeout 120s

build: test
	@echo "compiling..."
	@GOOS=darwin  GOARCH=amd64 go build -o stream-exec_darwin_amd64 main.go
	@GOOS=darwin  GOARCH=arm64 go build -o stream-exec_darwin_arm64 main.go
	@GOOS=linux   GOARCH=amd64 go build -o stream-exec_linux_amd64  main.go
	@GOOS=linux   GOARCH=arm64 go build -o stream-exec_linux_arm64  main.go
