// Package load finds the files a compile is made of.
//
// The checker is handed a check.FileSet -- a map from import string to parsed
// file -- and says so: "who found them ... is the driver's question". This
// package is that driver's half. It opens files, parses them, follows their
// imports, and hands the checker a set.
//
// What it resolves against is a list of local include roots and nothing else.
// Nothing here knows about git, tags, versions, caches, manifests or lock
// files; that is nanov/protobuf-to-messagepack-sharp#31 and it is open, and a
// loader that reached for a network would be answering it by accident. An
// include root is a directory on this machine, an import string is a path
// relative to one, and that is the whole model.
//
// Two rules the loader is deliberately not allowed to invent:
//
//   - Which root wins when two of them provide one import string is #48, and
//     it is unresolved. The loader searches in the order the user gave, so it
//     always has a candidate -- but it reports the ambiguity rather than
//     ranking the roots, because a build whose meaning turns on an unsettled
//     rule should say so out loud.
//   - A cycle in the import graph is harmless to the checker, whose scope is
//     not transitive, but it is not harmless to a resolver that walks it. The
//     walk here is memoised on the set itself: a file already in the set is
//     never opened twice, so `a` importing `b` importing `a` terminates and
//     produces both. That is the loader half of #48 and it holds however #48
//     rules on scope.
package load

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/nanov/protobuf-to-messagepack-sharp/internal/check"
	"github.com/nanov/protobuf-to-messagepack-sharp/internal/diag"
	"github.com/nanov/protobuf-to-messagepack-sharp/internal/source"
	"github.com/nanov/protobuf-to-messagepack-sharp/internal/syntax"
)

// Result is a loaded program.
type Result struct {
	// Set is every file the compile touches, keyed by the import string that
	// names it -- the inputs and everything reachable from them.
	Set check.FileSet

	// Targets are the keys of the files named on the command line, in the
	// order they were named. A file that is only in the set because something
	// imported it is not a target: protopc compiles what it was asked for and
	// reads the rest to make sense of it, which is protoc's split and the one
	// that keeps `generate one file` from writing the whole repository.
	Targets []string
}

// Load reads inputs and everything they import, parsing each into bag.
//
// The two kinds of failure are kept apart, because the exit codes are. A
// returned error is the command being wrong -- an input that is not there, a
// root that is not a directory -- and the caller exits 2. A diagnostic in bag
// is the *schema* being wrong, including an import that names no file under
// any root, and the caller exits 1. The user typed the input path; they did not
// type the import path, they wrote it in a file that can be pointed at.
func Load(inputs, roots []string, bag *diag.Bag) (*Result, error) {
	rs, err := resolveRoots(roots, inputs)
	if err != nil {
		return nil, err
	}
	l := &loader{
		roots:     rs,
		bag:       bag,
		set:       make(check.FileSet, len(inputs)),
		keyByPath: make(map[string]string, len(inputs)),
		nameByKey: make(map[string]string, len(inputs)),
	}

	res := &Result{Set: l.set}
	for _, in := range inputs {
		key, err := l.load(in, l.key(in))
		if err != nil {
			return nil, err
		}
		if !slices.Contains(res.Targets, key) {
			res.Targets = append(res.Targets, key)
		}
	}

	// Breadth-first from the inputs, in input order and then in each file's
	// own source order, so that the diagnostics a load produces come out in
	// the same order on every run. The queue grows while it is walked, which
	// is the transitive part; l.set is what stops it.
	for i := 0; i < len(l.queue); i++ {
		if err := l.follow(l.queue[i]); err != nil {
			return nil, err
		}
	}
	return res, nil
}

// unit is one loaded file: what the checker will be given, plus the path it was
// read from, which the set does not carry.
type unit struct {
	key  string
	path string
	src  *source.File
	ast  *syntax.File
}

type loader struct {
	roots []string
	bag   *diag.Bag

	set   check.FileSet
	queue []unit

	// keyByPath is the same file reached twice -- once as an input and once as
	// somebody's import, say. One file must be one *unit*: reading and parsing
	// it twice would make it two files to the checker, and every declaration in
	// it would be emitted twice under two names. It may be in the set under
	// more than one key, because two import strings may name it; see load.
	keyByPath map[string]string

	// nameByKey is the reverse, for the message when two different files claim
	// one key.
	nameByKey map[string]string
}

