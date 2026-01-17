<template>
  <div class="min-h-screen bg-slate-900 text-slate-100 p-4 md:p-8">
    <div class="max-w-4xl mx-auto">
      <header class="mb-8">
        <h1 class="text-3xl md:text-4xl font-bold text-center mb-2">ImageFunnel</h1>
        <p class="text-slate-400 text-center">AI生成图片筛选工具</p>
      </header>

      <div class="bg-slate-800 rounded-lg p-6 mb-6">
        <h2 class="text-xl font-semibold mb-4">创建新会话</h2>

        <div class="space-y-6">
          <div>
            <label class="block text-sm font-medium text-slate-300 mb-4">
              选择评分预设
            </label>
            <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
              <div
                v-for="preset in presets"
                :key="preset.id"
                :class="[
                  'p-4 rounded-lg cursor-pointer transition-all border-2',
                  selectedPresetId === preset.id
                    ? 'bg-blue-600 border-blue-500 shadow-lg shadow-blue-500/30'
                    : 'bg-slate-700 border-slate-600 hover:border-slate-500 hover:bg-slate-650'
                ]"
                @click="selectedPresetId = preset.id"
              >
                <h3 class="font-semibold text-lg mb-2">{{ preset.name }}</h3>
                <p class="text-sm opacity-80 mb-3">{{ preset.description }}</p>
                <div class="grid grid-cols-2 gap-2 text-xs">
                  <div>
                    <span class="opacity-70">队列评分:</span> {{ preset.queueRating }}
                  </div>
                  <div>
                    <span class="opacity-70">保留评分:</span> {{ preset.keepRating }}
                  </div>
                  <div>
                    <span class="opacity-70">稍后再看:</span> {{ preset.reviewRating }}
                  </div>
                  <div>
                    <span class="opacity-70">排除评分:</span> {{ preset.rejectRating }}
                  </div>
                </div>
              </div>
            </div>
          </div>

          <div v-if="selectedPreset" class="bg-slate-700 rounded-lg p-4">
            <h3 class="font-medium mb-2">已选择预设详情</h3>
            <div class="grid grid-cols-2 gap-2 text-sm">
              <div>
                <span class="text-slate-400">队列评分:</span> {{ selectedPreset.queueRating }}
              </div>
              <div>
                <span class="text-slate-400">保留评分:</span> {{ selectedPreset.keepRating }}
              </div>
              <div>
                <span class="text-slate-400">稍后再看:</span> {{ selectedPreset.reviewRating }}
              </div>
              <div>
                <span class="text-slate-400">排除评分:</span> {{ selectedPreset.rejectRating }}
              </div>
            </div>
          </div>

          <div>
            <label class="block text-sm font-medium text-slate-300 mb-2">
              保留目标数量
            </label>
            <input
              v-model.number="targetKeep"
              type="number"
              min="1"
              max="100"
              class="w-full px-4 py-2 bg-slate-700 border border-slate-600 rounded-lg focus:outline-none focus:ring-2 focus:ring-blue-500 text-white"
            />
          </div>

          <button
            :disabled="!canCreate"
            class="w-full py-3 px-6 bg-blue-600 hover:bg-blue-700 disabled:bg-slate-600 disabled:cursor-not-allowed rounded-lg font-medium transition-colors"
            @click="createSession"
          >
            开始筛选
          </button>
        </div>
      </div>

      <div v-if="error" class="bg-red-900 border border-red-700 rounded-lg p-4 mb-6">
        <p class="text-red-200">{{ error }}</p>
      </div>

      <div v-if="loading" class="text-center text-slate-400">
        加载中...
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { useRouter } from 'vue-router'
import useQuery from '../graphql/utils/useQuery'
import mutate from '../graphql/utils/mutate'
import { GetPresetsDocument, CreateSessionDocument } from '../graphql/generated'

const router = useRouter()

const loadingCount = ref(0)
const loading = computed(() => loadingCount.value > 0 || creatingSession.value)
const creatingSession = ref(false)
const error = ref<string>('')

const { data: presetsData } = useQuery(GetPresetsDocument, {
  loadingCount,
})

const presets = computed(() => presetsData.value?.presets || [])
const selectedPresetId = ref<string>('')
const targetKeep = ref<number>(10)

const selectedPreset = computed(() => {
  return presets.value.find(p => p.id === selectedPresetId.value)
})

const canCreate = computed(() => {
  return selectedPresetId.value && targetKeep.value > 0
})

watch(presets, (newPresets) => {
  if (newPresets.length > 0) {
    const lastPresetId = localStorage.getItem('lastSelectedPresetId')
    if (lastPresetId && newPresets.find(p => p.id === lastPresetId)) {
      selectedPresetId.value = lastPresetId
    } else {
      selectedPresetId.value = newPresets[0].id
    }
  }
}, { immediate: true })

async function createSession() {
  creatingSession.value = true
  error.value = ''

  try {
    const { data } = await mutate(CreateSessionDocument, {
      variables: {
        input: {
          presetId: selectedPresetId.value,
          targetKeep: targetKeep.value
        }
      }
    })

    localStorage.setItem('lastSelectedPresetId', selectedPresetId.value)
    if (data?.createSession) {
      router.push(`/session/${data.createSession.session.id}`)
    }
  } catch (err: unknown) {
    error.value = '创建会话失败: ' + (err instanceof Error ? err.message : 'Unknown error')
  } finally {
    creatingSession.value = false
  }
}
</script>
