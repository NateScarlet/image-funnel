import {
  shallowRef,
  onUnmounted,
  toValue,
  getCurrentInstance,
  type Ref,
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
   * 是否启用当前快捷键，支持响应式更新
   * @default true
   */
  enabled?: boolean | Ref<boolean> | (() => boolean);
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
  enabled?: boolean | Ref<boolean> | (() => boolean);
}

// 全局注册的快捷键列表，按注册顺序排列，后注册的在数组末尾，优先级更高
const registeredHotkeys: RegisteredHotkey[] = [];

/**
 * 解析 "ctrl+shift+1" 格式 of 快捷键字符串为 HotkeyCombination 对象
 * @param shortcut 快捷键字符串
 */
function parseHotkey(shortcut: string): HotkeyCombination {
  const parts = shortcut.toLowerCase().split("+");
  const result: HotkeyCombination = { key: "" };

  // 特殊处理末尾的 '+' 键本身（如 "ctrl++" 拆分后最后一个元素是空字符串）
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
 * 全局的键盘按下事件处理器，负责按照注册顺序从后往前分发，并处理阻断逻辑
 */
function globalKeydownHandler(e: KeyboardEvent) {
  for (let i = registeredHotkeys.length - 1; i >= 0; i--) {
    const hotkey = registeredHotkeys[i];

    // 1. 判断是否被禁用
    if (hotkey.enabled !== undefined && !toValue(hotkey.enabled)) {
      continue;
    }

    // 2. 检查输入框聚焦过滤
    if (!hotkey.allowInInputs) {
      if (
        e.target instanceof HTMLInputElement ||
        e.target instanceof HTMLTextAreaElement ||
        (e.target instanceof HTMLElement && e.target.isContentEditable)
      ) {
        continue;
      }
    }

    // 3. 匹配按键组合
    let matched = false;
    for (const combination of hotkey.combinations) {
      const matchesCtrl = !!e.ctrlKey === !!combination.ctrl;
      const matchesShift = !!e.shiftKey === !!combination.shift;
      const matchesAlt = !!e.altKey === !!combination.alt;
      const matchesMeta = !!e.metaKey === !!combination.meta;

      let matchesKey = e.key.toLowerCase() === combination.key.toLowerCase();

      // 针对数字键的特殊兼容：当配合 Shift 按下数字键时，e.key 会变成其对应的字符（例如 Shift+1 变成 !）
      // 这种情况下，我们需要借助物理键码 e.code (如 Digit1, Numpad1) 来做辅助匹配
      if (!matchesKey && /^[0-9]$/.test(combination.key)) {
        matchesKey =
          e.code === `Digit${combination.key}` ||
          e.code === `Numpad${combination.key}`;
      }

      // 新增：针对物理键码的前缀精确匹配（支持 digit0-digit9, numpad0-numpad9 显式指定）
      if (
        !matchesKey &&
        (combination.key.toLowerCase().startsWith("numpad") ||
          combination.key.toLowerCase().startsWith("digit"))
      ) {
        matchesKey = e.code.toLowerCase() === combination.key.toLowerCase();
      }

      if (
        matchesCtrl &&
        matchesShift &&
        matchesAlt &&
        matchesMeta &&
        matchesKey
      ) {
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

      // 若设置了阻断事件，则直接结束循环，不触发其他快捷键
      if (hotkey.stopPropagation) {
        break;
      }
    }
  }
}

// 在全局绑定唯一的键盘事件监听器
if (typeof window !== "undefined") {
  window.addEventListener("keydown", globalKeydownHandler);
}

/**
 * 通用快捷键管理 Composable。
 * 当宿主组件被卸载时，绑定的快捷键会自动注销。
 *
 * @param keys 快捷键描述，可以是字符串（如 "ctrl+shift+1"）、组合对象，或者其组成的数组
 * @param handler 快捷键触发时的回调函数
 * @param options 快捷键配置项
 */
export default function useHotkey(
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
  } = options;

  // 将传入的各种快捷键格式统一转换并标准化为组合对象数组
  const combinations = (Array.isArray(keys) ? keys : [keys]).map((k) => {
    if (typeof k === "string") {
      return parseHotkey(k);
    }
    return k;
  });

  const id = Math.random().toString(36).substring(2, 9);

  // 构造注册项并推入全局注册列表
  const newHotkey: RegisteredHotkey = {
    id,
    combinations,
    handler,
    allowInInputs,
    preventDefault,
    stopPropagation,
    enabled,
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

  // 组件卸载时自动注销
  const instance = getCurrentInstance();
  if (instance) {
    onUnmounted(() => {
      // 从全局注册列表中移除
      const index = registeredHotkeys.findIndex((item) => item.id === id);
      if (index !== -1) {
        registeredHotkeys.splice(index, 1);
      }
      // 从帮助列表中移除
      if (description) {
        activeHotkeys.value = activeHotkeys.value.filter(
          (item) => item.id !== id,
        );
      }
    });
  }
}

/**
 * 活跃快捷键条目
 */
export interface ActiveHotkey {
  id: string;
  keys: string[][];
  description: string;
  category?: string;
  enabled?: boolean | Ref<boolean> | (() => boolean);
}

/**
 * 当前活跃注册的快捷键响应式列表
 */
export const activeHotkeys = shallowRef<ActiveHotkey[]>([]);