// load reads path into the set under key and queues its imports. It returns the
// key the file actually ended up under, which is not key when the same file was
// already loaded under another one.
func (l *loader) load(path, key string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("%s: %w", path, err)
	}
	if had, ok := l.keyByPath[abs]; ok {
		if had != key {
			// One file, two import strings. `import "./b.prague"` and `import
			// "b.prague"` are two keys -- the key is the import string as
			// written, which is check.FileSet's contract and not this
			// package's to change -- and both of them resolved, here, to this
			// file. So both of them have to be in the set: the set is the
			// whole of the checker's index, and a resolved import missing
			// from it is PROTOP0107 raised against a file the loader has
			// already opened and parsed.
			//
			// One unit under two keys, and not a second load: the file is
			// read once and parsed once, so its *parse* diagnostics are raised
			// once. Whether a non-canonical import string should be legal at
			// all is a language question -- protoc rejects it, buf cleans it --
			// and aliasing rules on neither. It says only that the loader
			// answers for everything it resolved.
			//
			// What it does leave is a residue downstream: check.Check walks the
			// set, so a file under two keys is declared twice and its *checker*
			// diagnostics come out twice. Emission is not doubled -- every input
			// is loaded before any import is followed, so a target's key is
			// always the first one and an alias is never a target -- and the
			// residue is a duplicated message, not a duplicated file. Collapsing
			// it belongs wherever the canonicality question is answered: doing it
			// here would mean picking one of the two keys as the real one, which
			// is the ruling this fix exists not to take.
			l.set[key] = l.set[had]
			l.nameByKey[key] = path
		}
		return had, nil
	}
	if other, ok := l.nameByKey[key]; ok {
		return "", fmt.Errorf("%s and %s both resolve to the import path %q; "+
			"an import string names one file, so one of them cannot be imported at all", other, path, key)
	}

	text, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	// The file is named by the path as it was given. source.File says the name
	// is what a diagnostic prints and that it should echo the argument, and an
	// import's argument is the path the loader built to open it.
	src := source.NewFile(path, string(text))

	// Within, and not a bare Parse, because the parser sees one file at a time
	// and the loader is holding several: an unstamped diagnostic renders its
	// snippet against whichever file the printer happened to be given.
	var ast *syntax.File
	l.bag.Within(src, func(bag *diag.Bag) { ast = syntax.Parse(src, bag) })
	u := unit{key: key, path: path, src: src, ast: ast}

	l.set[key] = check.Unit{Source: u.src, File: u.ast}
	l.keyByPath[abs] = key
	l.nameByKey[key] = path
	l.queue = append(l.queue, u)
	return key, nil
}

// follow resolves each of a file's imports and loads what they name.
func (l *loader) follow(u unit) error {
	sink := l.bag.In(u.src)
	for _, im := range u.ast.Imports {
		key := check.ImportPath(im.Path)
		if _, done := l.set[key]; done {
			// Already loaded, so already resolved -- including its ambiguity,
			// which was reported at whichever import site reached it first.
			// Once is the right number: the ambiguity is a fact about the
			// roots, not about this line.
			continue
		}

		found := l.search(key)
		switch len(found) {
		case 0:
			sink.Raise(diag.ImportNotFound, im.Span,
				"no file named %q was found under any include root; searched %s",
				key, quoteList(l.roots))
			continue
		case 1:
		default:
			sink.Raise(diag.ImportAmbiguous, im.Span,
				"%q is provided by more than one include root -- %s -- and which one wins is not settled; "+
					"remove a root, or rename one of the files",
				key, quoteList(found))
		}
		if _, err := l.load(found[0], key); err != nil {
			return err
		}
	}
	return nil
}

