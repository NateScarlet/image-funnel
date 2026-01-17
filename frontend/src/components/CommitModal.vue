<template>
  <div class="fixed inset-0 bg-black bg-opacity-75 flex items-center justify-center z-50 p-4">
    <div class="bg-slate-800 rounded-lg max-w-md w-full p-6">
      <h2 class="text-xl font-bold mb-4">提交更改</h2>
      
      <div class="mb-4">
        <p class="text-slate-300 mb-2">
          将 {{ stats?.processed || 0 }} 个操作写入 XMP 文件
        </p>
        <div class="grid grid-cols-3 gap-2 text-sm">
          <div class="bg-green-900 bg-opacity-30 rounded p-2 text-center">
            <div class="text-green-400 font-bold">{{ stats?.kept || 0 }}</div>
            <div class="text-slate-400">保留</div>
          </div>
          <div class="bg-yellow-900 bg-opacity-30 rounded p-2 text-center">
            <div class="text-yellow-400 font-bold">{{ stats?.reviewed || 0 }}</div>
            <div class="text-slate-400">稍后</div>
          </div>
          <div class="bg-red-900 bg-opacity-30 rounded p-2 text-center">
            <div class="text-red-400 font-bold">{{ stats?.rejected || 0 }}</div>
            <div class="text-slate-400">排除</div>
          </div>
        </div>
      </div>
      
      <div v-if="committing" class="text-center mb-4">
        <div class="animate-spin rounded-full h-8 w-8 border-b-2 border-blue-500 mx-auto mb-2"></div>
        <p class="text-slate-400">正在写入...</p>
      </div>
      
      <div v-else-if="commitResult" class="mb-4">
        <div :class="commitResult.success ? 'text-green-400' : 'text-red-400'">
          {{ commitResult.success ? '✓ 提交成功' : '✗ 提交失败' }}
        </div>
        <div class="text-sm text-slate-400">
          写入: {{ commitResult.written }} | 失败: {{ commitResult.failed }}
        </div>
        <div v-if="commitResult.errors.length > 0" class="mt-2 text-sm text-red-300">
          <div v-for="(err, i) in commitResult.errors" :key="i">{{ err }}</div>
        </div>
      </div>
      
      <div class="flex gap-3">
        <button
          :disabled="committing"
          class="flex-1 px-4 py-2 bg-slate-700 hover:bg-slate-600 disabled:bg-slate-800 disabled:cursor-not-allowed rounded-lg"
          @click="$emit('close')"
        >
          取消
        </button>
        <button
          v-if="!commitResult"
          :disabled="committing"
          class="flex-1 px-4 py-2 bg-blue-600 hover:bg-blue-700 disabled:bg-slate-600 disabled:cursor-not-allowed rounded-lg"
          @click="commit"
        >
          确认提交
        </button>
        <button
          v-else
          class="flex-1 px-4 py-2 bg-green-600 hover:bg-green-700 rounded-lg"
          @click="$emit('committed')"
        >
          完成
        </button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import useQuery from '../graphql/utils/useQuery'
import mutate from '../graphql/utils/mutate'
import { GetSessionDocument, CommitChangesDocument } from '../graphql/generated'

interface Props {
  sessionId: string
}

interface CommitResult {
  success: boolean
  written: number
  failed: number
  errors: string[]
}

const props = defineProps<Props>()
defineEmits(['close', 'committed'])

const { data: sessionData } = useQuery(GetSessionDocument, {
  variables: () => ({ id: props.sessionId })
})

const stats = computed(() => sessionData.value?.session?.stats)
const committing = ref(false)
const commitResult = ref<CommitResult | null>(null)

async function commit() {
  committing.value = true
  
  try {
    const { data } = await mutate(CommitChangesDocument, {
      variables: { input: { sessionId: props.sessionId } }
    })
    
    if (data) {
      commitResult.value = data.commitChanges
    }
  } catch (err: unknown) {
    commitResult.value = {
      success: false,
      written: 0,
      failed: 1,
      errors: [err instanceof Error ? err.message : 'Unknown error']
    }
  } finally {
    committing.value = false
  }
}
</script>
