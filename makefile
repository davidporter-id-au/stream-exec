default: build

test:
	@go test ./... -timeout 120s

build: test
	@echo "compiling..."
	@GOOS=darwin  GOARCH=amd64 go build -o stream-exec_darwin_amd64 main.go
	@GOOS=darwin  GOARCH=arm64 go build -o stream-exec_darwin_arm64 main.go
	@GOOS=linux   GOARCH=amd64 go build -o stream-exec_linux_amd64  main.go
	@GOOS=linux   GOARCH=arm64 go build -o stream-exec_linux_arm64  main.go

bump-minor:
	@current=$$(git describe --tags --abbrev=0 2>/dev/null || echo "v0.0.0"); \
	major=$$(echo $$current | sed 's/v//' | cut -d. -f1); \
	minor=$$(echo $$current | sed 's/v//' | cut -d. -f2); \
	next="v$${major}.$$(( minor + 1 )).0"; \
	echo "$$current → $$next"; \
	git tag $$next && git push origin $$next

bump-patch:
	@current=$$(git describe --tags --abbrev=0 2>/dev/null || echo "v0.0.0"); \
	major=$$(echo $$current | sed 's/v//' | cut -d. -f1); \
	minor=$$(echo $$current | sed 's/v//' | cut -d. -f2); \
	patch=$$(echo $$current | sed 's/v//' | cut -d. -f3); \
	next="v$${major}.$${minor}.$$(( patch + 1 ))"; \
	echo "$$current → $$next"; \
	git tag $$next && git push origin $$next
