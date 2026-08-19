import { defineConfig, loadEnv } from 'vite';
import react from '@vitejs/plugin-react';
import path from 'path';
export default defineConfig(function (_a) {
    var mode = _a.mode;
    var env = loadEnv(mode, process.cwd(), '');
    var backendUrl = env.VITE_BACKEND_URL || 'http://localhost:8080';
    return {
        plugins: [react()],
        resolve: {
            alias: {
                '@': path.resolve(__dirname, './src'),
            },
        },
        server: {
            host: '0.0.0.0',
            port: 5173,
            proxy: {
                '/api': { target: backendUrl, changeOrigin: true },
                '/health': { target: backendUrl, changeOrigin: true },
                '/ready': { target: backendUrl, changeOrigin: true },
                '/metrics': { target: backendUrl, changeOrigin: true },
            },
        },
    };
});
