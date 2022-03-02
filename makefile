default: build

build:
	@echo "testing..."
	@go test ./...
	@echo "compiling "
	@GOOS=darwin go build -o stream-exec_darwin main.go
	@GOOS=linux go build -o stream-exec_linux main.go

