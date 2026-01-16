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
    emptyOutDir: true,
    chunkSizeWarningLimit: 1500,
    rollupOptions: {
      output: {
        manualChunks(id) {
          if (id.includes('node_modules')) {
            if (id.includes('element-plus')) {
              return 'element-plus'
            }
            if (id.includes('vue') || id.includes('pinia') || id.includes('vue-router')) {
              return 'vue-vendor'
            }
            if (id.includes('@element-plus/icons-vue')) {
              return 'element-plus-icons'
            }
            return 'vendor'
          }
        }
      }
    }
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
