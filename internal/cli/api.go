// `zatu api` <path>` — the raw-API escape hatch (gh api equivalent),
// remote mode only.
package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/podhmo/go-spa-sample/internal/client"
)

// zatu api <path> [-X METHOD] [-f name=value]... [-F name=value]...
// [--template TMPL] — call the path against the configured server and
// print the body (verbatim, or through a Go template when --template is
// given). -f/-F fields become query parameters on GET and a JSON request
// body on other methods. Non-2xx responses print the body to stderr and
// exit 1.
func apiCmd(ctx context.Context, o Options, args []string) error {
	fs := flag.NewFlagSet("api", flag.ContinueOnError)
	bf := o.addBackendFlags(fs)
	method := fs.String("method", "GET", "HTTP method")
	fs.StringVar(method, "X", "GET", "shorthand for --method")
	var strFields nameFlag
	fs.Var(&strFields, "f", "add a string parameter name=value (query on GET, JSON body otherwise; repeatable)")
	fs.Var(&strFields, "field", "long form of -f")
	var rawFields nameFlag
	fs.Var(&rawFields, "F", "add a typed parameter name=value — value parsed as int/float/bool/null (repeatable)")
	fs.Var(&rawFields, "raw-field", "long form of -F")
	tmpl := fs.String("template", "", "render the JSON body through a Go text/template")
	rest, err := o.parseLeaf(fs, "<path>", args, 1)
	if err != nil {
		return err
	}
	if len(rest) == 0 {
		return usageError{msg: "usage: zatu api <path> [-X METHOD] [-f name=value]..."}
	}
	if bf.server == "" {
		return usageError{msg: "zatu api requires --server (or ZATUDAN_SERVER)"}
	}
	warnPlaintext(o.Stderr, bf.server, bf.token)

	// Paths resolve under /api/v1 like gh api resolves under its API host,
	// so both `api issues` and `api /issues` hit the JSON endpoint.
	path := strings.TrimPrefix(rest[0], "/api/v1/")
	path = "/api/v1/" + strings.TrimPrefix(path, "/")

	verb := strings.ToUpper(*method)
	isQuery := verb == http.MethodGet || verb == http.MethodHead
	var body []byte
	params := url.Values{}
	bodyObj := map[string]any{}
	for _, kv := range strFields {
		k, v, err := splitField(kv)
		if err != nil {
			return usageError{msg: fmt.Sprintf("-f/--field %v", err)}
		}
		if isQuery {
			params.Add(k, v)
		} else {
			bodyObj[k] = v
		}
	}
	for _, kv := range rawFields {
		k, v, err := splitField(kv)
		if err != nil {
			return usageError{msg: fmt.Sprintf("-F/--raw-field %v", err)}
		}
		if isQuery {
			params.Add(k, v)
		} else {
			bodyObj[k] = typedFieldValue(v)
		}
	}
	if len(params) > 0 {
		sep := "?"
		if strings.Contains(path, "?") {
			sep = "&"
		}
		path += sep + params.Encode()
	}
	var reqBody io.Reader
	if !isQuery && len(bodyObj) > 0 {
		body, err = json.Marshal(bodyObj)
		if err != nil {
			return err
		}
		reqBody = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, verb,
		strings.TrimSuffix(bf.server, "/")+path, reqBody)
	if err != nil {
		return err
	}
	if reqBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if bf.token != "" {
		req.Header.Set("Authorization", "Bearer "+bf.token)
	}
	// client.HTTPClient refuses https→http redirects, which would leak
	// the Bearer token (Go forwards Authorization on same-host redirects).
	resp, err := client.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("request %s: %w", req.URL, err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 400 {
		fmt.Fprintf(o.Stderr, "%s %s → %s\n%s", req.Method, req.URL, resp.Status, respBody)
		return fmt.Errorf("API returned %s", resp.Status)
	}
	if *tmpl != "" {
		if len(respBody) == 0 {
			return nil
		}
		var v any
		if err := json.Unmarshal(respBody, &v); err != nil {
			return fmt.Errorf("response is not JSON (cannot apply --template): %w", err)
		}
		return writeTemplate(o.Stdout, v, *tmpl)
	}
	fmt.Fprint(o.Stdout, string(respBody))
	return nil
}

// splitField parses a `name=value` field flag.
func splitField(kv string) (string, string, error) {
	k, v, ok := strings.Cut(kv, "=")
	if !ok || k == "" {
		return "", "", fmt.Errorf("must be name=value, got %q", kv)
	}
	return k, v, nil
}

// typedFieldValue parses a -F value like gh: null, true/false, integer,
// or floating point; anything else stays a string. Non-finite floats
// (NaN, ±Inf) stay strings — encoding/json refuses to marshal them and a
// field must never make the request unencodable.
func typedFieldValue(v string) any {
	switch v {
	case "null":
		return nil
	case "true":
		return true
	case "false":
		return false
	}
	if i, err := strconv.ParseInt(v, 10, 64); err == nil {
		return i
	}
	if f, err := strconv.ParseFloat(v, 64); err == nil &&
		!math.IsNaN(f) && !math.IsInf(f, 0) {
		return f
	}
	return v
}
