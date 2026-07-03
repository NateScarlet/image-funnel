import {
  ref,
  shallowRef,
  onUnmounted,
  toValue,
  getCurrentInstance,
  provide,
  inject,
  computed,
  watch,
  useId,
  type Ref,
  type MaybeRefOrGetter,
} from "vue";

/**
 * 快捷键配置项
 */
export interface HotkeyOptions {
  /**
   * 是否在输入框等可输入元素聚焦时依然触发
   * @default false
   */
  allowInInputs?: boolean;
  /**
   * 是否阻止默认行为
   * @default true
   */
  preventDefault?: boolean;
  /**
   * 是否阻止事件冒泡 (在全局分发中表现为阻断更低优先级的快捷键触发)
   * @default true
   */
  stopPropagation?: boolean;
  /**
   * 快捷键的功能描述
   */
  description?: string;
  /**
   * 快捷键所属的分组名称 (如 "图片评分", "图片操作", "导航切换")
   */
  category?: string;
  /**
   * 是否启用当前快捷键，支持响应式更新。
   * 如果传入的函数包含参数，则被视作 context 过滤函数运行，绕过默认的 scope 匹配规则。
   * @default true
   */
  enabled?:
    | MaybeRefOrGetter<boolean>
    | ((ctx: { topmostScope: string | undefined; activeScopes: string[] }) => boolean);
  /**
   * 是否是全局快捷键，全局快捷键在任何 active scope 之下都可以被触发。
   * @default false
   */
  global?: boolean;
  /**
   * 显式指定快捷键所属的 scope。默认从父组件 inject 注入。
   */
  scope?: MaybeRefOrGetter<string | undefined>;
}

/**
 * 快捷键组合键参数
 */
export interface HotkeyCombination {
  /**
   * 按键值（如 "1", "a", "arrowup"），不区分大小写
   */
  key: string;
  /**
   * 是否按下 Ctrl 键
   */
  ctrl?: boolean;
  /**
   * 是否按下 Shift 键
   */
  shift?: boolean;
  /**
   * 是否按下 Alt 键
   */
  alt?: boolean;
  /**
   * 是否按下 Meta 键 (Windows 键或 Command 键)
   */
  meta?: boolean;
}

/**
 * 全局注册项定义
 */
interface RegisteredHotkey {
  id: string;
  combinations: HotkeyCombination[];
  handler: (e: KeyboardEvent) => void;
  allowInInputs: boolean;
  preventDefault: boolean;
  stopPropagation: boolean;
  enabled?:
    | MaybeRefOrGetter<boolean>
    | ((ctx: { topmostScope: string | undefined; activeScopes: string[] }) => boolean);
  global: boolean;
  getScope: () => string | undefined;
}

/**
 * 活跃快捷键条目 (供帮助列表使用)
 */
export interface ActiveHotkey {
  id: string;
  keys: string[][];
  description: string;
  category?: string;
  enabled?:
    | MaybeRefOrGetter<boolean>
    | ((ctx: { topmostScope: string | undefined; activeScopes: string[] }) => boolean);
}

// 依赖注入标识
export const HotkeyScopeKey = Symbol("HotkeyScope");

// 全局活跃的作用域栈
export const activeScopes = ref<string[]>([]);

// 全局注册的快捷键列表，按注册顺序排列，后注册的在数组末尾，优先级更高
const registeredHotkeys: RegisteredHotkey[] = [];

// 当前活跃注册的快捷键响应式列表
export const activeHotkeys = shallowRef<ActiveHotkey[]>([]);

/**
 * 解析 "ctrl+shift+1" 格式的快捷键字符串为 HotkeyCombination 对象
 */
function parseHotkey(shortcut: string): HotkeyCombination {
  const parts = shortcut.toLowerCase().split("+");
  const result: HotkeyCombination = { key: "" };

  // 特殊处理末尾的 '+' 键本身
  if (parts.length > 1 && parts[parts.length - 1] === "") {
    parts.splice(-2, 2, "+");
  }

  for (const part of parts) {
    if (part === "ctrl" || part === "control") {
      result.ctrl = true;
    } else if (part === "shift") {
      result.shift = true;
    } else if (part === "alt") {
      result.alt = true;
    } else if (part === "meta" || part === "win" || part === "cmd") {
      result.meta = true;
    } else {
      result.key = part;
    }
  }
  return result;
}

/**
 * 全局按键分发处理器，实现作用域匹配和事件隔离
 */
