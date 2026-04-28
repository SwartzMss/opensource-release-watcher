import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

const backendURL = process.env.BACKEND_URL ?? 'http://localhost:8080';

export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    proxy: {
      '/api': backendURL,
      '/healthz': backendURL,
    },
  },
});
