package app

import (
	"testing"

	"github.com/AVGsync/protolab/internal/engine"
	"github.com/AVGsync/protolab/internal/model"
)

func parsedWith(n int) engine.ParsedSession {
	return engine.ParsedSession{Messages: make([]model.Message, n)}
}

func TestMessageCacheBudgetCountsMessages(t *testing.T) {
	c := newMessageCache(100)
	c.put("a", parsedWith(40))
	c.put("b", parsedWith(40))
	if _, ok := c.get("a"); !ok { // a becomes the most recently used
		t.Fatal("a evicted too early")
	}
	c.put("c", parsedWith(40)) // 123 > 100: the least recently used (b) goes
	if _, ok := c.get("b"); ok {
		t.Fatal("b must be evicted")
	}
	for _, k := range []string{"a", "c"} {
		if _, ok := c.get(k); !ok {
			t.Fatalf("%s must stay", k)
		}
	}
	if c.used != 82 {
		t.Fatalf("used = %d, want 82", c.used)
	}

	c.put("huge", parsedWith(500)) // larger than the whole cache: not stored, nothing evicted
	if _, ok := c.get("huge"); ok || c.used != 82 {
		t.Fatalf("oversized entry cached: used %d", c.used)
	}
	c.put("a", parsedWith(10)) // replacing an entry adjusts the weight
	if c.used != 52 {
		t.Fatalf("used after replace = %d, want 52", c.used)
	}
	c.clear()
	if c.used != 0 || c.order.Len() != 0 {
		t.Fatal("clear must reset the cache")
	}
}
