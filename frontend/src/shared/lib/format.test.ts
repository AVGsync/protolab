import { describe, expect, it } from 'vitest';
import { formatDuration } from './format';

describe('formatDuration', () => {
  it('does not present a sub-millisecond run as zero time', () => {
    expect(formatDuration(0)).toBe('< 1 мс');
    expect(formatDuration(0.25)).toBe('250 мкс');
  });
  it('formats milliseconds, seconds and minutes', () => {
    expect(formatDuration(3)).toBe('3 мс');
    expect(formatDuration(3.25)).toBe('3.25 мс');
    expect(formatDuration(1500)).toBe('1.50 с');
    expect(formatDuration(486_000)).toBe('8 мин 6 с');
  });
});
