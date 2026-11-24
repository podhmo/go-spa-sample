// `zatu workspace` — list/create/delete workspaces. The workspace
// itself is the argument, so these verbs ignore --workspace.
package cli

import (
	"context"
	"flag"
	"fmt"
	"strings"

	"github.com/podhmo/go-spa-sample/internal/store"
)

const workspaceUsage = `usage: zatu workspace <action> [flags]

actions:
  list     list workspaces
  create   create a workspace: SLUG [--name N] [--description D]
  delete   delete a workspace and its contents: SLUG

"zatu workspace <action> --help" shows an action's flags.
`

func workspaceCmd(ctx context.Context, o Options, args []string) error {
	if len(args) == 0 {
		return usageError{msg: "workspace requires a verb: list|create|delete"}
	}
	if isHelpArg(args[0]) {
		fmt.Fprint(o.Stdout, workspaceUsage)
		return nil
	}
	switch args[0] {
	case "list":
		return workspaceList(ctx, o, args[1:])
	case "create":
		return workspaceCreate(ctx, o, args[1:])
	case "delete":
		return workspaceDelete(ctx, o, args[1:])
	default:
		return usageError{msg: fmt.Sprintf("unknown workspace command %q", args[0])}
	}
}

func workspaceList(ctx context.Context, o Options, args []string) error {
	fs := flag.NewFlagSet("workspace list", flag.ContinueOnError)
	bf := o.addBackendFlags(fs)
	pos, err := o.parseLeaf(fs, "", args, 0)
	if err != nil {
		return err
	}
	_ = pos
	b, closer, err := o.backendIn(ctx, bf, "")
	if err != nil {
		return err
	}
	if closer != nil {
		defer closer.Close()
	}
	wss, err := b.ListWorkspaces(ctx)
	if err != nil {
		return err
	}
	for _, ws := range wss {
		fmt.Fprintf(o.Stdout, "%s\t%s\tissues=%d discussions=%d\n", ws.Slug, ws.Name, ws.IssueCount, ws.DiscussionCount)
	}
	return nil
}

func workspaceCreate(ctx context.Context, o Options, args []string) error {
	fs := flag.NewFlagSet("workspace create", flag.ContinueOnError)
	bf := o.addBackendFlags(fs)
	name := fs.String("name", "", "display name (defaults to the slug)")
	desc := fs.String("description", "", "description")
	pos, err := o.parseLeaf(fs, "SLUG", args, 1)
	if err != nil {
		return err
	}
	if len(pos) < 1 {
		return usageError{msg: "missing workspace slug"}
	}
	input := store.WorkspaceInput{
		Slug:        strings.TrimSpace(pos[0]),
		Name:        strings.TrimSpace(*name),
		Description: strings.TrimSpace(*desc),
	}
	if fe := store.ValidateWorkspaceInput(input); fe.Any() {
		for f, msg := range func() map[string]string {
			m := map[string]string{}
			for k, v := range fe {
				m[k] = v.Message
			}
			return m
		}() {
			fmt.Fprintf(o.Stderr, "%s: %s\n", f, msg)
		}
		return usageError{msg: "invalid workspace input"}
	}
	b, closer, err := o.backendIn(ctx, bf, "")
	if err != nil {
		return err
	}
	if closer != nil {
		defer closer.Close()
	}
	if _, err := b.CreateWorkspace(ctx, input); err != nil {
		return err
	}
	fmt.Fprintf(o.Stdout, "workspace %q created\n", input.Slug)
	return nil
}

func workspaceDelete(ctx context.Context, o Options, args []string) error {
	fs := flag.NewFlagSet("workspace delete", flag.ContinueOnError)
	bf := o.addBackendFlags(fs)
	pos, err := o.parseLeaf(fs, "SLUG", args, 1)
	if err != nil {
		return err
	}
	if len(pos) < 1 {
		return usageError{msg: "missing workspace slug"}
	}
	slug := pos[0]
	b, closer, err := o.backendIn(ctx, bf, "")
	if err != nil {
		return err
	}
	if closer != nil {
		defer closer.Close()
	}
	if err := b.DeleteWorkspace(ctx, slug); err != nil {
		return err
	}
	fmt.Fprintf(o.Stdout, "workspace %q deleted\n", slug)
	return nil
}
