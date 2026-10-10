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
  /**
   * 声明式快捷键的声明标识（如钩子 id）。声明式快捷键由用户自由填写，
   * 可能与应用自身快捷键或其它声明撞键。
   * 撞键时采取可见降级：不注册 handler（按下既不派发也不拦截事件），
   * 但仍以 `invalid: "conflict"` 出现在快捷键说明列表中。
   * 判定按逐键粒度进行，同一声明中的某个键冲突不影响其余键。
   *
   * 同一标识的多个注册项（如同一个钩子绑定到查看器与批量操作栏两个上下文）
   * 属于同一声明，彼此不算撞键，由 scope 决定当下哪个上下文生效。
   */
  declared?: string;
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
  /** 声明标识，undefined 表示应用自身快捷键 */
  declared?: string;
  /** 是否已登记到帮助列表，用于决定失效状态是否需要同步到列表 */
  listedInHelp: boolean;
  invalid?: HotkeyInvalidReason;
}

/**
 * 快捷键失效原因
 */
export type HotkeyInvalidReason = "conflict";

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
  /**
   * 失效原因。非空表示该条快捷键已被声明但当前不可用（详见 ADR 0008），
   * 此时不注册 handler，仅在帮助列表中以错误样式提示。
   */
  invalid?: HotkeyInvalidReason;
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
 * 组合键的规范化标识，用于比对两个组合键在语义上是否等价
 * （如 "F5" 与 "f5"、"ctrl+k" 与 "control+K" 视为同一个组合键）
 */
function combinationKey(comb: HotkeyCombination): string {
  const modifiers = `${comb.ctrl ? "ctrl+" : ""}${comb.shift ? "shift+" : ""}${comb.alt ? "alt+" : ""}${comb.meta ? "meta+" : ""}`;
  return `${modifiers}${comb.key.toLowerCase()}`;
}

/**
 * 全局按键分发处理器，实现作用域匹配和事件隔离
 */
