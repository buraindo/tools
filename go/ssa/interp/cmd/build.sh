#!/bin/bash

set -euxo pipefail

./jni_build.sh
./jna_build.sh
