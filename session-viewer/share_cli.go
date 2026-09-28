package main

// `bonnie share` — grant and revoke from the command line, as root.
//
// PLAN.md §4 M4.2 requires an out-of-band route to sharing for two reasons.
//
// The gate authenticates as Bob and only Bob (§5 uses one test identity), but
// the operation under test is Alice granting. Driving the grant with raw
// setfacl from a fixture would test the shell script rather than the code that
// actually runs in production, so the gate calls this, which calls exactly the
// same grantShare/revokeShare as the HTTP handlers.
//
// It is also the A5 check: "ssh in as Bob, run the binary by hand — identical
// result." Sharing must not be something that only exists inside the web UI.
//
// Root-only, like provision, because it acts on two users' trees: it spawns a
// child as the owner to write the ACL and another as the sharee to write the
// symlink. It never writes to either tree as root (PLAN.md §1).

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"session-viewer/internal/principal"
)

func runShare(argv []string) int {
	fs := flag.NewFlagSet("share", flag.ContinueOnError)
	var (
		owner   = fs.String("owner", "", "unix username granting access (required)")
		sharee  = fs.String("sharee", "", "unix username receiving access (required)")
		cwd     = fs.String("cwd", "", "project directory containing the conversation (required)")
		session = fs.String("session", "", "conversation id to share (required)")
		revoke  = fs.Bool("revoke", false, "remove the share instead of creating it")
		list    = fs.Bool("list", false, "list what the owner has shared with the sharee")
	)
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr,
			"usage: bonnie share --owner <user> --sharee <user> --cwd <dir> --session <id> [--revoke]\n"+
				"       bonnie share --owner <user> --sharee <user> --list\n\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(argv); err != nil {
		return 2
	}
	if *owner == "" || *sharee == "" {
		fmt.Fprintln(os.Stderr, "share: --owner and --sharee are required")
		return 2
	}
	if !*list && (*cwd == "" || *session == "") {
		fmt.Fprintln(os.Stderr, "share: --cwd and --session are required unless --list")
		return 2
	}
	if os.Geteuid() != 0 {
		// Same reasoning as provision: fail up front rather than part-way
		// through, which would leave a grant applied and no symlink.
		fmt.Fprintln(os.Stderr, "share: must run as root (it acts as two different users)")
		return 2
	}

	ownerPrin, err := principal.Lookup(*owner)
	if err != nil {
		fmt.Fprintf(os.Stderr, "share: owner %s: %v\n", *owner, err)
		return 1
	}
	shareePrin, err := principal.Lookup(*sharee)
	if err != nil {
		fmt.Fprintf(os.Stderr, "share: sharee %s: %v\n", *sharee, err)
		return 1
	}

	if *list {
		entries, err := currentACL(ownerPrin.Home, shareePrin.Username)
		if err != nil {
			fmt.Fprintf(os.Stderr, "share: %v\n", err)
			return 1
		}
		n := 0
		for _, e := range entries {
			// Corridor bits are scaffolding; printing them would imply the
			// owner shared directories they never chose to share.
			if !e.isGrant() {
				continue
			}
			fmt.Println(e.Path)
			n++
		}
		if n == 0 {
			fmt.Fprintf(os.Stderr, "(no shares from %s to %s)\n", *owner, *sharee)
		}
		return 0
	}

	// A conversation, not a directory. The CLI takes the same (cwd, id) pair
	// as the HTTP route and derives the path the same way, so the two cannot
	// disagree about what a share is — which matters because the gate drives
	// Alice's half through here and Bob's half through the API.
	absCWD, err := filepath.Abs(*cwd)
	if err != nil {
		fmt.Fprintf(os.Stderr, "share: %v\n", err)
		return 1
	}
	abs := sessionDir(absCWD, *session)
	// Resolve symlinks before acting, for the same reason the HTTP handler
	// does: a link could otherwise point the ACL outside the owner's tree
	// while the textual path looks fine.
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		abs = resolved
	}

	if *revoke {
		if err := revokeShare(ownerPrin, shareePrin, abs); err != nil {
			fmt.Fprintf(os.Stderr, "share: revoke: %v\n", err)
			return 1
		}
		fmt.Printf("revoked %s from %s\n", abs, shareePrin.Username)
		return 0
	}

	if err := grantShare(ownerPrin, shareePrin, abs); err != nil {
		fmt.Fprintf(os.Stderr, "share: grant: %v\n", err)
		return 1
	}
	fmt.Printf("shared %s with %s at %s\n",
		abs, shareePrin.Username, linkNameFor(shareePrin.Home, ownerPrin.Username, abs))
	return 0
}
