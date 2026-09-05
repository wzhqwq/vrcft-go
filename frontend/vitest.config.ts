import {svelte} from '@sveltejs/vite-plugin-svelte';
import {defineConfig} from 'vitest/config';

export default defineConfig({
  define: {
    __COMPONENT_WORKBENCH__: 'true',
  },
  plugins: [svelte()],
  resolve: {
    conditions: ['browser'],
  },
  test: {
    environment: 'jsdom',
    setupFiles: ['./src/lib/test/setup.ts'],
    exclude: ['**/node_modules/**', 'e2e/**'],
  },
});
