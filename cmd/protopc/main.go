// Command protopc compiles protop schemas.
//
// The compiler is split the way LLVM is: the frontend under internal/syntax
// parses source into an AST, a later checking stage resolves it into one
// schema model, and backends see only that model -- never the source, never
// the AST. Adding a target language is adding a backend, and nothing in the
// frontend learns about it.
//
// This file is the driver: the stage that opens files, decides where output
// goes, and turns a bag of diagnostics into an exit code. Nothing about the
// .NET build is here. Whether protopc runs during an MSBuild build or before
// one is nanov/protobuf-to-messagepack-sharp#44, unresolved, and a .props or a
// .targets written before that ruling would be a guess at half of it.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/nanov/protobuf-to-messagepack-sharp/internal/check"
	"github.com/nanov/protobuf-to-messagepack-sharp/internal/csharp"
	"github.com/nanov/protobuf-to-messagepack-sharp/internal/diag"
	"github.com/nanov/protobuf-to-messagepack-sharp/internal/load"
	"github.com/nanov/protobuf-to-messagepack-sharp/internal/source"
)

// The exit codes, which are an interface in the same way the diagnostic codes
// are: a build script branches on them.
//
// The distinction that matters is the last two. A diagnostic is not a crash and
// a crash is not a diagnostic: exit 1 means protopc ran, read the schema, and
// has something to say about it, and a CI job should show those lines to the
// author. Exit 2 means protopc never got far enough to have an opinion -- a
// file that is not there, a flag it does not have -- and the fix is to the
// command, not to the schema.
const (
	exitOK    = 0
	exitDiags = 1
	exitUsage = 2
)

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

const usage = `protopc compiles protop schemas to C#.

Usage:
        protopc <command> [flags] <file.protop>...

Commands:
        generate   parse, check, and write the C# for the named files
        check      parse and check the named files, writing nothing

Flags:
        -I dir             include root to resolve imports against; repeatable,
                           searched in the order given. Defaults to the
                           directory of each named file.
        -o dir             where generate writes (default ".")
        --error-format f   gcc (default) or msvs
        --manifest path    generate: write the list of emitted files here

Exit codes:
        0  no errors
        1  the schema has errors
        2  the command or the filesystem does
`

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return exitUsage
	}
	switch args[0] {
	case "generate":
		return compile(args[0], args[1:], true, stdout, stderr)
	case "check":
		return compile(args[0], args[1:], false, stdout, stderr)
	case "-h", "--help", "help":
		fmt.Fprint(stdout, usage)
		return exitOK
	default:
		fmt.Fprintf(stderr, "protopc: unknown command %q\n\n", args[0])
		fmt.Fprint(stderr, usage)
		return exitUsage
	}
}

// repeated is a flag that may be given more than once, keeping the order. The
// order is the whole of what -I means, so it cannot be a set.
type repeated []string

func (r *repeated) String() string     { return strings.Join(*r, ", ") }
func (r *repeated) Set(v string) error { *r = append(*r, v); return nil }

