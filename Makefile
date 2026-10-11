.PHONY: build test check demo

build:
	go build -trimpath -o bin/gitlab-mr-graph ./cmd/gitlab-mr-graph

test:
	go test -race ./...
	node --test internal/server/graph_view.test.cjs

check:
	go vet ./...
	node --check internal/server/web/app.js
	node --check internal/server/web/graph-view.js
	node --check internal/server/web/theme.js
	sh -n scripts/install.sh
	bash -n scripts/build-release.sh

demo: build
	./bin/gitlab-mr-graph --demo
