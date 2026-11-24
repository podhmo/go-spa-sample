package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestAPICommandFields exercises the wire behavior of `zatu api`: -f/-F
// fields are query parameters on GET and a JSON body on other methods;
// --template renders the decoded response.
func TestAPICommandFields(t *testing.T) {
	type captured struct {
		method string
		query  string
		body   string
	}
	var got captured
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = captured{method: r.Method, query: r.URL.RawQuery}
		b, _ := io.ReadAll(r.Body)
		got.body = string(b)
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"ok":true}`)
	}))
	defer srv.Close()

	o := Options{}
	runAPI := func(t *testing.T, args ...string) (int, string, string) {
		t.Helper()
		return run(t, o, append(args, "--server", srv.URL)...)
	}

	code, _, errOut := runAPI(t, "api", "issues", "-f", "state=all", "-F", "per_page=5")
	if code != ExitOK {
		t.Fatalf("api GET: exit %d\n%s", code, errOut)
	}
	if got.method != http.MethodGet || got.body != "" {
		t.Fatalf("api GET: method %q body %q", got.method, got.body)
	}
	if !strings.Contains(got.query, "state=all") || !strings.Contains(got.query, "per_page=5") {
		t.Fatalf("api GET query: %q", got.query)
	}

	code, _, errOut = runAPI(t, "api", "issues", "-X", "POST",
		"-f", "title=x", "-F", "n=3", "-F", "flag=true", "-F", "nothing=null")
	if code != ExitOK {
		t.Fatalf("api POST: exit %d\n%s", code, errOut)
	}
	if got.method != http.MethodPost {
		t.Fatalf("api POST method: %q", got.method)
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(got.body), &body); err != nil {
		t.Fatalf("api POST body not JSON: %q (%v)", got.body, err)
	}
	if body["title"] != "x" || body["n"] != float64(3) || body["flag"] != true || body["nothing"] != nil {
		t.Fatalf("api POST body: %v", body)
	}

	// A non-finite float stays a string rather than making the body
	// unencodable (json.Marshal rejects NaN/Inf).
	code, _, errOut = runAPI(t, "api", "issues", "-X", "POST", "-F", "v=NaN")
	if code != ExitOK {
		t.Fatalf("api -F v=NaN: exit %d\n%s", code, errOut)
	}
	if err := json.Unmarshal([]byte(got.body), &body); err != nil {
		t.Fatalf("api POST NaN body not JSON: %q", got.body)
	}
	if body["v"] != "NaN" {
		t.Fatalf("api -F NaN: body %v", body)
	}

	code, out, _ := runAPI(t, "api", "issues", "--template", "{{.ok}}")
	if code != ExitOK || strings.TrimSpace(out) != "true" {
		t.Fatalf("api --template: exit %d out %q", code, out)
	}

	code, _, _ = runAPI(t, "api", "issues", "-f", "novalue")
	if code != ExitUsage {
		t.Fatalf("api -f novalue: want exit %d got %d", ExitUsage, code)
	}
}

func TestCompletion(t *testing.T) {
	o := Options{}

	code, out, _ := run(t, o, "completion", "bash")
	if code != ExitOK || !strings.Contains(out, "complete -F") {
		t.Fatalf("completion bash: exit %d\n%s", code, out)
	}
	code, out, _ = run(t, o, "completion", "zsh")
	if code != ExitOK || !strings.Contains(out, "compdef _zatu zatu") {
		t.Fatalf("completion zsh must register via compdef: exit %d\n%s", code, out)
	}
	code, out, _ = run(t, o, "completion", "fish")
	if code != ExitOK || !strings.Contains(out, "complete -c zatu") {
		t.Fatalf("completion fish: exit %d\n%s", code, out)
	}
	if code, _, _ := run(t, o, "completion"); code != ExitUsage {
		t.Fatalf("completion (no shell): want %d got %d", ExitUsage, code)
	}
	if code, _, _ := run(t, o, "completion", "tcsh"); code != ExitUsage {
		t.Fatalf("completion tcsh: want %d got %d", ExitUsage, code)
	}
}
