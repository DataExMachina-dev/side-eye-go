//go:build tools
// +build tools

// This package and module exists to ensure that the proper versions of
// protoc-gen-go and protoc-gen-go-grpc are used to generate the protos.

package tools

import (
	// Keep this around so that go mod tidy doesn't remove it from go.mod.
	_ "google.golang.org/grpc/cmd/protoc-gen-go-grpc"
	_ "google.golang.org/protobuf/cmd/protoc-gen-go"
	_ "google.golang.org/protobuf/proto"
)
