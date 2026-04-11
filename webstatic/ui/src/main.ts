import { mount } from './app.js';

const root = document.getElementById('app');
if (!root) throw new Error('missing #app');
mount(root);
