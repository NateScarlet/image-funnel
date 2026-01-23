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
    proxy: {
      "/graphql": {
        target: "http://localhost:8000",
        ws: true,
      },
      "/image": {
        target: "http://localhost:8000",
      },
    },
  },
  optimizeDeps: {
    include: ["@apollo/client", "graphql-tag"],
  },
  assetsInclude: ["**/*.gql"],
});
