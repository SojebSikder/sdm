#!/bin/bash

platforms=("windows/amd64" "windows/arm64" "linux/amd64" "linux/arm64" "darwin/amd64" "darwin/arm64")

for platform in "${platforms[@]}"; do
    OS=${platform%/*}
    ARCH=${platform#*/}
    OUTPUT="build/sdm-${OS}-${ARCH}"

    # Append .exe for Windows
    if [ "$OS" == "windows" ]; then
        OUTPUT+=".exe"
    fi

    echo "Building for $OS $ARCH..."
    GOOS=$OS GOARCH=$ARCH go build -o $OUTPUT
done

echo "Build completed!"