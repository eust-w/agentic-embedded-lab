import { defineConfig } from 'vitest/config'
import react from '@vitejs/plugin-react'

export default defineConfig({
  plugins: [react()],
  server: { port: 34115, strictPort: true },
  build: { outDir: 'dist', sourcemap: true },
  test: { environment: 'jsdom', globals: true, setupFiles: './src/test/setup.ts' },
})
