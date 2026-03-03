#!/bin/sh

# If VERSION is not set, try to get it from git tags, fallback to "dev"
if [ -z "$VERSION" ]; then
    VERSION=$(git describe --tags 2>/dev/null || echo "dev")
fi

BUILD_TIME=$(date -u +'%Y-%m-%dT%H:%M:%SZ')
GIT_COMMIT=$(git rev-parse HEAD 2>/dev/null || echo "None / build outside repository")
OUTPUT=${1:-motdy}

# Add .exe extension for windows targets
if [ "$GOOS" = "windows" ]; then
    OUTPUT="${OUTPUT}.exe"
fi

go build -ldflags="-s -w -X main.Version=$VERSION -X main.BuildTime=$BUILD_TIME -X 'main.GitCommit=$GIT_COMMIT'" -o "$OUTPUT" main.go