// compile runs the pipeline for both commands. `check` is `generate` without
// the writing, and they are one function because anything else would let the
// two drift into checking different things -- which is the one property a
// `check` command has to have.
func compile(name string, args []string, write bool, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("protopc "+name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() { fmt.Fprint(stderr, usage) }

	var roots repeated
	fs.Var(&roots, "I", "include root, repeatable")
	format := fs.String("error-format", "gcc", "diagnostic format: gcc or msvs")

	// `check` writes nothing, so it does not have -o or --manifest at all.
	// Accepting a flag and then ignoring it is worse than rejecting it: the
	// user who writes `protopc check -o out` believes something was written.
	outDir, manifestPath := new(string), new(string)
	if write {
		outDir = fs.String("o", ".", "output directory")
		manifestPath = fs.String("manifest", "", "write the list of emitted files here")
	}

	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	errFormat, ok := diag.ParseFormat(*format)
	if !ok {
		fmt.Fprintf(stderr, "protopc: unknown --error-format %q; protopc has gcc and msvs\n", *format)
		return exitUsage
	}
	inputs := fs.Args()
	if len(inputs) == 0 {
		fmt.Fprintf(stderr, "protopc %s: no input files\n\n", name)
		fmt.Fprint(stderr, usage)
		return exitUsage
	}

	var bag diag.Bag
	res, err := load.Load(inputs, roots, &bag)
	if err != nil {
		fmt.Fprintln(stderr, "protopc:", err)
		return exitUsage
	}

	schema := check.Check(res.Set, &bag)
	report(stderr, errFormat, res, &bag)
	if bag.HasErrors() {
		return exitDiags
	}
	// A warning is reported and does not fail the build. There is no -Werror
	// here; the lint ladder is #16 and open, and inventing half of it in the
	// driver would put the ladder's first rung somewhere #16 cannot move it.
	if !write {
		return exitOK
	}

	written, err := emit(*outDir, schema, res.Targets)
	if err != nil {
		fmt.Fprintln(stderr, "protopc:", err)
		return exitUsage
	}
	if *manifestPath != "" {
		if err := writeManifest(*manifestPath, written); err != nil {
			fmt.Fprintln(stderr, "protopc:", err)
			return exitUsage
		}
	}
	for _, p := range written {
		fmt.Fprintln(stdout, p)
	}
	return exitOK
}

// report prints every diagnostic in the bag to stderr.
//
// The fallback file is only ever reached by a diagnostic that names no file of
// its own, and after the loader there are none: it stamps each parse. It is
// passed because Render takes one, and it is taken from the set rather than
// invented so that it is a real file if it is ever used.
func report(w io.Writer, format diag.Format, res *load.Result, bag *diag.Bag) {
	if bag.Len() == 0 {
		return
	}
	var fallback *source.File
	for _, key := range res.Targets {
		if u, ok := res.Set[key]; ok {
			fallback = u.Source
			break
		}
	}
	if fallback == nil {
		return
	}
	fmt.Fprintln(w, diag.RenderAllAs(format, fallback, bag))
}

// emit writes the C# for the target files and returns the paths it wrote, in
// the order it wrote them.
//
// Only the targets. A file that is in the set because something imported it was
// read to make sense of the ones that were named, and writing it would mean
// `protopc generate one.protop` rewriting files the user did not mention.
//
// The file name is <TypeName>.cs, flat under -o, and it is provisional. Whether
// a declaration gets its own file, what a generated file's banner says, and who
// owns the output directory are #25 Q1-Q4 and human-in-the-loop; what is *not*
// open is the constraint underneath them, which this milestone does hold to:
// the name must not end .g.cs or .generated.cs, the path must not run through
// obj/, bin/ or Generated/, and the first 500 bytes must not contain
// "auto-generated". See TestEmittedFilesAreVisibleToTheDownstreamGenerator.
func emit(dir string, schema *check.Schema, targets []string) ([]string, error) {
	want := make(map[string]bool, len(targets))
	for _, t := range targets {
		want[t] = true
	}

	var written []string
	by := make(map[string]string)
	for _, u := range csharp.Emit(schema) {
		if !want[u.Decl.File.Path] {
			continue
		}
		path := filepath.Join(dir, u.TypeName+".cs")
		// Flat output makes two declarations of one name in two namespaces one
		// file. Which of the two survives is not a thing to decide quietly:
		// the layout that would avoid the collision is #25's to choose, and
		// until it does, the collision is an error and not a coin toss.
		if prev, clash := by[path]; clash {
			return nil, fmt.Errorf("%s and %s would both be written to %s; "+
				"the output layout is nanov/protobuf-to-messagepack-sharp#25 and does not yet separate them",
				prev, u.Namespace+"."+u.TypeName, path)
		}
		by[path] = u.Namespace + "." + u.TypeName

		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
		if err := os.WriteFile(path, []byte(u.Source), 0o644); err != nil {
			return nil, err
		}
		written = append(written, path)
	}
	if len(written) == 0 {
		return nil, errors.New("nothing to write: the named files declare nothing")
	}
	sort.Strings(written)
	return written, nil
}
