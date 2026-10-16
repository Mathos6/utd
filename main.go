// Command utd (Use Then Delete) runs a program that is not installed on the
// system by fetching it, and its dependencies, from the distribution package
// manager, extracting them into a temporary directory and running the program
// from there. The temporary files are removed on reboot, or immediately after
// a successful run when --delete=now is used.
package main

import (
	"fmt"
	"os"

	"github.com/mathieu/utd/cmd"
)

// version is injected at build time with:
//
//	go build -ldflags "-X main.version=v1.2.3"
var version = "dev"

func main() {
	cmd.Version = version

	if err := cmd.NewRootCmd().Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "utd: error: %v\n", err)
		os.Exit(1)
	}
}
