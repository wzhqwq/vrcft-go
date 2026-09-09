import './style.css'
import {mount} from 'svelte'

const target = document.getElementById('app')

if (target === null) {
  throw new Error('app mount target not found')
}

const app = import.meta.env.DEV && __COMPONENT_WORKBENCH__
  ? mount((await import('./dev/ComponentWorkbench.svelte')).default, {target})
  : mount((await import('./App.svelte')).default, {target})

export default app
