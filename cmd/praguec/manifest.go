package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
)

// Manifest is the ledger of what one `praguec generate` wrote.
//
// It is specified here and wired to nothing. The reason it can be specified
// before praguedb/praguec#44 rules on when praguec runs is
// that the answer does not change who owns stale output. MSBuild's
// IncrementalClean only reaches $(OutDir) and $(IntermediateOutputPath), and
// Prague.Feed.Cache's generator skips anything under obj/ or bin/ -- so
// praguec's output has to live outside both, which puts it outside everything
// MSBuild would clean. Under either answer to #44, praguec is the only thing
// that can delete a file praguec wrote and no longer writes.
//
// The format, so that the tool that eventually reads it has something to read:
//
//		{
//		  "version": 1,
//		  "files": ["Odd.cs", "SportGame.cs"]
//		}
//
//	  - version is 1 and is bumped when the shape changes, so a reader can
//	    refuse a manifest it does not understand rather than mis-delete.
//	  - files are slash-separated and relative to the directory holding the
//	    manifest, sorted. Relative because a manifest checked in beside its
//	    output must mean the same thing on another machine; sorted because a
//	    file that reorders on every run is a diff on every run.
//
// What this milestone does not do is act on it. Deleting a file because a
// previous manifest listed it is a destructive operation whose blast radius
// depends on where the output directory is, and that is #25's question.
type Manifest struct {
	Version int      `json:"version"`
	Files   []string `json:"files"`
}

// manifestVersion is bumped when the shape above changes.
const manifestVersion = 1

// writeManifest records paths in a manifest at path.
//
// All three of the properties the comment above claims are made here, by the
// writer, rather than assumed of the caller: relative to the manifest's own
// directory, slash-separated, and sorted. Sorted last and not by the caller,
// because the sort has to be over the names as they are written down -- a
// caller that sorted absolute paths would produce a manifest that is sorted by
// something the reader cannot see.
func writeManifest(path string, paths []string) error {
	dir := filepath.Dir(path)
	m := Manifest{Version: manifestVersion, Files: make([]string, 0, len(paths))}
	for _, p := range paths {
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			rel = p
		}
		m.Files = append(m.Files, filepath.ToSlash(rel))
	}
	sort.Strings(m.Files)

	text, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	if dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return os.WriteFile(path, []byte(string(text)+"\n"), 0o644)
}
