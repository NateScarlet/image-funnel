import { defineConfig } from "vite";
// Plugin 仅作类型使用：configLoader native 以原生 ESM 加载本文件，须用 type-only 导入
import type { Plugin } from "vite";
import { createRequire } from "node:module";
import vue from "@vitejs/plugin-vue";
import tailwindcss from "@tailwindcss/vite";
import path from "path";
import fs from "fs";
import { optimize } from "svgo";

// #region 受限沙箱兼容层
// vite 8 在 Windows 上首次路径解析时会无条件 exec("net use") 收集网络映射盘信息，
// 受限沙箱禁止子进程 stdio 管道导致 spawn EPERM 直接中断启动流程。
// 返回空输出时 vite 回退到 fs.realpathSync.native，无映射盘环境下行为完全等价；
// 其余命令不受影响地放行。通过 createRequire 拿到可写的内置模块对象完成替换。
type ExecCallback = (error: Error | null, stdout: string, stderr: string) => void;

const globalScope = globalThis as typeof globalThis & {
  __IMAGE_FUNNEL_NETUSE_PATCHED__?: boolean;
};
if (!globalScope.__IMAGE_FUNNEL_NETUSE_PATCHED__) {
  globalScope.__IMAGE_FUNNEL_NETUSE_PATCHED__ = true;

  const cp = createRequire(import.meta.url)("node:child_process") as {
    exec: (command: string, options?: unknown, callback?: ExecCallback) => unknown;
    exec: (command: string, callback?: ExecCallback) => unknown;
  };

  const originalExec = cp.exec;
  cp.exec = ((command: string, optionsOrCallback?: unknown, maybeCallback?: ExecCallback) => {
    if (command === "net use") {
      const callback = (typeof optionsOrCallback === "function"
        ? optionsOrCallback
        : maybeCallback) as ExecCallback;
      process.nextTick(() => callback(null, "", ""));
      return {
        on() {},
        kill() {},
        unref() {},
        stdout: null,
        stderr: null,
        stdin: null,
        pid: 0,
      };
    }
    return (originalExec as (...args: unknown[]) => unknown)(command, optionsOrCallback, maybeCallback);
  }) as typeof cp.exec;
}
// #endregion

// 构建和开发阶段注入首屏 Loading SVG 的插件
function injectLoadingSvg(): Plugin {
  return {
    name: "inject-loading-svg",
    transformIndexHtml(html) {
      const svgPath = path.resolve(import.meta.dirname, "./src/assets/loading.svg");
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

export default defineConfig(() => ({
  plugins: [vue(), tailwindcss(), injectLoadingSvg()],
  resolve: {
    alias: {
      "@": path.resolve(import.meta.dirname, "./src"),
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
  test: {
    environment: "jsdom",
    // 受限沙箱禁止子进程 stdio 管道，默认的 forks pool（子进程）无法启动；
    // threads 使用进程内 worker_threads，测试隔离在线程级进行
    pool: "threads",
  },
}));
