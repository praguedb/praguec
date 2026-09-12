# Build loop

How an autonomous session takes a `protopc` build ticket from claimed to closed.

Planning tickets are a different job — see `issue-tracker.md` for the tracker
operations and `triage-labels.md` for which tickets an agent may rule on at all.

## What a build ticket is

An issue labelled `wayfinder:task`, a sub-issue of map #1, titled `M<n>: ...`.
It names the spec sections in scope, lists its done-when, and states what is out
of scope. The milestones are strictly ordered and each one is blocked by the
last, so the frontier query returns exactly one:

```sh
repo=nanov/protobuf-to-messagepack-sharp
for n in $(gh issue list --repo "$repo" --label wayfinder:task --state open \
             --json number --jq '.[].number' | sort -n); do
  open_blockers=$(gh api "repos/$repo/issues/$n/dependencies/blocked_by" \
                    --jq '[.[] | select(.state == "open")] | length')
  if [ "$open_blockers" = "0" ]; then echo "$n"; fi
done
```

If it returns more than one, the dependencies were not wired; say so on the map
rather than picking. If it returns none, every milestone is done or claimed —
which is a thing to report, not a thing to work around by inventing a ticket.

**The ticket's scope is the deliverable.** A build ticket that reads a construct
the next milestone owns has not gone faster; it has moved the boundary, and the
boundary is what makes the next ticket claimable by someone who was not here.

## The loop

1. **Claim** — `gh issue edit <n> --add-assignee @me`, the session's first write.
2. **Read what the ticket names, and stop.** `docs/spec/protop.md` is an index,
   not the spec: its table names one file per section and says which task needs
   which. The whole spec is several times the size of any one job in it.
3. **Write the code and its tests together.** Tests are not a follow-up commit —
   M1 shipped as two commits and the second one was the whole of the milestone's
   remaining risk.
4. **Check** — all three, all clean:
   ```
   gofmt -l ./internal ./cmd    # prints nothing
   go vet ./...
   go test ./...
   ```
5. **Commit**, ending the message with:
   ```
   Closes #<n>

   Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
   ```
6. **Close** — comment the outcome on the ticket (what was built, what was
   pinned, what is deliberately still out), close it, and append one line to map
   #1's *Decisions so far*. The line is for someone who will never read the
   ticket: say what the milestone decided, not what it did.

Writing the next milestone's ticket is part of closing, when the ticket you
closed names one. Three writes make it claimable, and all three are easy to
forget:

```sh
repo=nanov/protobuf-to-messagepack-sharp
gh issue create --repo "$repo" --label wayfinder:task --title "M<n>: ..." --body ...
id=$(gh api "repos/$repo/issues/<new>" --jq .id)
prev_id=$(gh api "repos/$repo/issues/<prev>" --jq .id)
gh api --method POST "repos/$repo/issues/1/sub_issues"     -F sub_issue_id="$id"
gh api --method POST "repos/$repo/issues/<new>/dependencies/blocked_by" -F issue_id="$prev_id"
```

The dependency takes the blocker's **database id**, not its number.

## Conventions the frontend already committed to

Breaking one of these is a regression even when the tests pass:

- **No keyword token kinds.** Every word lexes as `KindIdent` and the parser
  decides by position — `key key = 1 (event_id, bet_type_id);` and `alias
  bet_type_id = id;` are unparseable otherwise. Token kinds are prefixed `Kind*`
  because the AST owns names like `Ident` in the same package.
- **The lexer is already complete.** It covers punctuation the parser cannot yet
  read, so growing the language is a parser change alone. Adding a token kind
  should be rare and suspicious.
- **Diagnostics are collected, never thrown.** Codes live in
  `internal/diag/codes.go`, grouped 0001 lexical / 0100 file structure / 0200
  declarations / 0300 fields / 0900 not-yet. A shipped code keeps its number
  forever; a 0900 code is *deleted* when its milestone lands, never redefined.
- **A construct the spec defines and the compiler cannot yet read is
  `NotYetImplemented`, not a syntax error.** The difference is "protopc is
  behind" versus "you wrote this wrong".
- **Partial trees.** The parser appends what it managed to read and returns;
  `bag.HasErrors()` is what gates everything downstream. Any stage added after
  the parser inherits that contract and must check it.
- **Recovery** skips to the next `;` or balanced `}`, except for a missing
  trailing `;`, which is reported in place — skipping there would swallow the
  member that follows.

## Test conventions

- **Fixtures are lifted verbatim out of the spec**, and a second test asserts the
  fixture still appears inside the spec file via `strings.Contains`. The pair is
  the point: the first test alone only proves the parser reads a file that once
  resembled the spec. See `TestSpecSubsetIsVerbatim`.
- **Assert trees with `dump`** — one line per node, compared against a literal,
  so a whole parse is one assertion and a mismatch reads as a diff.
- **Assert diagnostics as a code sequence**, not a count: codes are protopc's
  interface. Position via `f.Position` or a whole `diag.Render` snippet — never
  by a hand-computed byte offset.
- **Pin the behaviour, not the implementation.** If a test would still pass with
  the behaviour it names broken, it is not a test. Mutating the parser and
  watching the right test fail takes a minute and is worth it.

## Where an autonomous session stops

- **`wayfinder:grilling` is HITL.** An agent may gather evidence and comment it
  on the ticket. It may not rule, and may not close. Same for `wayfinder:map`.
- **The closed scalar set is #32 and it is unresolved.** `scalarTypes` in
  `internal/syntax/parser.go` is hard-coded from a census of the spec. Do not
  widen it to make a test pass.
- **Anything the spec marks sketched or TBD** — `versioning.md` — is not
  implementable yet, whatever a ticket's wording seems to allow.
