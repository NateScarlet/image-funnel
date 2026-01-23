import { defineConfig } from "vite";
import vue from "@vitejs/plugin-vue";
import tailwindcss from "@tailwindcss/vite";
import path from "path";

export default defineConfig({
  plugins: [vue(), tailwindcss()],
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "./src"),
    },
  },
  server: {
    port: 8080,
    strictPort: true,
    host: "127.0.0.1",
    proxy: {
      "/graphql": {
        target: "http://127.0.0.1:8000",
        ws: true,
      },
      "/image": {
        target: "http://127.0.0.1:8000",
      },
    },
  },
  optimizeDeps: {
    include: ["@apollo/client"],
  },
  assetsInclude: ["**/*.gql"],
});
