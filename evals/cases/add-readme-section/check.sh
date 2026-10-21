#!/usr/bin/env bash
grep -qiE '^#+ +usage' README.md && grep -q 'python3 app.py' README.md && grep -qiE '^#+ +install' README.md
