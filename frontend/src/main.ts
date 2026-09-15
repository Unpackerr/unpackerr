import './app.css'
import './lib/theme.svelte'
import './lib/i18n/locale.svelte'
import { mount } from 'svelte'
import App from './App.svelte'

const app = mount(App, { target: document.getElementById('app')! })

export default app
