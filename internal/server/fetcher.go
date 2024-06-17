package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"

	"golang.org/x/sync/singleflight"
	"google.golang.org/protobuf/proto"

	"github.com/DataExMachina-dev/side-eye-go/internal/artifactspb"
	"github.com/DataExMachina-dev/side-eye-go/internal/snapshotpb"
)

type SnapshotFetcher interface {
	FetchSnapshotProgram(ctx context.Context, key string) (*snapshotpb.SnapshotProgram, error)
}

func NewSnapshotFetcher(
	artifacts artifactspb.ArtifactStoreClient,
) SnapshotFetcher {
	return newCachedSnapshotFetcher(newRemoteSnapshotFetcher(artifacts), 2)
}

type cachedSnapshotFetcher struct {
	g           singleflight.Group
	maxCapacity int
	underlying  SnapshotFetcher
	mu          struct {
		sync.Mutex
		cache map[string]*snapshotpb.SnapshotProgram
	}
}

func newCachedSnapshotFetcher(underlying SnapshotFetcher, maxCapacity int) *cachedSnapshotFetcher {
	return &cachedSnapshotFetcher{
		g:           singleflight.Group{},
		maxCapacity: maxCapacity,
		underlying:  underlying,
		mu: struct {
			sync.Mutex
			cache map[string]*snapshotpb.SnapshotProgram
		}{
			cache: make(map[string]*snapshotpb.SnapshotProgram),
		},
	}
}

func (s *cachedSnapshotFetcher) getCached(key string) (*snapshotpb.SnapshotProgram, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.mu.cache[key]
	return p, ok
}

func (s *cachedSnapshotFetcher) FetchSnapshotProgram(ctx context.Context, key string) (*snapshotpb.SnapshotProgram, error) {
	if p, ok := s.getCached(key); ok {
		return p, nil
	}
	var called bool
	for {
		called = false
		pi, err, _ := s.g.Do(key, func() (interface{}, error) {
			called = true
			// TODO: Unhook this from the context.
			p, err := s.underlying.FetchSnapshotProgram(ctx, key)
			if err != nil {
				return nil, err
			}
			s.mu.Lock()
			defer s.mu.Unlock()
			for len(s.mu.cache) >= s.maxCapacity && len(s.mu.cache) > 0 {
				// Use this to randomize which snapshot is evicted.
				for k := range s.mu.cache {
					delete(s.mu.cache, k)
					break
				}
			}
			s.mu.cache[key] = p
			return p, nil
		})
		retry := err != nil &&
			!called &&
			ctx.Err() == nil &&
			(errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded))
		if retry {
			continue
		}
		if err != nil {
			return nil, err
		}
		return pi.(*snapshotpb.SnapshotProgram), nil
	}

}

type remoteSnapshotFetcher struct {
	client artifactspb.ArtifactStoreClient
}

func newRemoteSnapshotFetcher(client artifactspb.ArtifactStoreClient) *remoteSnapshotFetcher {
	return &remoteSnapshotFetcher{
		client: client,
	}
}

func (r *remoteSnapshotFetcher) FetchSnapshotProgram(
	ctx context.Context,
	key string,
) (*snapshotpb.SnapshotProgram, error) {
	chunks, err := r.client.GetArtifact(ctx, &artifactspb.GetArtifactRequest{
		Key:  key,
		Kind: artifactspb.GetArtifactRequest_SNAPSHOT_PROGRAM,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get snapshot program: %w", err)
	}
	var buf []byte
	for {
		chunk, err := chunks.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to receive chunk: %w", err)
		}
		buf = append(buf, chunk.Data...)
	}
	var req snapshotpb.SnapshotProgram
	if err := proto.Unmarshal(buf, &req); err != nil {
		return nil, fmt.Errorf("failed to unmarshal snapshot program: %w", err)
	}
	return &req, nil
}
