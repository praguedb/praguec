package rust

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/praguedb/praguec/internal/check"
	"github.com/praguedb/praguec/internal/diag"
	"github.com/praguedb/praguec/internal/source"
	"github.com/praguedb/praguec/internal/syntax"
)

// The Rust backend's oracle is not here. Whether the emitted decoders compile
// and read what the C# backend's types write is held by
// github.com/praguedb/prague.rs, which decodes fixtures written by Prague
// (.NET)'s own serializer from one schema compiled both ways. These tests hold
// the shape: what lands at which index, and which annotations cross.

func schemaOf(t *testing.T, files map[string]string) *check.Schema {
	t.Helper()
	var bag diag.Bag
	set := make(check.FileSet, len(files))
	var any *source.File
	for path, src := range files {
		f := source.NewFile(path, src)
		any = f
		set[path] = check.Unit{Source: f, File: syntax.Parse(f, &bag)}
	}
	schema := check.Check(set, &bag)
	if bag.Len() != 0 {
		t.Fatalf("fixtures do not check cleanly:\n%s", diag.RenderAll(any, &bag))
	}
	return schema
}

func emitOne(t *testing.T, src string) string {
	t.Helper()
	units, err := Emit(schemaOf(t, map[string]string{"a.prague": src}), func(*check.File) bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	if len(units) != 1 || units[0].Name != "a.rs" {
		t.Fatalf("units: %+v", units)
	}
	return units[0].Source
}

func assertHas(t *testing.T, src string, wants ...string) {
	t.Helper()
	for _, w := range wants {
		if !strings.Contains(src, w) {
			t.Errorf("emission lacks %q\n---\n%s", w, src)
		}
	}
}

// section is the text from the first line containing start to the next line
// that is exactly "}".
func section(src, start string) string {
	s := src[strings.Index(src, start):]
	return s[:strings.Index(s, "\n}\n")]
}

const shop = `revision 1;
namespace Shop;

enum Status {
  UNKNOWN = 0;
  NOT_STARTED = 1;
}

record Price {
  decimal(4) amount = 1;
}

table Offer
  [cs: DataCacheTopic("x")] [rust: derive(serde::Serialize)]
{
  reserved 4;
  key key = 1 (product_id, offer_id);
  int64 product_id = 2 [rust: serde(rename = "productId")] [cs: DataCacheIndex];
  int64 id = 3;
  alias offer_id = id;
  Price? price = 5;
  list<Price?> history = 6;
  Status? status = 7;
  double? score = 8;
  string type = 9;
}

view SlimOffer of Offer {
  key;
  product_id;
  id;
}
`

func TestATagIsTheIndexPlusOne(t *testing.T) {
	src := emitOne(t, shop)
	assertHas(t, src,
		`1 => x.product_id = Decode::decode(r).map_err(|e| e.at("Offer", i))?,`,
		`4 => x.price = Decode::decode(r).map_err(|e| e.at("Offer", i))?,`,
		`_ => r.skip().map_err(|e| e.at("Offer", i))?,`,
	)
	// Slot 0 is the composite key's duplicated tuple and slot 3 is reserved:
	// neither is decoded into anything.
	decoder := section(src, "impl Decode for Offer {")
	for _, slot := range []string{"0 =>", "3 =>"} {
		if strings.Contains(decoder, slot) {
			t.Errorf("Offer decodes %s", slot)
		}
	}
}

func TestPresenceIsAnOptionAndAListIsAVec(t *testing.T) {
	src := emitOne(t, shop)
	assertHas(t, src,
		"pub price: Option<Price>,",
		"pub history: Vec<Option<Price>>,",
		"pub status: Option<Status>,",
		"pub amount: wire::Decimal,",
		"use prague::wire::{self, Decode, Reader};",
	)
}

func TestAKeywordIsARawIdentifier(t *testing.T) {
	assertHas(t, emitOne(t, shop), "pub r#type: String,", "x.r#type = Decode::decode(r)")
}

// A double cannot be Eq or Hash; a type holding one anywhere derives
// PartialEq alone, and one that does not can key a cache or an index.
func TestEqAndHashFollowTheMembers(t *testing.T) {
	src := emitOne(t, shop)
	assertHas(t, src,
		"#[derive(Debug, Clone, PartialEq, Default, Eq, Hash)]\npub struct Price {",
		"#[derive(Debug, Clone, PartialEq, Default)]\npub struct Offer {",
		"#[derive(Debug, Clone, PartialEq, Default, Eq, Hash)]\npub struct SlimOffer {",
	)
}

func TestCompositeIdentityIsAHashableStructBuiltFromItsParts(t *testing.T) {
	src := emitOne(t, shop)
	assertHas(t, src,
		"#[derive(Debug, Clone, PartialEq, Eq, Hash, Default)]\npub struct OfferKey {\n    pub product_id: i64,\n    pub offer_id: i64,\n}",
		"impl Keyed for Offer {\n    type Key = OfferKey;\n    fn cache_key(&self) -> OfferKey {\n        OfferKey {\n            product_id: self.product_id.clone(),\n            offer_id: self.id.clone(),\n        }",
		"pub fn offer_id(&self) -> &i64 {\n        &self.id\n    }",
		`1 => k.offer_id = Decode::decode(r).map_err(|e| e.at("OfferKey", i))?,`,
	)
}

// Section 4 gives a view its source's identity, so it has no key type of its own.
func TestAViewKeysByItsSourcesKeyType(t *testing.T) {
	src := emitOne(t, shop)
	assertHas(t, src, "impl Keyed for SlimOffer {\n    type Key = OfferKey;")
	if strings.Contains(src, "SlimOfferKey") {
		t.Error("a view emitted a key type of its own")
	}
}

func TestOnlyRustAnnotationsCross(t *testing.T) {
	src := emitOne(t, shop)
	assertHas(t, src,
		"#[derive(serde::Serialize)]\n#[derive(Debug, Clone, PartialEq, Default)]\npub struct Offer {",
		"    #[serde(rename = \"productId\")]\n    pub product_id: i64,",
	)
	if strings.Contains(src, "DataCache") {
		t.Error("a cs: annotation reached the Rust emission")
	}
}

// An enum is open: a value the schema does not know is kept, not rejected.
func TestAnEnumIsAnOpenNewtype(t *testing.T) {
	assertHas(t, emitOne(t, shop),
		"pub struct Status(pub i32);",
		"pub const NOT_STARTED: Status = Status(1);",
		"i32::decode(r).map(Status)",
		`None => write!(f, "Status({})", self.0),`,
	)
}

func TestEnrichmentWritesFromTheMessageAndSaysWhatItReads(t *testing.T) {
	src := emitOne(t, `revision 1;
namespace Shop;

table Product {
  key int64 id = 1;
  string tenant_id = 2 [header];
  string? region = 3 [header("x-region")];
  int64 written_at = 4 [from_timestamp];
}

table Plain {
  key int64 id = 1;
}
`)
	assertHas(t, src,
		`const ENRICHMENT: wire::Enrichment = wire::Enrichment { headers: &["TenantId", "x-region"], timestamp: true };`,
		"fn enrich(&mut self, m: &dyn wire::Meta) {\n        wire::header(m, \"TenantId\", &mut self.tenant_id);\n        wire::header_opt(m, \"x-region\", &mut self.region);\n        self.written_at = m.timestamp();\n    }",
	)
	if strings.Contains(section(src, "impl Decode for Plain {"), "enrich") {
		t.Error("a type with nothing to enrich has an enrich")
	}
}

// A kind the reader has no arm for keeps its payload rather than failing, so a
// producer can add a kind ahead of its consumers.
func TestAUnionDecodesItsPairAndKeepsWhatItDoesNotKnow(t *testing.T) {
	src := emitOne(t, `revision 1;
namespace A;
enum Kind {
  NONE = 0;
  NUMBER = 1;
  BLOB = 2;
}
union Value of Kind {
  NONE;
  NUMBER int64;
  BLOB raw;
}
`)
	assertHas(t, src,
		"pub enum Value {\n    None,\n    Number(i64),\n    Blob(wire::Raw),",
		"    Unknown(Kind, wire::Raw),\n}",
		"let (kind, n) = wire::union_begin(r, \"Value\")?;",
		"Kind::NONE => {\n                r.skip()?;\n                Self::None\n            }",
		"Kind::NUMBER => Self::Number(Decode::decode(r)",
		"k => Self::Unknown(k, Decode::decode(r)?),",
		"wire::union_end(r, n)?;",
		"impl Default for Value {\n    fn default() -> Self {\n        Self::None\n    }",
	)
}

func TestAnArmNamedUnknownMovesTheFallback(t *testing.T) {
	src := emitOne(t, `revision 1;
namespace A;
enum Kind {
  UNKNOWN = 0;
  NUMBER = 1;
}
union Value of Kind {
  UNKNOWN;
  NUMBER int64;
}
`)
	assertHas(t, src, "    Unknown,\n", "UnknownArm(Kind, wire::Raw),")
}

// A timestamp and a date are wire integers, so they order and key a range index.
func TestTimestampAndDateAreWireIntegers(t *testing.T) {
	assertHas(t, emitOne(t, `revision 1;
namespace Clock;
table Tick {
  key int64 id = 1;
  timestamp  at  = 2;
  timestamp? end = 3;
  date       on  = 4;
}`),
		"pub at: wire::Timestamp,",
		"pub end: Option<wire::Timestamp>,",
		"pub on: wire::Date,",
	)
}

func TestAnImportIsASiblingModule(t *testing.T) {
	units, err := Emit(schemaOf(t, map[string]string{
		"base-types.prague": "revision 1;\nnamespace Base;\nrecord Money { int64 cents = 1; }\n",
		"shop.prague":       "revision 1;\nnamespace Shop;\nimport \"base-types.prague\";\ntable Item { key int64 id = 1; Money price = 2; }\n",
	}), func(*check.File) bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]string{}
	for _, u := range units {
		names[u.Name] = u.Source
	}
	if _, ok := names["base_types.rs"]; !ok {
		t.Fatalf("units: %v", names)
	}
	assertHas(t, names["shop.rs"], "use super::base_types::*;", "pub price: Money,")
}

// Every example the C# backend is held against must emit.
func TestTheExamplesEmit(t *testing.T) {
	dir := filepath.Join("..", "..", "examples", "prague")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{}
	for _, e := range entries {
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		files[e.Name()] = string(b)
	}
	units, err := Emit(schemaOf(t, files), func(*check.File) bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	if len(units) != len(files) {
		t.Fatalf("%d units for %d files", len(units), len(files))
	}
}
