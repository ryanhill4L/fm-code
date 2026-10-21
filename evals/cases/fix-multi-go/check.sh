#!/usr/bin/env bash
[ "$(go run . 2>&1)" = "top=3" ]
