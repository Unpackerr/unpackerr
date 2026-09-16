import { writeFileSync } from 'node:fs'
import { defineConfig } from 'vite'
import { svelte } from '@sveltejs/vite-plugin-svelte'

// Where the running unpackerr HTTP server is, for `npm run dev`.
// Override with UN_BACKEND=http://host:port npm run dev
const backend = process.env.UN_BACKEND || 'http://127.0.0.1:5656'

const embedStub =
  '# Placeholder so //go:embed all:dist compiles before npm run build.\n*\n'

function keepEmbedStub() {
  return {
    name: 'keep-embed-stub',
    closeBundle() {
      writeFileSync('dist/.gitignore', embedStub)
    },
  }
}

// https://vite.dev/config/
export default defineConfig({
  // Relative base so the built assets work under any urlbase when embedded.
  base: './',
  plugins: [svelte(), keepEmbedStub()],
  server: {
    proxy: {
      '/api': { target: backend, changeOrigin: true },
      '/metrics': { target: backend, changeOrigin: true },
      '/ws': { target: backend, changeOrigin: false, ws: true },
    },
  },
  build: {
    outDir: 'dist',
    chunkSizeWarningLimit: 1500,
    sourcemap: false,
    rolldownOptions: {
      output: {
        codeSplitting: {
          groups: [
            {
              name: 'bootstrap',
              test: /[\\/]node_modules[\\/](?:@sveltestrap[\\/]sveltestrap|bootstrap)(?:[\\/]|$)/,
            },
          ],
        },
      },
    },
  },
})