function globalKeydownHandler(e: KeyboardEvent) {
  const topmostScope =
    activeScopes.value.length > 0 ? activeScopes.value[activeScopes.value.length - 1] : undefined;

  for (let i = registeredHotkeys.length - 1; i >= 0; i--) {
    const hotkey = registeredHotkeys[i];

    // 1. 判断是否被启用
    let isEnabled: boolean;
    if (typeof hotkey.enabled === "function") {
      if (hotkey.enabled.length > 0) {
        isEnabled = hotkey.enabled({
          topmostScope,
          activeScopes: activeScopes.value,
        });
      } else {
        isEnabled = !!(hotkey.enabled as () => boolean)();
      }
    } else if (hotkey.enabled !== undefined) {
      isEnabled = !!toValue(hotkey.enabled);
    } else {
      isEnabled = true;
    }

    if (!isEnabled) {
      continue;
    }

    // 2. 检查 Scope。如果 enabled 是 context 过滤函数且带参数，我们绕过默认的 scope 匹配规则
    const isContextFn = typeof hotkey.enabled === "function" && hotkey.enabled.length > 0;
    if (!isContextFn) {
      const hotkeyScope = hotkey.getScope();
      if (topmostScope !== undefined) {
        if (!hotkey.global && hotkeyScope !== topmostScope) {
          continue;
        }
      } else {
        if (!hotkey.global && hotkeyScope !== undefined) {
          continue;
        }
      }
    }

    // 3. 检查输入框聚焦过滤
    if (!hotkey.allowInInputs) {
      if (
        e.target instanceof HTMLInputElement ||
        e.target instanceof HTMLTextAreaElement ||
        (e.target instanceof HTMLElement && e.target.isContentEditable)
      ) {
        continue;
      }
    }

    // 4. 匹配按键组合
    let matched = false;
    for (const combination of hotkey.combinations) {
      const matchesCtrl = !!e.ctrlKey === !!combination.ctrl;
      const matchesShift = !!e.shiftKey === !!combination.shift;
      const matchesAlt = !!e.altKey === !!combination.alt;
      const matchesMeta = !!e.metaKey === !!combination.meta;

      let matchesKey = e.key.toLowerCase() === combination.key.toLowerCase();

      // 针对数字键的特殊兼容
      if (!matchesKey && /^[0-9]$/.test(combination.key)) {
        matchesKey = e.code === `Digit${combination.key}` || e.code === `Numpad${combination.key}`;
      }

      // 针对物理键码的前缀精确匹配
      if (
        !matchesKey &&
        (combination.key.toLowerCase().startsWith("numpad") ||
          combination.key.toLowerCase().startsWith("digit"))
      ) {
        matchesKey = e.code.toLowerCase() === combination.key.toLowerCase();
      }

      if (matchesCtrl && matchesShift && matchesAlt && matchesMeta && matchesKey) {
        matched = true;
        break;
      }
    }

    if (matched) {
      if (hotkey.preventDefault) {
        e.preventDefault();
      }
      if (hotkey.stopPropagation) {
        e.stopPropagation();
      }
      hotkey.handler(e);

      if (hotkey.stopPropagation) {
        break;
      }
    }
  }
}

// 全局绑定键盘监听
if (typeof window !== "undefined") {
  window.addEventListener("keydown", globalKeydownHandler);
}

/**
 * 内部快捷键注册方法
 */
function registerSingleHotkey(
  id: string,
  keys: string | HotkeyCombination | (string | HotkeyCombination)[],
  handler: (e: KeyboardEvent) => void,
  options: HotkeyOptions = {},
) {
  const {
    allowInInputs = false,
    preventDefault = true,
    stopPropagation = true,
    category,
    enabled,
    global = false,
    scope,
  } = options;

  const combinations = (Array.isArray(keys) ? keys : [keys]).map((k) => {
    if (typeof k === "string") {
      return parseHotkey(k);
    }
    return k;
  });

  const newHotkey: RegisteredHotkey = {
    id,
    combinations,
    handler,
    allowInInputs,
    preventDefault,
    stopPropagation,
    enabled,
    global,
    getScope: () => (scope !== undefined ? toValue(scope) : undefined),
  };
  registeredHotkeys.push(newHotkey);

  // 收集快捷键配置以展示到帮助列表中
  const description = options.description;
  if (description) {
    const parseCombinationToKeys = (comb: HotkeyCombination): string[] => {
      const parts: string[] = [];
      if (comb.ctrl) parts.push("Ctrl");
      if (comb.shift) parts.push("Shift");
      if (comb.alt) parts.push("Alt");
      if (comb.meta) parts.push("Meta");

      const keyName = comb.key.toLowerCase();
      if (keyName === "arrowup") parts.push("↑");
      else if (keyName === "arrowdown") parts.push("↓");
      else if (keyName === "arrowleft") parts.push("←");
      else if (keyName === "arrowright") parts.push("→");
      else if (keyName.startsWith("numpad")) {
        parts.push("Num " + keyName.slice(6));
      } else if (keyName.startsWith("digit")) {
        parts.push(keyName.slice(5));
      } else {
        parts.push(comb.key.toUpperCase());
      }

      return parts;
    };

    const keysList = combinations.map(parseCombinationToKeys);

    activeHotkeys.value = [
      ...activeHotkeys.value,
      {
        id,
        keys: keysList,
        description,
        category,
        enabled,
      },
    ];
  }

  const instance = getCurrentInstance();
  if (instance) {
    onUnmounted(() => {
      const index = registeredHotkeys.findIndex((item) => item.id === id);
      if (index !== -1) {
        registeredHotkeys.splice(index, 1);
      }
      if (description) {
        activeHotkeys.value = activeHotkeys.value.filter((item) => item.id !== id);
      }
    });
  }
}

