import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// https://vite.dev/config/
export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    // Dev-only: proxy auth + API calls to the API server (:8080) so the
    // browser sees same-origin requests — cookies, CSRF header, and the
    // session model all work exactly as they do once this SPA is built
    // and served for real by internal/webreact. internal/api's CORS
    // middleware exists as a documented fallback, not something this
    // proxy needs to route around.
    proxy: {
      '/api': 'http://localhost:8080',
      '/login': 'http://localhost:8080',
      '/register': 'http://localhost:8080',
      '/logout': 'http://localhost:8080',
      '/password-reset': 'http://localhost:8080',
    },
  },
})
