package main

import (
	"fmt"
	"os"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) == 0 {
		fmt.Println("Prolane\nProve every software change before it reaches production.")
		return 0
	}
	if len(args) == 1 && (args[0] == "-h" || args[0] == "--help") {
		fmt.Println("Usage: prolane [command]\n\nCommands:\n  record    Forward HTTP traffic and record exchange metadata\n\nUse prolane record --help for recorder options.")
		return 0
	}
	if args[0] == "record" {
		return runRecord(args[1:])
	}

	fmt.Fprintln(os.Stderr, "prolane: unknown command or arguments; use --help")
	return 2
}
