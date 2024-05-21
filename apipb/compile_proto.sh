#!/bin/bash
set -eux

protoc \
  --go_out=./ \
  --go-grpc_out=./ \
  --go_opt=paths=source_relative \
  --go-grpc_opt=paths=source_relative \
  --go_opt=Mapi.proto=stackviz/api/apipb \
  --go-grpc_opt=Mapi.proto=stackviz/api/apipb \
  api.proto
