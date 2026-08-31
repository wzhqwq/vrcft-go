import tailwindcss from '@tailwindcss/vite'
import {svelte} from '@sveltejs/vite-plugin-svelte'
import {defineConfig} from 'vite'

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [tailwindcss(), svelte()]
})
