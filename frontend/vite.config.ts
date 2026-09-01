import tailwindcss from '@tailwindcss/vite'
import {svelte} from '@sveltejs/vite-plugin-svelte'
import {defineConfig, loadEnv} from 'vite'

export function isComponentWorkbenchEnabled(mode: string, value: string | undefined): boolean {
  return mode !== 'production' && value === 'true'
}

// https://vitejs.dev/config/
export default defineConfig(({mode}) => {
  const env = loadEnv(mode, process.cwd(), 'VITE_')

  return {
    define: {
      __COMPONENT_WORKBENCH__: JSON.stringify(isComponentWorkbenchEnabled(mode, env.VITE_COMPONENT_WORKBENCH)),
    },
    plugins: [tailwindcss(), svelte()],
  }
})
