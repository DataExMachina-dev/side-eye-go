#!/bin/bash
set -eux

# Use the protoc-gen-go and protoc-gen-go-grpc binaries from the current
# directory, not the ones in $PATH.
DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
TOOLS_DIR="$(dirname "$DIR")/tools"
export PATH="$TOOLS_DIR:$PATH"

protoc \
  --go_out=./ \
  --go-grpc_out=./ \
  --go_opt=paths=source_relative \
  --go-grpc_opt=paths=source_relative \
  --go_opt=Mmachina.proto=github.com/DataExMachina-dev/side-eye-go/internal/machinapb \
  --go-grpc_opt=Mmachina.proto=github.com/DataExMachina-dev/side-eye-go/internal/machinapb \
  --go_opt=Mprocess.proto=github.com/DataExMachina-dev/side-eye-go/internal/machinapb \
  --go-grpc_opt=Mprocess.proto=github.com/DataExMachina-dev/side-eye-go/internal/machinapb \
  --go_opt=Mchunk.proto=github.com/DataExMachina-dev/side-eye-go/internal/chunkpb \
  --go-grpc_opt=Mchunk.proto=github.com/DataExMachina-dev/side-eye-go/internal/chunkpb \
  -I . \
  machina.proto process.proto
