import type { Ref } from "vue";
import { getCurrentInstance, onUnmounted, shallowRef, watch } from "vue";

/**
 * 任务上下文，提供资源管理和取消信号
 */
class TaskContext {
  /**
   * 用于管理任务相关资源的 DisposableStack
   */
  public readonly stack = new DisposableStack();

  /**
   * 创建一个与当前上下文关联的 AbortSignal
   * 当上下文被销毁时自动触发 abort
   */
  public readonly signal = () => {
    const ctr = this.stack.adopt(new AbortController(), (i) => i.abort());
    return ctr.signal;
  };
}

/**
 * 异步任务函数类型
 */
type TaskFunction<T, TArgs extends readonly unknown[]> = (
  ...v: [...TArgs, TaskContext]
) => PromiseLike<T> | T;

/**
 * 管理异步任务的组合式函数，支持自动取消和资源清理
 * @param options 任务配置项或任务函数本身
 * @returns 包含结果值、错误信息和重启方法的对象
 */
export default function useAsyncTask<
  const T,
  const TArgs extends readonly unknown[],
  const TDefault = undefined,
>(
  options:
    | TaskFunction<T, TArgs>
    | {
        /** 参数获取函数，返回 undefined 时跳过执行 */
        args?: () => TArgs | undefined;
        /** 自定义参数比较函数，默认浅比较 */
        argsEqual?: (a: TArgs, b: TArgs) => boolean;
        /** 要执行的异步任务函数 */
        task: TaskFunction<T, TArgs>;
        /** 默认值工厂函数 */
        defaultValue?: () => TDefault;
        /** 是否保留最后一次成功结果（默认 false） */
        keepLatest?: boolean;
        /** 外部加载计数引用，用于跟踪加载状态 */
        loadingCount?: Ref<number>;
      },
) {
  // 简化单函数参数的情况
  if (typeof options === "function") {
    return useAsyncTask({ task: options });
  }

  const {
    args,
    argsEqual = (a, b) =>
      a.length === b.length && a.every((_, index) => a[index] === b[index]),
    task,
    defaultValue,
    keepLatest,
    loadingCount,
  } = options;

  const error = shallowRef<unknown>();
  const value = shallowRef<T | TDefault>(defaultValue?.() as TDefault);
  let ctx = new TaskContext();
  let currentArgs: TArgs | undefined;

  // 组件卸载时清理当前上下文
  if (getCurrentInstance()) {
    onUnmounted(() => {
      ctx.stack.dispose();
    });
  }

  /**
   * 执行异步任务
   * @param ctx 当前任务上下文
   * @param args 任务参数
   */
  async function run(ctx: TaskContext, ...args: TArgs) {
    currentArgs = args;
    // 处理加载状态计数
    if (loadingCount) {
      loadingCount.value += 1;
    }
    try {
      error.value = undefined;
      if (!keepLatest) {
        value.value = defaultValue?.();
      }
      try {
        const res = await task(...args, ctx);
        if (ctx.stack.disposed) {
          // 如果上下文已被销毁（任务已被取消），则忽略结果
          return;
        }
        value.value = res;
      } catch (err) {
        if (ctx.stack.disposed) {
          return;
        }
        if (import.meta.env.DEV) {
          console.error({
            message: "Error in async task",
            task,
            args,
            error: err,
          });
        }
        error.value = err;
      }
    } finally {
      if (loadingCount) {
        loadingCount.value -= 1;
      }
    }
  }

  /**
   * 重启任务（自动取消之前的执行）
   * @param newArgs 新参数（默认使用当前参数）
   */
  function restart(): Promise<void>;
  function restart(...newArgs: TArgs): Promise<void>;
  function restart(...newArgs: TArgs) {
    // 清理之前的上下文（触发 AbortSignal 和 disposables）
    ctx.stack.dispose();

    // 创建新上下文
    ctx = new TaskContext();
    if (newArgs.length === 0) {
      currentArgs = currentArgs ?? args?.();
      if (currentArgs == null) {
        throw new Error(
          "restart async task requires args, but args() returned undefined",
        );
      }
      return run(ctx, ...currentArgs);
    }
    return run(ctx, ...newArgs);
  }

  // 自动监听参数变化
  if (args) {
    watch(
      args,
      (newArgs, oldArgs) => {
        if (
          newArgs == null ||
          (oldArgs != null && argsEqual(newArgs, oldArgs))
        ) {
          return;
        }
        restart(...newArgs);
      },
      { immediate: true },
    );
  } else {
    // @ts-expect-error 无参数的 run 必定支持直接执行
    run(ctx);
  }

  return {
    /** @deprecated 任务结果（响应式） */
    value,
    /** 任务结果（响应式） */
    result: value,
    /** 错误信息（响应式） */
    error,
    /** 手动重启任务的方法 */
    restart,
  };
}
