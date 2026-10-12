import { describe, expect, it } from 'vitest';
import html from '../../index.html?raw';

// The Go server sends Content-Security-Policy "script-src 'self'" (internal/server/server.go):
// an inline script in index.html would be blocked in the real application, not in `npm run dev`.
describe('index.html', () => {
  it('has no inline scripts', () => {
    const scripts = [...html.matchAll(/<script\b([^>]*)>([\s\S]*?)<\/script>/g)];
    expect(scripts.length).toBeGreaterThan(0);
    for (const [, attrs, body] of scripts) {
      expect(attrs).toMatch(/\bsrc=/);
      expect(body.trim()).toBe('');
    }
  });
});
