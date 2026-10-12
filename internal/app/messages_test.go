package app

import (
	"testing"
	"unsafe"

	"github.com/AVGsync/protolab/internal/engine"
	"github.com/AVGsync/protolab/internal/model"
)

func parsedWith(n int) engine.ParsedSession {
	return engine.ParsedSession{Messages: make([]model.Message, n)}
}

func TestMessageCacheBudgetCountsBytes(t *testing.T) {
	unit := parsedSize(parsedWith(1)) - parsedSize(parsedWith(0)) // one empty message
	base := parsedSize(parsedWith(0))
	c := newMessageCache(100 * unit)
	c.put("a", parsedWith(40))
	c.put("b", parsedWith(40))
	if _, ok := c.get("a"); !ok { // a becomes the most recently used
		t.Fatal("a evicted too early")
	}
	c.put("c", parsedWith(40)) // 120 messages > 100: the least recently used (b) goes
	if _, ok := c.get("b"); ok {
		t.Fatal("b must be evicted")
	}
	for _, k := range []string{"a", "c"} {
		if _, ok := c.get(k); !ok {
			t.Fatalf("%s must stay", k)
		}
	}
	if c.used != 80*unit+2*base {
		t.Fatalf("used = %d, want %d", c.used, 80*unit+2*base)
	}

	c.put("huge", parsedWith(500)) // larger than the whole cache: not stored, nothing evicted
	if _, ok := c.get("huge"); ok || c.used != 80*unit+2*base {
		t.Fatalf("oversized entry cached: used %d", c.used)
	}
	c.put("a", parsedWith(10)) // replacing an entry adjusts the weight
	if c.used != 50*unit+2*base {
		t.Fatalf("used after replace = %d", c.used)
	}

	// Weight follows what a message holds, not how many there are.
	heavy := parsedWith(1)
	heavy.Messages[0].Data = make([]byte, 10*unit)
	heavy.Messages[0].Fields = []model.ParsedField{{Path: "entries", Children: make([]model.ParsedField, 100)}}
	if w := parsedSize(heavy); w < 10*unit+100*int(unsafe.Sizeof(model.ParsedField{})) {
		t.Errorf("weight of a heavy message = %d", w)
	}
	c.clear()
	if c.used != 0 || c.order.Len() != 0 {
		t.Fatal("clear must reset the cache")
	}
}
