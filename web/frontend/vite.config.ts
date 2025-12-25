import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import { resolve } from 'path'

// https://vite.dev/config/
export default defineConfig(({ command }) => ({
  plugins: [vue()],
  resolve: {
    alias: {
      '@': resolve(__dirname, 'src')
    }
  },
  build: {
    outDir: '../static/dist',
    emptyOutDir: true
  },
  esbuild: {
    drop: command === 'build' ? ['console', 'debugger'] : [],
    pure: command === 'build' ? ['console.log', 'console.info', 'console.debug', 'console.warn', 'console.error'] : []
  },
  server: {
    proxy: {
      '/api': {
        target: 'http://localhost:8080',
        changeOrigin: true
      },
      '/logout': {
        target: 'http://localhost:8080',
        changeOrigin: true
      }
    }
  }
}))
