# 5. Views

A view is a field subset of a table or record, at that source's tags. Tag, type and presence come from the source; the view line carries the name and an optional rename.

**A view may add untagged members. A view may never add a tag.** Not policy — structure. The producer writes the table, so a tag the table does not declare is never written by anyone and would deserialize to nil forever, while squatting a slot the table might later claim. Only the table extends the tag space. This is the same rule as *no tag, no wire slot*, one level down.

Protobuf's answer to the same pressure is extension ranges: an owner reserves `extensions 100 to 199;` and a foreign file claims tags inside it. That works because a *producer* can populate an extension. In a single-producer topic nobody writes outside the table, so the mechanism would delegate slots that stay empty. The asymmetry of the pipeline is what rules it out.

**A view inherits the topic.** It is a property of the table, so restating it on the view was the last piece of view-as-second-copy — and inheriting it means a view cannot point at the wrong topic.

**A view may narrow a member to another view.** `SlimOdd odds;` takes tag 26 and requires `SlimOdd` to be a view of `Odd`. A leading type asks for the narrowing; a bare name takes the source's type unchanged. A view of a view stays pinned to the original tags, so `SlimOdd` cannot quietly renumber and desync tag 26.

**A view may narrow and rename in one member.** `SlimOdd odds as o;` is still tag 26: the leading type says what the member holds, `as o` says what the consumer calls it, and the tag is the source's as it is on every other view line — neither half is about the tag. The two rules are written apart above only because they are separately motivated; refusing them together would deny a consumer a name for the subset it just took, and would buy nothing back, since the one thing a view must not invent is the number that neither half touches.

**A view's source is a table or a record.** `SlimOdd of Odd` narrows a record, because a view is about tags and members rather than identity.

### Computed members

Members that carry no tag and are not aliases — `StatusActual`, `IsAvailableForClient`, `IsDisabled` in the corpus — live in the hand-written partial class, not in protop. They are relocated, not deleted: a member and its attributes travel together, so moving one to the partial takes its annotations with it.

Two classes of computed member remain in the language, neither of them an expression:

- **Composite identity** — `key key = 1 (event_id, bet_type_id);`. Borrowed from Cap'n Proto's `group`, a name over fields that keep their own ordinals. Cap'n Proto's group consumes no ordinal; this one does, because the corpus wire format already says so — the tuple is `[Key(0)]` and duplicates what tags 2 and 3 carry. Writing the tag makes the duplication visible and the collision checkable.
- **Alias** — `alias bet_type_id = id;`. Get and set passthrough, no wire slot. It exists so a table's API matches the view that renames the field.
