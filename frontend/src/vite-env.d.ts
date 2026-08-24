declare module "*.vue" {
  import type { DefineComponent } from "vue";
  const component: DefineComponent<Record<string, unknown>, Record<string, unknown>, unknown>;
  export default component;
}

/** 构建时由 Vite define 注入的应用版本，与后端同源（git describe）；无法判定时为 "dev" */
// eslint-disable-next-line no-underscore-dangle
declare const __APP_VERSION__: string;
