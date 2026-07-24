package provider

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/mberwanger/quartermaster/internal/archive"
	"github.com/mberwanger/quartermaster/internal/bundle"
	"github.com/mberwanger/quartermaster/internal/cache"
)

// maxTarball bounds what will be pulled over https, so a wrong URL cannot fill
// the disk.
const maxTarball = 512 << 20 // 512 MiB

// resolveHTTPS fetches a bundle tarball.
//
// The cache is keyed by the URL rather than by content, because the content
// digest is not known until after the download and the point of the cache is to
// skip the download. That is safe for the versioned, immutable URLs this is
// meant for — a release asset — and wrong for a URL whose content changes under
// the same name, which is why a manifest should pin a version in the path.
func resolveHTTPS(url string, auth Auth) (*bundle.Bundle, error) {
	// The URL alone keys the cache. Credentials authorize the fetch but do not
	// change the content, so they must not change the key, or the same asset
	// pulled with two tokens would cache twice.
	key := sha256.Sum256([]byte(url))

	dir, err := cache.Populate("https", hex.EncodeToString(key[:]), func(tmp string) error {
		body, err := fetch(url, auth)
		if err != nil {
			return err
		}
		return archive.UntarGz(body, tmp)
	})
	if err != nil {
		return nil, err
	}

	return load(dir)
}

func fetch(url string, auth Auth) ([]byte, error) {
	client := &http.Client{Timeout: 5 * time.Minute}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	switch {
	case auth.Token != "":
		req.Header.Set("Authorization", "Bearer "+auth.Token)
	case auth.Username != "" || auth.Password != "":
		req.SetBasicAuth(auth.Username, auth.Password)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch %s: %s", url, resp.Status)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxTarball))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", url, err)
	}
	return body, nil
}
