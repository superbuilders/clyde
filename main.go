package main

import (
	"fmt"
	"os"

	"github.com/superbuilders/clyde/cli"
)

// version is injected at link time with -X main.version=<ref>. It defaults to
// "dev" so a plain `go build` still produces a binary that answers --version.
//
// This is load-bearing for Bonnie, not cosmetic: a Bonnie release pins clyde by
// absolute path under /opt/bonnie/versions/<sha>/clyde, and `bonnie doctor`
// verifies the pin by running `clyde --version` and checking that the output
// names that version. Without a --version flag, the version-named directory could
// hold any build at all and the pin would be a comment rather than a guarantee.
var version = "dev"

func main() {
	// Handled here rather than in cli.ParseFlagsExt because it must not be
	// possible for --version to reach the agent loop: anything that starts the
	// agent costs an API call, and `doctor` must be free and offline.
	for _, arg := range os.Args[1:] {
		if arg == "--version" || arg == "-V" {
			fmt.Println("clyde " + version)
			return
		}
	}
	cli.Run()
}
