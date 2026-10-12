/** Categorical colors for field highlights in the hex view. */
const FIELD_COLORS = ['#4c9aff', '#a78bfa', '#3fb97f', '#f08a24', '#e879f9', '#22d3ee', '#facc15', '#fb7185'];

export const fieldColor = (index: number): string => FIELD_COLORS[index % FIELD_COLORS.length];