export interface HotkeyBinding {
  keys: string | HotkeyCombination | (string | HotkeyCombination)[];
  handler: (e: KeyboardEvent) => void;
  options?: Omit<HotkeyOptions, "scope" | "category">;
}

// 重载定义 1：仅定义 Scope
export function useHotkeys(
  options: HotkeyOptions & {
    defineScope: MaybeRefOrGetter<string | undefined>;
  },
): Ref<string | undefined>;

// 重载定义 2：注册多个快捷键，并可选定义 Scope
export function useHotkeys(
  bindings: HotkeyBinding[] | Record<string, (e: KeyboardEvent) => void>,
  options?: HotkeyOptions & {
    defineScope?: MaybeRefOrGetter<string | undefined>;
  },
): Ref<string | undefined>;

/**
 * 快捷键系统统一入口 Composable
 */
export function useHotkeys(
  bindingsOrOptions:
    | HotkeyBinding[]
    | Record<string, (e: KeyboardEvent) => void>
    | (HotkeyOptions & { defineScope: MaybeRefOrGetter<string | undefined> }),
  optionsOrUndefined?: HotkeyOptions & {
    defineScope?: MaybeRefOrGetter<string | undefined>;
  },
): Ref<string | undefined> {
  let bindings: HotkeyBinding[] | Record<string, (e: KeyboardEvent) => void> | null = null;
  let options: HotkeyOptions & {
    defineScope?: MaybeRefOrGetter<string | undefined>;
  };

  if (
    bindingsOrOptions &&
    typeof bindingsOrOptions === "object" &&
    !Array.isArray(bindingsOrOptions) &&
    !("keys" in bindingsOrOptions) &&
    "defineScope" in (bindingsOrOptions as Record<string, unknown>)
  ) {
    options = bindingsOrOptions;
  } else {
    bindings = bindingsOrOptions as HotkeyBinding[] | Record<string, (e: KeyboardEvent) => void>;
    options = optionsOrUndefined || {};
  }

  const { defineScope, ...hotkeyOptions } = options;

  const localScopeId = ref<string | undefined>(undefined);

  // 1. 如果指定了 defineScope，注册并按响应式的值触发 Scope 的压栈/退栈
  if (defineScope !== undefined) {
    const computedScope = computed(() => toValue(defineScope));

    provide(HotkeyScopeKey, computedScope);

    watch(
      computedScope,
      (newVal, oldVal) => {
        if (oldVal) {
          activeScopes.value = activeScopes.value.filter((id) => id !== oldVal);
        }
        if (newVal) {
          if (!activeScopes.value.includes(newVal)) {
            activeScopes.value = [...activeScopes.value, newVal];
          }
        }
        localScopeId.value = newVal;
      },
      { immediate: true },
    );

    const instance = getCurrentInstance();
    if (instance) {
      onUnmounted(() => {
        const val = computedScope.value;
        if (val) {
          activeScopes.value = activeScopes.value.filter((id) => id !== val);
        }
      });
    }
  }

  const injectedScope = inject(HotkeyScopeKey, undefined);

  // 2. 注册快捷键
  if (bindings) {
    const items = Array.isArray(bindings)
      ? bindings
      : Object.entries(bindings).map(([keys, handler]) => ({
          keys,
          handler,
        }));

    for (const item of items) {
      const hotkeyScope = computed(() => {
        if (hotkeyOptions.global) return undefined;
        if (defineScope !== undefined) return localScopeId.value;
        if (hotkeyOptions.scope !== undefined) return toValue(hotkeyOptions.scope);
        return toValue(injectedScope);
      });

      const hotkeyId = useId();
      const options = "options" in item ? item.options : undefined;
      registerSingleHotkey(hotkeyId, item.keys, item.handler, {
        ...hotkeyOptions,
        ...options,
        scope: hotkeyScope,
      });
    }
  }

  return localScopeId;
}
