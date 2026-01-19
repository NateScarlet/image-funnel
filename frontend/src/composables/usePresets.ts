import useStorage from "./useStorage";

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

const presetsStorage = useStorage(
  localStorage,
  "presets@6309f070-f3fd-42a0-85e5-e75d9ff38d6d",
  () => [...defaultPresets],
);
const presets = presetsStorage.model;

const lastSelectedPresetIdStorage = useStorage<string>(
  localStorage,
  "lastPreset@6309f070-f3fd-42a0-85e5-e75d9ff38d6d",
);
const lastSelectedPresetId = lastSelectedPresetIdStorage.model;

export function usePresets() {
  function getPreset(id: string): Preset | undefined {
    return presets.value.find((p) => p.id === id);
  }

  function addPreset(preset: Omit<Preset, "id">): Preset {
    const newPreset: Preset = {
      ...preset,
      id: generateId(),
    };
    presets.value = [...presets.value, newPreset];
    return newPreset;
  }

  function updatePreset(id: string, updates: Partial<Preset>): Preset | null {
    const index = presets.value.findIndex((p) => p.id === id);
    if (index === -1) return null;

    const newPresets = [...presets.value];
    newPresets[index] = { ...newPresets[index], ...updates };
    presets.value = newPresets;
    return newPresets[index];
  }

  function deletePreset(id: string): boolean {
    const index = presets.value.findIndex((p) => p.id === id);
    if (index === -1) return false;

    const newPresets = [...presets.value];
    newPresets.splice(index, 1);
    presets.value = newPresets;
    return true;
  }

  function resetToDefaults(): void {
    presets.value = [...defaultPresets];
  }

  return {
    presets,
    lastSelectedPresetId,
    getPreset,
    addPreset,
    updatePreset,
    deletePreset,
    resetToDefaults,
  };
}
