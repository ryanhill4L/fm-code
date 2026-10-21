#!/usr/bin/env bash
[ "$(node index.js 2>&1)" = 'total: $110.00' ]
