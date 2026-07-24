package provider

import (
	"context"
	"fmt"

	"github.com/mberwanger/quartermaster/internal/bundle"
	"github.com/mberwanger/quartermaster/internal/cache"
	"github.com/mberwanger/quartermaster/internal/oci"
)

// resolveOCI pulls a bundle artifact from a registry.
//
// The reference is resolved to a descriptor first, which is a cheap call, so a
// cached artifact is served without fetching the layer at all. The cache key is
// the registry digest, which makes a hit exact rather than a freshness guess.
func resolveOCI(ref string) (*bundle.Bundle, error) {
	ctx := context.Background()

	repo, err := oci.Open(ref)
	if err != nil {
		return nil, err
	}

	desc, err := repo.Resolve(ctx)
	if err != nil {
		return nil, fmt.Errorf("resolve oci://%s: %w", ref, err)
	}

	dir, err := cache.Populate("oci", desc.Digest.String(), func(tmp string) error {
		return repo.Extract(ctx, desc, tmp)
	})
	if err != nil {
		return nil, err
	}

	return load(dir)
}
