import path from 'path';
import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import tailwindcss from '@tailwindcss/vite';

export default defineConfig({
	plugins: [tailwindcss(), react()],
	resolve: {
		alias: { '@': path.resolve(__dirname, './src') }
	},
	server: {
		proxy: {
			'/api/v1': 'http://localhost:8080',
			'/api/v1/ws': { target: 'ws://localhost:8080', ws: true },
			'/login': 'http://localhost:8080'
		}
	},
	build: {
		outDir: '../web/dist',
		emptyOutDir: true
	}
});
