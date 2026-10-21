#!/usr/bin/env bash
[ "$(go run . 2>&1)" = "5 3" ]
