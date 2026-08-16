#!/bin/sh

set -eu
cd "$(dirname "$0")/.."
protoc -I . --go_out=. --go_opt=paths=source_relative net/proto/*.proto
