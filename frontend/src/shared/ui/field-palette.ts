/**
 * Fields cycle through four tones (hex underline, lane tint, table swatch).
 * The hues avoid the channel colours (amber, cyan) and the danger red.
 */
export const FIELD_TONES = 4;

export const fieldTone = (index: number): number => ((index % FIELD_TONES) + FIELD_TONES) % FIELD_TONES;

const SWATCH = ['bg-field-1', 'bg-field-2', 'bg-field-3', 'bg-field-4'] as const;
const LANE = ['bg-field-lane-1', 'bg-field-lane-2', 'bg-field-lane-3', 'bg-field-lane-4'] as const;
const UNDERLINE = ['var(--pl-field-1)', 'var(--pl-field-2)', 'var(--pl-field-3)', 'var(--pl-field-4)'] as const;

/** Solid swatch next to a field name. */
export const fieldSwatchClass = (index: number): string => SWATCH[fieldTone(index)];
/** Tinted segment of the field lane. */
export const fieldLaneClass = (index: number): string => LANE[fieldTone(index)];
/** CSS colour of the 2px hex underline. */
export const fieldUnderline = (index: number): string => UNDERLINE[fieldTone(index)];
