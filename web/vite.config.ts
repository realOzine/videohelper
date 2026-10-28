import { resolve } from 'node:path';
import tailwindcss from '@tailwindcss/vite';
import react from '@vitejs/plugin-react';
import { defineConfig } from 'vite';

// One HTML file per page keeps the plain URLs (search.html, video.html?id=…) that any static host serves.
const pages = ['search', 'library', 'video', 'settings'];

export default defineConfig({
  base: './',
  publicDir: 'src/fonts/licenses',
  plugins: [react(), tailwindcss()],
  server: { host: '127.0.0.1', port: 5173, strictPort: true },
  build: { rollupOptions: { input: Object.fromEntries(pages.map((p) => [p, resolve(import.meta.dirname, `${p}.html`)])) } },
});
