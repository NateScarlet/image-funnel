import useEventListeners from "./useEventListeners";

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
   * 是否阻止事件冒泡
   * @default true
   */
  stopPropagation?: boolean;
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
 * 解析 "ctrl+shift+1" 格式的快捷键字符串为 HotkeyCombination 对象
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
  } = options;

  // 将传入的各种快捷键格式统一转换并标准化为组合对象数组
  const combinations = (Array.isArray(keys) ? keys : [keys]).map((k) => {
    if (typeof k === "string") {
      return parseHotkey(k);
    }
    return k;
  });

  // 使用通用的事件监听 composable，支持生命周期自动清理
  useEventListeners(window, ({ on }) => {
    on("keydown", (e) => {
      // 默认跳过可输入区域，防止干扰用户的正常打字输入
      if (!allowInInputs) {
        if (
          e.target instanceof HTMLInputElement ||
          e.target instanceof HTMLTextAreaElement ||
          (e.target instanceof HTMLElement && e.target.isContentEditable)
        ) {
          return;
        }
      }

      // 遍历所有可能的组合，判断当前按键是否匹配
      for (const combination of combinations) {
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

        if (
          matchesCtrl &&
          matchesShift &&
          matchesAlt &&
          matchesMeta &&
          matchesKey
        ) {
          if (preventDefault) {
            e.preventDefault();
          }
          if (stopPropagation) {
            e.stopPropagation();
          }
          handler(e);
          break;
        }
      }
    });
  });
}
