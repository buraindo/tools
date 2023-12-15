#!/bin/bash

set -euxo pipefail

if [[ "$OSTYPE" == "linux-gnu"* ]]; then
	osdir=linux
	folder=/usr/lib/jvm/java-17-openjdk-amd64/include
elif [[ "$OSTYPE" == "darwin"* ]]; then
  osdir=darwin
  folder=/Users/e.k.ibragimov/.sdkman/candidates/java/current/include
fi

export CGO_CFLAGS="-I ${folder} -I ${folder}/${osdir}"
go run java_jna_bridge.go bridge.go main.go
