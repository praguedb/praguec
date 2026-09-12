# 7. Tag ownership

`reserved` is the mechanism that keeps cross-repo skew safe. Only the shared repo can retire a tag, and a removal without a reservation breaks every view at compile time — which is the correct outcome, and the reason it only works with a single owner.

This is the value of the whole design. Today six field lists drift independently: `EventId` becomes `Id`, `Key` and `Id` disagree across pairs, and one consumer carries a hand-written comment asking nobody to renumber. With one table and N views pinned to its tags, that drift stops being a matter of discipline and becomes a compile error in the repo that drifted.
