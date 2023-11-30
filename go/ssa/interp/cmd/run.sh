#!/bin/bash

set -euxo pipefail

export CGO_CFLAGS="-I /usr/lib/jvm/java-17-openjdk-amd64/include -I /usr/lib/jvm/java-17-openjdk-amd64/include/linux"
go run java_bridge.go main.go
