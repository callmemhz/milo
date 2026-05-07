package main

import (
	"fmt"
	"os"

	"github.com/callmemhz/milo-apps-kit/internal/cli"
)

func main() {
	if err := cli.RootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
