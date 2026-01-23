import { computed, ref, toValue, type MaybeRefOrGetter } from "vue";
import type { SessionFragment } from "../graphql/generated";
import { usePresets } from "./usePresets";

export function useSessionConfig(
  initialSession?: MaybeRefOrGetter<SessionFragment | undefined | null>,
) {
  const { presets, getPreset, lastSelectedPresetId } = usePresets();

  const session = computed(() => toValue(initialSession));

  const selectedPresetIdBuffer = ref<string>();
  const targetKeepBuffer = ref<number>();
  const ratingBuffer = ref<number[]>();

  const selectedPresetId = computed({
    get() {
      // If buffers have values (meaning user modified them), treat as custom (empty string)
      if (targetKeepBuffer.value != null || ratingBuffer.value != null) {
        return "";
      }

      // If we have an explicit preset selected (this buffer is set when user clicks a preset)
      if (selectedPresetIdBuffer.value != null) {
        return selectedPresetIdBuffer.value;
      }

      // If we are editing an existing session (initialSession provided), default to custom
      if (session.value) {
        return "";
      }

      const defaultId = presets.value[0]?.id ?? "";
      return lastSelectedPresetId.value ?? defaultId;
    },
    set(v: string) {
      // Clear buffers when switching presets
      targetKeepBuffer.value = undefined;
      ratingBuffer.value = undefined;
      selectedPresetIdBuffer.value = v;

      // Update last selected preset (including "custom")
      lastSelectedPresetId.value = v;
    },
  });

  const selectedPreset = computed(() => getPreset(selectedPresetId.value));

  const targetKeep = computed({
    get: () =>
      targetKeepBuffer.value ??
      selectedPreset.value?.targetKeep ??
      session.value?.targetKeep ??
      presets.value[0]?.targetKeep ??
      0,
    set: (v) => {
      targetKeepBuffer.value = v;
    },
  });

  const rating = computed({
    get: () =>
      ratingBuffer.value ??
      selectedPreset.value?.filter.rating ??
      session.value?.filter.rating ??
      presets.value[0]?.filter.rating ??
      [],
    set: (v) => {
      ratingBuffer.value = v;
    },
  });

  return {
    presets,
    selectedPresetId,
    selectedPreset,
    targetKeep,
    rating,
  };
}
