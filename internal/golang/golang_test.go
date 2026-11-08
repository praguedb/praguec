package golang

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

// The Go backend's oracle is not here. Whether the emitted decoders read what
// the C# backend's types write is held by github.com/praguedb/prague.go, which
// decodes fixtures written by Prague (.NET)'s own serializer from one schema
// compiled both ways. These tests hold the shape: what lands at which index,
// and which annotations cross.

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
	units, err := Emit(schemaOf(t, map[string]string{"a.prague": src}), "a", func(*check.File) bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	if len(units) != 1 || units[0].Name != "a.go" {
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
  [cs: DataCacheTopic("x")] [go: Offer is one seller price.]
{
  reserved 4;
  key key = 1 (product_id, offer_id);
  int64 product_id = 2 [go: json:"productId"] [cs: DataCacheIndex];
  int64 id = 3;
  alias offer_id = id;
  Price? price = 5;
  list<Price?> history = 6;
  Status? status = 7;
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
		"case 1:\n\t\t\terr = d.Decode(&x.ProductId)",
		"case 4:\n\t\t\terr = d.Decode(&x.Price)",
		"default:\n\t\t\terr = d.Skip()",
	)
	// Slot 0 is the composite key's duplicated tuple and slot 3 is reserved:
	// neither is decoded into anything.
	decoder := src[strings.Index(src, "func (x *Offer) DecodeMsgpack"):]
	decoder = decoder[:strings.Index(decoder, "\n}\n")]
	for _, slot := range []string{"case 0:", "case 3:"} {
		if strings.Contains(decoder, slot) {
			t.Errorf("Offer decodes %s", slot)
		}
	}
}

func TestPresenceIsAPointerAndAListIsASlice(t *testing.T) {
	src := emitOne(t, shop)
	assertHas(t, src,
		"Price     *Price",
		"History   []*Price",
		"Status    *Status",
		"Amount wire.Decimal",
		`"github.com/praguedb/prague.go/wire"`,
	)
}

func TestCompositeIdentityIsAComparableStructBuiltFromItsParts(t *testing.T) {
	src := emitOne(t, shop)
	assertHas(t, src,
		"type OfferKey struct {\n\tProductId int64\n\tOfferId   int64\n}",
		"func (x *Offer) CacheKey() OfferKey {\n\treturn OfferKey{ProductId: x.ProductId, OfferId: x.Id}\n}",
		"func (x *Offer) OfferId() int64 { return x.Id }",
	)
}

// Section 4 gives a view its source's identity, so it has no key type of its own.
func TestAViewKeysByItsSourcesKeyType(t *testing.T) {
	src := emitOne(t, shop)
	assertHas(t, src, "func (x *SlimOffer) CacheKey() OfferKey {")
	if strings.Contains(src, "SlimOfferKey") {
		t.Error("a view emitted a key type of its own")
	}
}

func TestOnlyGoAnnotationsCross(t *testing.T) {
	src := emitOne(t, shop)
	assertHas(t, src, "ProductId int64 `json:\"productId\"`", "// Offer is one seller price.\ntype Offer struct")
	if strings.Contains(src, "DataCache") {
		t.Error("a cs: annotation reached the Go emission")
	}
}

func TestEnumArmsArePrefixedByTheirType(t *testing.T) {
	assertHas(t, emitOne(t, shop), "StatusNotStarted Status = 1")
}

// Every example the C# backend is held against must also produce Go that
// gofmt accepts; Emit fails otherwise.
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
	units, err := Emit(schemaOf(t, files), "models", func(*check.File) bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	if len(units) != len(files) {
		t.Fatalf("%d units for %d files", len(units), len(files))
	}
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
		"func (x *Product) Enrich(m wire.Meta) {\n\twire.Header(m, \"TenantId\", &x.TenantId)\n\twire.HeaderOpt(m, \"x-region\", &x.Region)\n\tx.WrittenAt = m.Timestamp()\n}",
		`var productEnrichment = wire.Enrichment{Headers: []string{"TenantId", "x-region"}, Timestamp: true}`,
		"func (*Product) Enrichment() wire.Enrichment { return productEnrichment }",
	)
	if strings.Contains(src, "func (x *Plain) Enrich") {
		t.Error("a type with nothing to enrich has an Enrich")
	}
}

// A kind the reader has no arm for keeps its payload in Raw rather than
// failing, so a producer can add a kind ahead of its consumers.
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
		"Number int64",
		"Blob   msgpack.RawMessage",
		"Raw msgpack.RawMessage",
		"case KindNone:\n\t\terr = d.Skip()",
		"case KindNumber:\n\t\terr = d.Decode(&x.Number)",
		"default:\n\t\tx.Raw, err = d.DecodeRaw()",
	)
}

// A timestamp and a date are wire integers, so they order and key a range index.
func TestTimestampAndDateAreWireIntegers(t *testing.T) {
	src := emitOne(t, `revision 1;
namespace Clock;
table Tick {
  key int64 id = 1;
  timestamp  at  = 2;
  timestamp? end = 3;
  date       on  = 4;
}`)
	assertHas(t, src,
		"At  wire.Timestamp",
		"End *wire.Timestamp",
		"On  wire.Date",
		`"github.com/praguedb/prague.go/wire"`,
	)
}
