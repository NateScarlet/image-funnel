import { defineConfig, Plugin } from "vite";
import vue from "@vitejs/plugin-vue";
import tailwindcss from "@tailwindcss/vite";
import path from "path";
import fs from "fs";
import { optimize } from "svgo";

// 构建和开发阶段注入首屏 Loading SVG 的插件
function injectLoadingSvg(): Plugin {
  return {
    name: "inject-loading-svg",
    transformIndexHtml(html) {
      const svgPath = path.resolve(__dirname, "./src/assets/loading.svg");
      if (fs.existsSync(svgPath)) {
        const svgContent = fs.readFileSync(svgPath, "utf-8");
        // 压缩 Loading SVG 以减小首屏 HTML 的体积，提高首屏加载速度
        const optimized = optimize(svgContent, {
          path: svgPath,
          multipass: true,
        });
        return html.replace("<!-- INJECT_LOADING_SVG -->", optimized.data);
      }
      return html;
    },
  };
}

export default defineConfig((env) => ({
  plugins: [vue(), tailwindcss(), injectLoadingSvg()],
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "./src"),
    },
  },
  esbuild: {
    pure:
      env.mode === "production"
        ? ["console.log", "console.info", "console.debug", "console.warn"]
        : [],
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
}));
