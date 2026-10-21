#!/usr/bin/env bash
jq -e '(.dependencies | has("lodash") | not) and (.dependencies | has("express")) and (.dependencies | has("zod")) and .version == "0.4.0"' package.json >/dev/null
