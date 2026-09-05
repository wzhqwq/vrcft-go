import {defineConfig} from '@playwright/test';
import {fileURLToPath} from 'node:url';

const frontendDirectory = fileURLToPath(new URL('.', import.meta.url));
const pnpm = process.platform === 'win32' ? 'pnpm.cmd' : 'pnpm';

export default defineConfig({
  testDir: './e2e',
  workers: 1,
  use: {
    baseURL: 'http://127.0.0.1:4173',
  },
  webServer: {
    command: `${pnpm} exec vite --host 127.0.0.1 --port 4173`,
    cwd: frontendDirectory,
    url: 'http://127.0.0.1:4173',
    reuseExistingServer: !process.env.CI,
  },
});
