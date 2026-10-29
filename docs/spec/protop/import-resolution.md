# Where an import resolves from

Part of §2 of the [protop specification](../protop.md).

`import "prague.prague";` names a file, not a location — and the two files in
this document live in two different repositories. Resolution *is* the
distribution model, so it has to be stated rather than assumed.

**Direction: git, by tag, the way Go does it.** A dependency is a git repository
pinned to an immutable tag, fetched and cached by the toolchain. Not a package
registry, not a vendored copy, not a submodule. The shared schema is already a
repo owned by one team; a tag on it is already how that team says *this is
published*. Nothing new has to be stood up for a consumer to depend on a version
of it.

This is the piece the strict-Protobuf spelling could never have had cheaply.
Under that model the import path problem was `protoc`'s to solve and protop was
a guest in it — which is exactly what the retired options-distribution question
was about. Owning the language means owning resolution, and owning resolution
means it can be the one thing Protobuf never shipped.

**Decided:** git, immutable tag, toolchain-fetched, lock file. **Open:** whether
the module path lives in the import line (`import "github.com/org/schemas@v1/prague.prague"`)
or in a manifest beside it; what the manifest is called; how the cache is keyed;
whether the tag is verified by content hash the way `go.sum` does it.

**One name collision to settle before it ships.** A git tag is a third thing in
this project called a version, beside `revision 1;` (the dialect) and the
per-table major/minor of the versioning sketch below. The sketch already flags
two as one too many.

**And one connection.** The versioning sketch below proposes `protop.lock`
holding `{major, minor, wire_hash, source_hash}` per table. Dependency pinning
wants a lock file too. Whether those are one file or two is open, but they are
the same kind of fact — *what I last published* and *what I last resolved* —
and deciding them together is cheaper than reconciling them later.
