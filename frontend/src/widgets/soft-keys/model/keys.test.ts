import { describe, expect, it } from 'vitest';
import { isTypingTarget, matchesKey } from './keys';

const key = (init: Partial<KeyboardEvent>) => ({ key: '', code: '', shiftKey: false, ...init }) as KeyboardEvent;

describe('matchesKey', () => {
  it('matches letters by physical key, so Cyrillic layouts work', () => {
    expect(matchesKey(key({ key: 'в', code: 'KeyD' }), 'D')).toBe(true);
    expect(matchesKey(key({ key: 'd', code: 'KeyD' }), 'd')).toBe(true);
    expect(matchesKey(key({ key: 'D', code: 'KeyD', shiftKey: true }), 'D')).toBe(false);
  });

  it('matches section digits', () => {
    expect(matchesKey(key({ key: '4', code: 'Digit4' }), '4')).toBe(true);
    expect(matchesKey(key({ key: '$', code: 'Digit4', shiftKey: true }), '4')).toBe(false);
  });

  it('distinguishes "/" from "?" on the same physical key', () => {
    expect(matchesKey(key({ key: '/', code: 'Slash' }), '/')).toBe(true);
    expect(matchesKey(key({ key: '?', code: 'Slash', shiftKey: true }), '/')).toBe(false);
    expect(matchesKey(key({ key: '?', code: 'Slash', shiftKey: true }), '?')).toBe(true);
    // "?" typed on a Cyrillic layout (Shift+7)
    expect(matchesKey(key({ key: '?', code: 'Digit7', shiftKey: true }), '?')).toBe(true);
  });
});

describe('isTypingTarget', () => {
  it('is false for non-elements', () => {
    expect(isTypingTarget(null)).toBe(false);
  });
});
