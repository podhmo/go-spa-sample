// Package client is the remote-mode Backend of the issues CLI: a thin
// HTTP client over /api/v1 (docs/sketch/ja/cli-and-web-api.md, Phase 3).
// It speaks the api package's DTOs — the single source for the wire
// format — and converts them to the store package's domain types so the
// CLI sees the same types either mode.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// apiMaxPerPage mirrors the server's cap on ?per_page= (api.maxPerPage);
// requests above it are stitched from multiple API pages by listWindow.
const apiMaxPerPage = 100

// listWindow serves a (page, perPage) window from a paged list endpoint.
// perPage <= apiMaxPerPage fetches a single API page (perPage <= 0 means
// "server default"); above the cap the server would silently clamp
// per_page and shift every later offset, so enough 100-item pages are
// fetched and the caller's window sliced out client-side. fetch performs
// one API request for the given API page and returns (items, total).
func listWindow[E any](ctx context.Context, page, perPage int,
	fetch func(apiPage, apiPerPage int) ([]*E, int, error)) ([]*E, int, error) {
	if page < 1 {
		page = 1
	}
	if perPage <= apiMaxPerPage {
		return fetch(page, perPage)
	}
	start := (page - 1) * perPage
	skip := start % apiMaxPerPage
	need := skip + perPage // items required counting from the first fetched page
	var acc []*E
	total := 0
	for apiPage := start/apiMaxPerPage + 1; len(acc) < need; apiPage++ {
		items, t, err := fetch(apiPage, apiMaxPerPage)
		if err != nil {
			return nil, 0, err
		}
		total = t
		acc = append(acc, items...)
		if len(items) < apiMaxPerPage {
			break
		}
	}
	if skip >= len(acc) {
		return []*E{}, total, nil
	}
	acc = acc[skip:]
	if len(acc) > perPage {
		acc = acc[:perPage]
	}
	return acc, total, nil
}

// Client calls the JSON API on a remote issues server. The zero-ish use is
// New(server, token); every method takes a context.
type Client struct {
	base  string // server root + /api/v1, e.g. http://127.0.0.1:8080/api/v1
	token string
	ws    string // workspace slug; "" means the unscoped client
	hc    *http.Client
}

// HTTPClient is the http.Client every remote call shares (the CLI's
// `issues api` included). It refuses https→http redirects: Go forwards
// Authorization on same-host redirects even when the scheme downgrades,
// which would send the token unencrypted.
var HTTPClient = &http.Client{
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return fmt.Errorf("stopped after 10 redirects")
		}
		for _, prev := range via {
			if prev.URL.Scheme == "https" && req.URL.Scheme == "http" {
				return fmt.Errorf("refusing https→http redirect to %s", req.URL)
			}
		}
		return nil
	},
}

// New returns a Client for the server's /api/v1. token is the Bearer
// credential — a shared env token or a per-user api_tokens value.
func New(server, token string) *Client {
	return &Client{
		base:  strings.TrimSuffix(server, "/") + "/api/v1",
		token: token,
		hc:    HTTPClient,
	}
}

// In returns a copy of the client scoped to workspace slug — its
// issue/discussion calls go under /api/v1/w/{slug}/... . Workspace
// management calls (ListWorkspaces etc.) and global resources (labels,
// categories) are unaffected. An empty slug leaves the client unchanged.
func (c *Client) In(slug string) *Client {
	out := *c
	out.ws = slug
	return &out
}

// wsPath prefixes a workspace resource path ("issues/1") with the
// /w/{slug} segment. The empty ws keeps the bare path — the unscoped
// client is only useful for /labels, /workspaces and `issues api`-style
// raw calls anyway.
func (c *Client) wsPath(p string) string {
	if c.ws == "" {
		return "/" + p
	}
	return "/w/" + c.ws + "/" + p
}

// Error is a non-2xx API response with the server's error message. It is
// deliberately not mapped onto the store's sentinel errors — remote
// callers compare Status (mapping 404→ErrNotFound or a machine-readable
// API error code is tracked as TODO work).
type Error struct {
	Status  int
	Message string
	Fields  map[string]string
}

func (e *Error) Error() string {
	if len(e.Fields) > 0 {
		var fs []string
		for k, v := range e.Fields {
			fs = append(fs, k+": "+v)
		}
		return fmt.Sprintf("API error %d: %s (%s)", e.Status, e.Message, strings.Join(fs, ", "))
	}
	return fmt.Sprintf("API error %d: %s", e.Status, e.Message)
}

// do sends a JSON request and decodes a JSON response body into out
// (nil out skips decoding). Non-2xx becomes *Error.
func (c *Client) do(ctx context.Context, method, path string, in, out any) error {
	var body *bytes.Reader
	if in != nil {
		raw, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(raw)
	} else {
		body = bytes.NewReader(nil)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return fmt.Errorf("%s %s: %w", method, req.URL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		var e struct {
			Err struct {
				Message string            `json:"message"`
				Fields  map[string]string `json:"fields"`
			} `json:"error"`
		}
		ae := &Error{Status: resp.StatusCode, Message: resp.Status}
		if json.NewDecoder(resp.Body).Decode(&e) == nil && e.Err.Message != "" {
			ae.Message = e.Err.Message
			ae.Fields = e.Err.Fields
		}
		return ae
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
