#!/bin/bash

set -euxo pipefail

if [[ "$OSTYPE" == "linux-gnu"* ]]; then
	osdir=linux
	folder=/usr/lib/jvm/java-17-openjdk-amd64/include
	libs=/home/buraindo/libs
elif [[ "$OSTYPE" == "darwin"* ]]; then
  osdir=darwin
  folder=/Users/e.k.ibragimov/.sdkman/candidates/java/current/include
	libs=/Users/e.k.ibragimov/Documents/University/MastersDiploma/libs
fi

export CGO_CFLAGS="-I ${folder} -I ${folder}/${osdir} -O2"
go build -o ${libs}/java_nalim_bridge.so -buildmode=c-shared nalim/bridge.go
go build -o ${libs}/java_jna_bridge.so -buildmode=c-shared jna/bridge.go
go build -o ${libs}/java_jni_bridge.so -buildmode=c-shared jni/bridge.go
