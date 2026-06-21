#!/bin/sh

node_major=$(node -p "process.versions.node.split('.')[0]" 2>/dev/null || echo 0)
if [ "$node_major" -lt 18 ]; then
    echo "ERROR: web UI tests require Node.js >= 18 (found $(node --version 2>/dev/null || echo none))."
    echo "       If using nvm: 'nvm use 20' (or newer) before running make."
    exit 1
fi

cd webstatic/ui || exit 1

echo "Installing Node.js dependencies for web UI tests ..."
npm install --no-audit --no-fund --silent
if [ $? -ne 0 ]; then
    echo "Error installing npm dependencies"
    exit 1
fi

# Restore the execute bit on the dependency bin shims. If node_modules was populated
# by an npm install on Windows (e.g. via the shared Vagrant folder), the shims arrive
# without the Unix exec bit and "npm test" fails with "tsc: Permission denied".
chmod +x node_modules/.bin/* 2>/dev/null || true

echo "Running web UI tests ..."
npm test
if [ $? -ne 0 ]; then
    echo "Test error"
    exit 1
fi