function globalKeydownHandler(e: KeyboardEvent) {
  const topmostScope =
    activeScopes.value.length > 0 ? activeScopes.value[activeScopes.value.length - 1] : undefined;

  for (let i = registeredHotkeys.length - 1; i >= 0; i--) {
    const hotkey = registeredHotkeys[i];

    // 0. 声明式快捷键撞键后不参与分发，按下既不派发也不拦截事件
    if (hotkey.invalid !== undefined) {
      continue;
    }

    // 1. 判断是否被启用
    let isEnabled: boolean;
    if (typeof hotkey.enabled === "function") {
      if (hotkey.enabled.length > 0) {
        isEnabled = hotkey.enabled({
          topmostScope,
          activeScopes: activeScopes.value,
        });
      } else {
        isEnabled = (hotkey.enabled as () => boolean)();
      }
    } else if (hotkey.enabled !== undefined) {
      isEnabled = toValue(hotkey.enabled);
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
      const matchesCtrl = e.ctrlKey === !!combination.ctrl;
      const matchesShift = e.shiftKey === !!combination.shift;
      const matchesAlt = e.altKey === !!combination.alt;
      const matchesMeta = e.metaKey === !!combination.meta;

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

function parseCombinationToKeys(comb: HotkeyCombination): string[] {
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
}

/**
 * 同步某个注册项的失效状态到帮助列表
 */
function setHotkeyInvalid(hotkey: RegisteredHotkey, invalid: HotkeyInvalidReason | undefined) {
  if (hotkey.invalid === invalid) return;
  hotkey.invalid = invalid;
  if (!hotkey.listedInHelp) return;
  activeHotkeys.value = activeHotkeys.value.map((item) =>
    item.id === hotkey.id ? { ...item, invalid } : item,
  );
}

/**
 * 重新判定所有声明式快捷键的冲突状态。
 *
 * 冲突的钩子键采取可见降级（见 ADR 0008）：不注册 handler，但仍出现在快捷键说明列表中。
 * 判定按逐键粒度进行；两个声明占用同一个组合键时双方都失效，不存在先到先得的赢家。
 * 每次注册或注销后都需整体重算，避免残留的失效状态无法随声明变化恢复。
 */
function resolveDeclaredConflicts() {
  if (!registeredHotkeys.some((item) => item.declared !== undefined)) return;

  // 组合键 -> 占用它的全部注册项
  const claims = new Map<string, RegisteredHotkey[]>();
  for (const hotkey of registeredHotkeys) {
    for (const combination of hotkey.combinations) {
      const key = combinationKey(combination);
      const claim = claims.get(key);
      if (claim) {
        claim.push(hotkey);
      } else {
        claims.set(key, [hotkey]);
      }
    }
  }

  for (const hotkey of registeredHotkeys) {
    if (hotkey.declared === undefined) continue;
    const conflicted = hotkey.combinations.some((combination) =>
      (claims.get(combinationKey(combination)) ?? []).some(
        // 同一声明标识的多个注册项属于同一声明，彼此不算撞键
        (other) => other.id !== hotkey.id && other.declared !== hotkey.declared,
      ),
    );
    setHotkeyInvalid(hotkey, conflicted ? "conflict" : undefined);
  }
}

/**
 * 内部快捷键注册方法
 */
function registerSingleHotkey(
  id: string,
  keys: HotkeyBinding["keys"],
  handler: (e: KeyboardEvent) => void,
  options: HotkeyOptions = {},
): void {
  const {
    allowInInputs = false,
    preventDefault = true,
    stopPropagation = true,
    category,
    enabled,
    global = false,
    scope,
    declared,
  } = options;

  const combinations = toCombinations(keys);

  const description = options.description;
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
    declared,
    listedInHelp: !!description,
  };
  registeredHotkeys.push(newHotkey);

  // 收集快捷键配置以展示到帮助列表中
  if (description) {
    activeHotkeys.value = [
      ...activeHotkeys.value,
      {
        id,
        keys: combinations.map(parseCombinationToKeys),
        description,
        category,
        enabled,
      },
    ];
  }

  resolveDeclaredConflicts();
}

/**
 * 内部快捷键注销方法
 */
function unregisterSingleHotkey(id: string) {
  const index = registeredHotkeys.findIndex((item) => item.id === id);
  if (index === -1) return;
  registeredHotkeys.splice(index, 1);
  activeHotkeys.value = activeHotkeys.value.filter((item) => item.id !== id);
  resolveDeclaredConflicts();
}

export interface HotkeyBinding {
  keys: string | HotkeyCombination | (string | HotkeyCombination)[];
  handler: (e: KeyboardEvent) => void;
  options?: Omit<HotkeyOptions, "scope" | "category">;
}

/**
 * 快捷键绑定集合
 */
export type HotkeyBindings = HotkeyBinding[] | Record<string, (e: KeyboardEvent) => void>;

/**
 * 把绑定输入统一成数组形式
 */
function normalizeBindings(bindings: HotkeyBindings): HotkeyBinding[] {
  if (Array.isArray(bindings)) return bindings;
  return Object.entries(bindings).map(([keys, handler]) => ({ keys, handler }));
}

/**
 * 把组合键输入统一成组合键数组
 */
function toCombinations(keys: HotkeyBinding["keys"]): HotkeyCombination[] {
  return (Array.isArray(keys) ? keys : [keys]).map((key) =>
    typeof key === "string" ? parseHotkey(key) : key,
  );
}

// 重载定义 1：仅定义 Scope
export function useHotkeys(
  options: HotkeyOptions & {
    defineScope: MaybeRefOrGetter<string | undefined>;
  },
): Ref<string | undefined>;

// 重载定义 2：注册多个快捷键，并可选定义 Scope
export function useHotkeys(
  bindings: MaybeRefOrGetter<HotkeyBindings>,
  options?: HotkeyOptions & {
    defineScope?: MaybeRefOrGetter<string | undefined>;
  },
): Ref<string | undefined>;

/**
 * 快捷键系统统一入口 Composable
 */
export function useHotkeys(
  bindingsOrOptions:
    | MaybeRefOrGetter<HotkeyBindings>
    | (HotkeyOptions & { defineScope: MaybeRefOrGetter<string | undefined> }),
  optionsOrUndefined?: HotkeyOptions & {
    defineScope?: MaybeRefOrGetter<string | undefined>;
  },
): Ref<string | undefined> {
  let bindings: MaybeRefOrGetter<HotkeyBindings> | null = null;
  let resolvedOptions: HotkeyOptions & {
    defineScope?: MaybeRefOrGetter<string | undefined>;
  };

  if (
    bindingsOrOptions &&
    typeof bindingsOrOptions === "object" &&
    !Array.isArray(bindingsOrOptions) &&
    !("keys" in bindingsOrOptions) &&
    "defineScope" in (bindingsOrOptions as unknown as Record<string, unknown>)
  ) {
    resolvedOptions = bindingsOrOptions as HotkeyOptions & {
      defineScope: MaybeRefOrGetter<string | undefined>;
    };
  } else {
    bindings = bindingsOrOptions as unknown as MaybeRefOrGetter<HotkeyBindings>;
    resolvedOptions = optionsOrUndefined || {};
  }

  const { defineScope, ...hotkeyOptions } = resolvedOptions;

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
    const source = bindings;
    const baseId = useId();
    const hotkeyScope = computed(() => {
      if (hotkeyOptions.global) return undefined;
      if (defineScope !== undefined) return localScopeId.value;
      if (hotkeyOptions.scope !== undefined) return toValue(hotkeyOptions.scope);
      return toValue(injectedScope);
    });

    // 本次调用已注册的快捷键 id，用于重新注册与卸载时清理
    let registeredIds: string[] = [];

    // 绑定可能来自异步数据（如钩子配置经 GraphQL 加载），变化时整体重注册，
    // 冲突判定也因此始终基于当前完整的声明集合
    function syncRegistrations(items: HotkeyBinding[]) {
      for (const id of registeredIds) {
        unregisterSingleHotkey(id);
      }
      registeredIds = [];

      items.forEach((item, index) => {
        const bindingOptions = "options" in item ? item.options : undefined;
        const options: HotkeyOptions = { ...hotkeyOptions, ...bindingOptions, scope: hotkeyScope };
        const bindingId = `${baseId}-${index}`;

        if (options.declared === undefined) {
          registerSingleHotkey(bindingId, item.keys, item.handler, options);
          registeredIds.push(bindingId);
          return;
        }

        // 声明式快捷键逐键生效：同一声明中的重复键只算一次，某个键冲突不影响其余键
        const declaredKeys = new Set<string>();
        for (const combination of toCombinations(item.keys)) {
          const key = combinationKey(combination);
          if (declaredKeys.has(key)) continue;
          declaredKeys.add(key);
          const declaredId = `${bindingId}-${declaredKeys.size}`;
          registerSingleHotkey(declaredId, combination, item.handler, options);
          registeredIds.push(declaredId);
        }
      });
    }

    watch(() => normalizeBindings(toValue(source)), syncRegistrations, { immediate: true });

    if (getCurrentInstance()) {
      onUnmounted(() => {
        for (const id of registeredIds) {
          unregisterSingleHotkey(id);
        }
      });
    }
  }

  return localScopeId;
}