// search returns every include root that provides key, in the user's root
// order. It returns paths rather than roots because the caller opens one of
// them and reports the rest.
//
// An include root is a starting point here and not a boundary: filepath.Join
// cleans, so `import "../secret/b.prague"` under `-I inc` resolves to
// inc/../secret/b.prague, loads, keys as written, and the compile is clean.
// Recorded rather than fixed. Whether a root fences the import graph is a
// language question adjacent to #48 and #31 -- protoc rejects a `..` import
// string outright at the syntax level, which is the same unruled question as
// `./b.prague`, and a loader that started refusing paths would be answering it
// -- so the behaviour stands and is written down instead.
func (l *loader) search(key string) []string {
	var found []string
	seen := make(map[string]bool, len(l.roots))
	for _, root := range l.roots {
		path := filepath.Join(root, filepath.FromSlash(key))
		st, err := os.Stat(path)
		if err != nil || !st.Mode().IsRegular() {
			continue
		}
		// Two roots that are two *lexical* spellings of one directory -- "inc",
		// "inc/", and the absolute form of either -- provide one file, not two,
		// and that is not an ambiguity worth a diagnostic.
		//
		// Lexical is the whole of it: this compares filepath.Abs, which does not
		// touch the filesystem, so `-I real -I link` where link is a symlink to
		// real is two paths here and raises a spurious PROTOP0108. Known, and
		// left as it is. EvalSymlinks would resolve it, but it is a stat per
		// root per import that can fail, on a machine where a root may be a
		// network mount, and it would make which root "wins" depend on what the
		// filesystem does rather than on what the user typed -- which is #48's
		// question, not this function's. Narrowed to what the code does, so the
		// gap is visible to whoever rules on it.
		abs, err := filepath.Abs(path)
		if err == nil {
			if seen[abs] {
				continue
			}
			seen[abs] = true
		}
		found = append(found, path)
	}
	return found
}

// key is the import string an input file is known by: its path relative to the
// first include root that contains it.
//
// It matters that this is not simply the path as typed. `protopc generate -I
// schemas schemas/a.prague` and an `import "a.prague"` elsewhere have to name
// the same entry in the set, or the file is loaded twice under two names and
// every declaration in it is emitted twice. An input under no root at all keeps
// its own path as its key, which is a file nothing can import -- true, and not
// an error: a one-file compile with no imports is a legitimate thing to ask for.
func (l *loader) key(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return filepath.ToSlash(filepath.Clean(path))
	}
	for _, root := range l.roots {
		absRoot, err := filepath.Abs(root)
		if err != nil {
			continue
		}
		rel, err := filepath.Rel(absRoot, abs)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue
		}
		return filepath.ToSlash(rel)
	}
	return filepath.ToSlash(filepath.Clean(path))
}

// resolveRoots is the include path: what the user gave with -I, or, when they
// gave nothing, the directory of each input.
//
// The default is the directory of each input rather than the working directory
// because `protopc generate schemas/a.prague` should find the `import
// "b.prague"` sitting next to it, which is what every user of protoc expects.
// The two are not combined: once -I is given it is the whole list, so that
// adding a root cannot silently shadow -- or be shadowed by -- a directory the
// user never named.
func resolveRoots(roots, inputs []string) ([]string, error) {
	if len(roots) > 0 {
		out := make([]string, 0, len(roots))
		for _, r := range roots {
			st, err := os.Stat(r)
			if err != nil {
				return nil, fmt.Errorf("include root %s: %w", r, err)
			}
			if !st.IsDir() {
				return nil, fmt.Errorf("include root %s: not a directory", r)
			}
			if c := filepath.Clean(r); !slices.Contains(out, c) {
				out = append(out, c)
			}
		}
		return out, nil
	}

	out := make([]string, 0, len(inputs))
	for _, in := range inputs {
		if d := filepath.Dir(in); !slices.Contains(out, d) {
			out = append(out, d)
		}
	}
	if len(out) == 0 {
		out = append(out, ".")
	}
	return out, nil
}

// quoteList renders a list of paths for a message, which is the difference
// between "I could not find it" and "I could not find it, and here is where I
// looked".
func quoteList(paths []string) string {
	out := make([]string, len(paths))
	for i, p := range paths {
		out[i] = fmt.Sprintf("%q", p)
	}
	return strings.Join(out, ", ")
}
