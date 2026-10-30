import {defineConfig} from 'vite'

export default defineConfig({
  build: {
    // Relative paths so assets resolve correctly inside the Wails webview.
    assetsDir: 'assets',
  },
})
