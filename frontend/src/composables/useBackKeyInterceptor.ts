import { onScopeDispose } from "vue";
import useEventListeners from "@/composables/useEventListeners";

// #region 全局活跃拦截栈与最新状态维护
interface InterceptorInstance {
  allow: () => boolean;
}

const activeInterceptors: InterceptorInstance[] = [];

// 记录物理返回前最新的历史记录状态与 URL，在拦截物理返回时重新推入以恢复页面状态
let latestState: unknown = window.history.state;
let latestStateURL = window.location.href;

console.log(
  "[useBackKeyInterceptor Global] Initialized, state:",
  latestState,
  "url:",
  latestStateURL,
);

const originalPushState = history.pushState;
const originalReplaceState = history.replaceState;

// 劫持 history 状态变更方法，同步更新最新页面状态与 URL
// XXX: 由于环境限制，没有更好的方法来记录最新状态
history.pushState = function (...args) {
  originalPushState.apply(this, args);
  latestState = args[0];
  latestStateURL = window.location.href;
  console.log(
    "[useBackKeyInterceptor Global] pushState called, state:",
    latestState,
    "url:",
    latestStateURL,
  );
};

history.replaceState = function (...args) {
  originalReplaceState.apply(this, args);
  latestState = args[0];
  latestStateURL = window.location.href;
  console.log(
    "[useBackKeyInterceptor Global] replaceState called, state:",
    latestState,
    "url:",
    latestStateURL,
  );
};

window.addEventListener("hashchange", () => {
  latestStateURL = window.location.href;
  latestState = window.history.state;
});
// #endregion

/**
 * 物理返回键拦截器的 Composable
 * @param allowPopState 物理返回触发时的关闭/拦截回调，需同步返回 boolean (false 代表拦截物理返回，不关闭)
 */
export default function useBackKeyInterceptor(allowPopState: () => boolean) {
  let stack: DisposableStack | undefined;
  onScopeDispose(() => stack?.dispose(), true);

  function register() {
    // eslint-disable-next-line no-constant-condition
    if (1 == 1) {
      // TODO!: fix this
      return;
    }
    stack?.dispose();
    stack = new DisposableStack();
    let dummyStateConsumed = false;

    stack.adopt(window.history.pushState(null, ""), () => {
      if (!dummyStateConsumed) {
        console.log("history back");
        window.history.back();
      }
    });
    stack.use(
      // 通过响应式目标实现按需绑定与自动注销
      useEventListeners(window, ({ on }) => {
        console.log(
          "[useBackKeyInterceptor Instance] Event listener active / setup on window",
        );
        on("popstate", () => {
          const isTop =
            activeInterceptors[activeInterceptors.length - 1] === instance;
          console.log(
            "[useBackKeyInterceptor Instance] popstate event triggered. isTop:",
            isTop,
            "activeInterceptors length:",
            activeInterceptors.length,
          );
          if (isTop) {
            const allowed = allowPopState();
            console.log(
              "[useBackKeyInterceptor Instance] allowPopState result:",
              allowed,
            );
            if (allowed) {
              unregister();
              latestState = window.history.state;
              latestStateURL = window.location.href;
              console.log(
                "[useBackKeyInterceptor Instance] allowed, updated latestState:",
                latestState,
                "url:",
                latestStateURL,
              );
              dummyStateConsumed = true;
            } else {
              // 如果不允许关闭，将发生返回前最新的状态重新推入历史记录以实现拦截
              console.log(
                "[useBackKeyInterceptor Instance] NOT allowed, pushState back to url:",
                latestStateURL,
                "state:",
                latestState,
              );
              history.pushState(latestState, "", latestStateURL);
            }
          }
        });
      }),
    );

    console.log("[useBackKeyInterceptor Instance] register() called");
    const instance: InterceptorInstance = { allow: allowPopState };
    if (!activeInterceptors.includes(instance)) {
      activeInterceptors.push(instance);
    }
    stack.defer(() => {
      const index = activeInterceptors.indexOf(instance);
      if (index !== -1) {
        activeInterceptors.splice(index, 1);
      }
    });
  }

  function unregister() {
    console.log("[useBackKeyInterceptor Instance] unregister() called");
    stack?.dispose();
    console.log(
      "[useBackKeyInterceptor Instance] unregistered, activeInterceptors length:",
      activeInterceptors.length,
    );
  }

  return {
    register,
    unregister,
  };
}
