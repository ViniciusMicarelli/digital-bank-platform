// Package qrfetch retrieves dynamic QR payloads from the receiver's bank
// over HTTP(S): the JWS at the code's URL and the bank's JWKS at
// <host>/pix/qr/jwks.json.
package qrfetch

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/ViniciusMicarelli/digital-bank-platform/services/pix-service/internal/ports"
)

type Fetcher struct {
	scheme string // "https" in production
	client *http.Client
}

func New(scheme string) *Fetcher {
	return &Fetcher{scheme: scheme, client: &http.Client{Timeout: 3 * time.Second}}
}

var _ ports.QRFetcher = (*Fetcher)(nil)

func (f *Fetcher) get(ctx context.Context, url string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := f.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %d", url, resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, limit))
}

func (f *Fetcher) Fetch(ctx context.Context, location string) (string, []ports.JWK, error) {
	if strings.Contains(location, "://") || strings.ContainsAny(location, " \\") {
		return "", nil, fmt.Errorf("bad QR location")
	}
	jws, err := f.get(ctx, f.scheme+"://"+location, 16<<10)
	if err != nil {
		return "", nil, err
	}
	host, _, _ := strings.Cut(location, "/")
	raw, err := f.get(ctx, f.scheme+"://"+host+"/pix/qr/jwks.json", 16<<10)
	if err != nil {
		return "", nil, err
	}
	var set struct {
		Keys []struct{ Kid, Kty, Crv, X, Alg string } `json:"keys"`
	}
	if err := json.Unmarshal(raw, &set); err != nil {
		return "", nil, fmt.Errorf("bad JWKS: %w", err)
	}
	keys := make([]ports.JWK, len(set.Keys))
	for i, k := range set.Keys {
		keys[i] = ports.JWK{Kid: k.Kid, Kty: k.Kty, Crv: k.Crv, X: k.X, Alg: k.Alg}
	}
	return strings.TrimSpace(string(jws)), keys, nil
}
