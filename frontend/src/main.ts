import './style.css'
import {mount} from 'svelte'

const target = document.getElementById('app')

if (target === null) {
  throw new Error('app mount target not found')
}

const Component = import.meta.env.DEV && __COMPONENT_WORKBENCH__
  ? (await import('./dev/ComponentWorkbench.svelte')).default
  : (await import('./App.svelte')).default

const app = mount(Component, {target})

export default app
