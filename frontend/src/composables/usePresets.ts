import { ref, watch } from "vue";

export interface Preset {
  id: string;
  name: string;
  description: string;
  filter: {
    rating: number[];
  };
  writeActions: {
    keepRating: number;
    pendingRating: number;
    rejectRating: number;
  };
  targetKeep: number;
}

const STORAGE_KEY = "imagefunnel-presets";

const defaultPresets: Preset[] = [
  {
    id: "draft-filter",
    name: "草稿阶段筛选",
    description: "从大量生成结果中快速筛选",
    filter: {
      rating: [0, 4],
    },
    writeActions: {
      keepRating: 4,
      pendingRating: 0,
      rejectRating: 2,
    },
    targetKeep: 4,
  },
  {
    id: "refine-filter",
    name: "细化阶段筛选",
    description: "从待定图片中精细筛选",
    filter: {
      rating: [0],
    },
    writeActions: {
      keepRating: 0,
      pendingRating: 0,
      rejectRating: 1,
    },
    targetKeep: 1,
  },
];

function generateId(): string {
  return `preset-${Date.now()}-${Math.random().toString(36).substr(2, 9)}`;
}

function loadPresets(): Preset[] {
  try {
    const stored = localStorage.getItem(STORAGE_KEY);
    if (stored) {
      return JSON.parse(stored);
    }
  } catch (err) {
    console.error("Failed to load presets:", err);
  }
  return [...defaultPresets];
}

function savePresets(presets: Preset[]): void {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(presets));
  } catch (err) {
    console.error("Failed to save presets:", err);
  }
}

const presets = ref<Preset[]>(loadPresets());

watch(
  presets,
  (newPresets) => {
    savePresets(newPresets);
  },
  { deep: true },
);

export function usePresets() {
  function getPreset(id: string): Preset | undefined {
    return presets.value.find((p) => p.id === id);
  }

  function addPreset(preset: Omit<Preset, "id">): Preset {
    const newPreset: Preset = {
      ...preset,
      id: generateId(),
    };
    presets.value.push(newPreset);
    return newPreset;
  }

  function updatePreset(id: string, updates: Partial<Preset>): Preset | null {
    const index = presets.value.findIndex((p) => p.id === id);
    if (index === -1) return null;

    presets.value[index] = { ...presets.value[index], ...updates };
    return presets.value[index];
  }

  function deletePreset(id: string): boolean {
    const index = presets.value.findIndex((p) => p.id === id);
    if (index === -1) return false;

    presets.value.splice(index, 1);
    return true;
  }

  function resetToDefaults(): void {
    presets.value = [...defaultPresets];
  }

  return {
    presets,
    getPreset,
    addPreset,
    updatePreset,
    deletePreset,
    resetToDefaults,
  };
}
