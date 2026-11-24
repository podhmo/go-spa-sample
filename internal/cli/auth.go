// `zatu auth token` — per-user API token management (local mode only).
// Issuance, listing and revocation all write the api_tokens table directly
// instead of going through HTTP: the point is managing a credential
// without exposing password verification over HTTP (cli-and-web-api.md
// 「認証」節).
package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"strings"

	"github.com/podhmo/go-spa-sample/internal/store"
)

const authUsage = `usage: zatu auth token [<action>] [flags]   (local mode only)

actions:
  token --user NAME     issue a per-user API token
  token list            list issued tokens [--user NAME]
  token revoke          revoke a token (ID|--token T)

"zatu auth token <action> --help" shows an action's flags.
`

// zatu auth token --user NAME | token list [--user NAME] | token revoke (ID|--token T)
func authCmd(ctx context.Context, o Options, args []string) error {
	if len(args) > 0 && isHelpArg(args[0]) {
		fmt.Fprint(o.Stdout, authUsage)
		return nil
	}
	if len(args) == 0 || args[0] != "token" {
		return usageError{msg: "usage: zatu auth token --user NAME | token list [--user NAME] | token revoke (ID|--token T)"}
	}
	args = args[1:]
	if len(args) > 0 {
		switch args[0] {
		case "list":
			return authTokenList(ctx, o, args[1:])
		case "revoke":
			return authTokenRevoke(ctx, o, args[1:])
		}
	}
	return authTokenCreate(ctx, o, args)
}

// openTokenStore rejects remote mode and opens the local token store.
func (o Options) openTokenStore(bf *backendFlags) (TokenStore, error) {
	if bf.server != "" {
		return nil, usageError{msg: "auth token is a local-mode command; it writes the database directly and ignores --server"}
	}
	return o.OpenTokenStore(bf.dbPath())
}

// zatu auth token --user NAME — issue a new token. The plaintext is
// shown once and never stored, so stdout stays the bare token
// (`export ZATUDAN_TOKEN=$(...)`).
func authTokenCreate(ctx context.Context, o Options, args []string) error {
	fs := flag.NewFlagSet("auth token", flag.ContinueOnError)
	bf := o.addBackendFlags(fs)
	user := fs.String("user", "", "login name of the user to issue a token for (required)")
	if _, err := o.parseLeaf(fs, "--user NAME", args, 0); err != nil {
		return err
	}
	if *user == "" {
		return usageError{msg: "--user is required"}
	}

	st, err := o.openTokenStore(bf)
	if err != nil {
		return err
	}
	defer st.Close()

	u, err := st.UserByName(ctx, *user)
	if errors.Is(err, store.ErrNotFound) {
		return fmt.Errorf("no such user: %s", *user)
	}
	if err != nil {
		return err
	}
	token, err := st.CreateAPIToken(ctx, u.ID)
	if err != nil {
		return err
	}
	fmt.Fprintln(o.Stdout, token)
	fmt.Fprintf(o.Stderr, "issued API token for user %q (shown once; store it safely)\n", u.Name)
	return nil
}

// zatu auth token list [--user NAME] — the stored (hashed) token ids
// with their owners. Only a prefix of the hash is printed: enough to name
// a token for `auth token revoke`, never enough to use it.
func authTokenList(ctx context.Context, o Options, args []string) error {
	fs := flag.NewFlagSet("auth token list", flag.ContinueOnError)
	bf := o.addBackendFlags(fs)
	user := fs.String("user", "", "show only this user's tokens")
	jsonFields := fs.String("json", "", "output JSON with the named fields (comma-separated)")
	tmpl := fs.String("template", "", "render output through a Go text/template")
	if _, err := o.parseLeaf(fs, "", args, 0); err != nil {
		return err
	}
	if *jsonFields != "" && *tmpl != "" {
		return usageError{msg: "--json and --template are mutually exclusive"}
	}

	st, err := o.openTokenStore(bf)
	if err != nil {
		return err
	}
	defer st.Close()

	tokens, err := st.ListAPITokens(ctx)
	if err != nil {
		return err
	}
	if *user != "" {
		var filtered []*store.APIToken
		for _, t := range tokens {
			if t.User != nil && strings.EqualFold(t.User.Name, *user) {
				filtered = append(filtered, t)
			}
		}
		tokens = filtered
	}
	if *jsonFields != "" {
		return writeJSONFields(o.Stdout, apiTokenViews(tokens), *jsonFields)
	}
	if *tmpl != "" {
		return writeTemplate(o.Stdout, apiTokenViews(tokens), *tmpl)
	}
	for _, t := range tokens {
		fmt.Fprintf(o.Stdout, "%s\t%s\t%s\n",
			t.ID[:8], authorName(t.User), fmtTimeShort(t.CreatedAt))
	}
	return nil
}

// zatu auth token revoke (ID|--token T) — delete one token row. ID is
// the hash prefix printed by `auth token list`; an explicit --token takes
// the plaintext value itself (e.g. revoking the token in $ZATUDAN_TOKEN).
// --token must be given on the command line — an ZATUDAN_TOKEN env default
// does not count, so remote-mode users cannot revoke by accident.
func authTokenRevoke(ctx context.Context, o Options, args []string) error {
	fs := flag.NewFlagSet("auth token revoke", flag.ContinueOnError)
	bf := o.addBackendFlags(fs)
	rest, err := o.parseLeaf(fs, "(ID|--token T)", args, 1)
	if err != nil {
		return err
	}
	explicitToken := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "token" {
			explicitToken = true
		}
	})
	if !explicitToken && len(rest) == 0 {
		return usageError{msg: "usage: zatu auth token revoke (ID|--token T)"}
	}
	if explicitToken && len(rest) > 0 {
		return usageError{msg: "pass either an id or --token, not both"}
	}

	st, err := o.openTokenStore(bf)
	if err != nil {
		return err
	}
	defer st.Close()

	if explicitToken {
		// Revoking by plaintext is a no-op when unknown — the caller
		// learns nothing about other stored tokens either way.
		if err := st.DeleteAPIToken(ctx, bf.token); err != nil {
			return err
		}
		fmt.Fprintln(o.Stdout, "revoked API token")
		return nil
	}

	// The id is a hash prefix: resolve it to exactly one stored token.
	prefix := rest[0]
	tokens, err := st.ListAPITokens(ctx)
	if err != nil {
		return err
	}
	var matches []*store.APIToken
	for _, t := range tokens {
		if strings.HasPrefix(t.ID, prefix) {
			matches = append(matches, t)
		}
	}
	switch len(matches) {
	case 0:
		return fmt.Errorf("no API token with id prefix %q", prefix)
	case 1:
	default:
		return fmt.Errorf("id prefix %q is ambiguous: matches %d tokens", prefix, len(matches))
	}
	if err := st.DeleteAPITokenByID(ctx, matches[0].ID); err != nil {
		return err
	}
	fmt.Fprintf(o.Stdout, "revoked API token %s (user %s)\n",
		matches[0].ID[:8], authorName(matches[0].User))
	return nil
}
