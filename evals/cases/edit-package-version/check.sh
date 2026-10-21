#!/usr/bin/env bash
jq -e '.version == "1.3.0" and .name == "widget" and .scripts.test == "node test.js" and (.dependencies.express != null)' package.json >/dev/null
