import { createApp } from "vue";
import type { RouteRecordRaw } from "vue-router";
import { createRouter, createWebHistory } from "vue-router";
import App from "./App.vue";
import "./main.tailwind.css";

import HomeView from "./views/HomeView.vue";
import SessionView from "./views/SessionView.vue";
import BrowseView from "./views/BrowseView.vue";
import AuthView from "./views/AuthView.vue";

import useNotification from "./composables/useNotification";
import { createVersionCheck } from "./utils/versionCheck";
import query from "./graphql/utils/query";
import { MetaDocument } from "./graphql/generated";
import { websocketConnected } from "./events";

const routes: RouteRecordRaw[] = [
  { path: "/", component: HomeView },
  {
    name: "session",
    path: "/session/:id",
    component: SessionView,
    props: true,
  },
  {
    name: "browse",
    path: "/browse",
    component: BrowseView,
  },
  {
    name: "auth",
    path: "/auth",
    component: AuthView,
  },
];

const router = createRouter({
  history: createWebHistory(),
  routes,
});

// #region 版本失配检测（组合根：在此装配依赖并注入）
// 前后端同包部署重启时，旧页面的 WS 必然断开重连；每次连接建立时比对
// 服务端 meta.version 与构建注入的 __APP_VERSION__，失配则提示手动刷新。
const { show, remove } = useNotification();
let stalePromptId: number | undefined;

const versionCheck = createVersionCheck({
  builtVersion: __APP_VERSION__,
  fetchServerVersion: async () => {
    try {
      const result = await query(MetaDocument, {
        context: { suppressError: true },
        fetchPolicy: "no-cache",
      });
      return result.data?.meta.version;
    } catch (error) {
      // 版本检查是 spec 约定的旁路逻辑：查询失败映射为"无法判定"跳过本次比对，
      // 等待下次 WS 连接再查；输出错误保持调试可见，不打扰主流程
      console.error("版本检查失败，等待下次连接重试", error);
      return undefined;
    }
  },
  showStalePrompt: (serverVersion) => {
    // 先移除旧提示再显示，保证同一时刻至多一条失配提示且文案为最新触发原因。
    // 非持久通知：用户可主动关闭继续操作，关闭后由 dismissPrompt 复位状态、下次检查重弹
    if (stalePromptId !== undefined) {
      remove(stalePromptId);
    }
    stalePromptId = show(
      serverVersion === undefined ? "页面资源已过期" : `发现新版本 ${serverVersion}`,
      "warning",
      0,
      "服务端已更新，刷新页面以恢复一致行为。",
      false,
      {
        openDetails: () => window.location.reload(),
        openDetailsText: "立即刷新",
        dismiss: () => versionCheck.dismissPrompt(),
      },
    );
  },
  clearStalePrompt: () => {
    if (stalePromptId !== undefined) {
      remove(stalePromptId);
      stalePromptId = undefined;
    }
  },
});

websocketConnected.subscribe(() => {
  void versionCheck.checkOnConnected();
});
// 懒加载 chunk 因重新部署被删除而加载失败：页面已不可靠，
// 阻止 Vite 把原始错误作为未捕获异常抛出，改为给出明确的刷新提示。
// vite:preloadError 是 Vite 在 window 上发出的原生事件，非项目自定义事件，直接在此接线
window.addEventListener("vite:preloadError", (event) => {
  event.preventDefault();
  versionCheck.reportPreloadFailure();
});
// #endregion

const app = createApp(App);
app.use(router);
app.mount("#app");
