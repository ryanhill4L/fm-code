#!/usr/bin/env bash
grep -qx 'PORT=8080' .env.example && grep -qx 'DEBUG=false' .env.example && grep -qx 'DB_URL=postgres://localhost/app' .env.example && ! grep -q 'PORT=3000' .env.example
