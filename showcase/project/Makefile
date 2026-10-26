build:
	go build -o bookshop .

test:
	go test ./...
	node web/format.test.js

run: build
	./bookshop

.PHONY: build test run
