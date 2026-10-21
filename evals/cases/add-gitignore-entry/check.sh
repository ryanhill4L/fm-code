#!/usr/bin/env bash
grep -qxE '\.env' .gitignore && grep -qx 'node_modules/' .gitignore && grep -qx '\*.log' .gitignore && grep -qx 'dist/' .gitignore
