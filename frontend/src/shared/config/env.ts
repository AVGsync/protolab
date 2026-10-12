export const env = {
  /** Base path of the REST API (served by the Go backend on the same origin). */
  apiBaseUrl: (import.meta.env.VITE_API_BASE_URL ?? '/api/v1').replace(/\/$/, ''),
  /** Emulate the API in the browser with MSW (src/mocks). */
  useMocks: import.meta.env.VITE_API_MOCKS === 'true',
} as const;
