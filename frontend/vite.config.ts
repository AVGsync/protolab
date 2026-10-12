import { readdirSync, rmSync } from 'node:fs';
import { join } from 'node:path';
import { fileURLToPath, URL } from 'node:url';
import tailwindcss from '@tailwindcss/vite';
import react from '@vitejs/plugin-react';
import { defineConfig, loadEnv, type Plugin } from 'vite';

// The production build is embedded into the Go binary (internal/webui, //go:embed all:dist).
const outDir = fileURLToPath(new URL('../internal/webui/dist', import.meta.url));
const KEEP = new Set(['README.md']); // tracked placeholder so `go build` works before the first frontend build

/** Cleans the embed directory except the tracked placeholder, and drops the MSW worker outside mock builds. */
function embedOutput(mode: string): Plugin {
  return {
    name: 'protolab:embed-output',
    apply: 'build',
    buildStart() {
      for (const name of readdirSync(outDir)) {
        if (!KEEP.has(name)) rmSync(join(outDir, name), { recursive: true, force: true });
      }
    },
    closeBundle() {
      if (mode !== 'mock') rmSync(join(outDir, 'mockServiceWorker.js'), { force: true });
    },
  };
}

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), '');

  return {
    plugins: [react(), tailwindcss(), embedOutput(mode)],
    resolve: {
      alias: { '@': fileURLToPath(new URL('./src', import.meta.url)) },
    },
    server: {
      port: 5173,
      // In dev mode all /api requests go to the Go backend.
      proxy: {
        '/api': {
          target: env.VITE_API_PROXY_TARGET || 'http://127.0.0.1:7420',
          changeOrigin: true,
        },
      },
    },
    build: {
      outDir,
      emptyOutDir: false,
      sourcemap: false,
      chunkSizeWarningLimit: 700,
    },
  };
});
