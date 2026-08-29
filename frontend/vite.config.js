import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// Backend alohida ishlaydi (Go, :8080). Manzil .env dagi VITE_API_URL bilan
// beriladi; berilmasa /api so'rovlari dev serverdan backendga proksilanadi.
export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    proxy: { '/api': 'http://localhost:8080' },
  },
})
