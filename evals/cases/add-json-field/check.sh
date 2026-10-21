#!/usr/bin/env bash
jq -e '.debug == true and .name == "app" and .port == 8080 and (.tags | length) == 2' config.json >/dev/null
