#!/usr/bin/env bash
go build ./... && go vet ./... && grep -q "\"/version\"" main.go && [ "$(grep -c /version main.go)" -ge 2 ]
