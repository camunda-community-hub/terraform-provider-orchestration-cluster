default: fmt lint install generate

build:
	go build -v ./...

install: build
	go install -v ./...

lint:
	golangci-lint run

generate:
	cd tools; go generate ./...

# Regenerate, then fail if generation left untracked files under docs/ or examples/.
# pre-commit and git diff only notice changed tracked files, so a new generated page would pass.
generate-check: generate
	@untracked=$$(git ls-files --others --exclude-standard -- docs examples); \
	if [ -n "$$untracked" ]; then \
		echo "make generate left untracked files, git add them:"; echo "$$untracked"; exit 1; \
	fi

fmt:
	gofmt -s -w -e .

test:
	go test -v -cover -timeout=120s -parallel=10 ./...

testacc:
	TF_ACC=1 go test -v -cover -timeout 120m ./...

.PHONY: fmt lint test testacc build install generate generate-check
