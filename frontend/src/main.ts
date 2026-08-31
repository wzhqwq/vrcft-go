import './style.css'
import App from './App.svelte'
import {mount} from 'svelte'

const target = document.getElementById('app')

if (target === null) {
  throw new Error('app mount target not found')
}

const app = mount(App, {target})

export default app
