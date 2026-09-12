// Command protopc compiles protop schemas.
//
// The compiler is split the way LLVM is: the frontend under internal/syntax
// parses source into an AST, a later checking stage resolves it into one
// schema model, and backends see only that model -- never the source, never
// the AST. Adding a target language is adding a backend, and nothing in the
// frontend learns about it.
package main

import (
	"fmt"
	"os"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "protopc:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	// The CLI is buf-shaped in the end -- build, lint, breaking, format, mod.
	// None of it exists yet; the frontend comes first.
	return fmt.Errorf("not implemented")
}
