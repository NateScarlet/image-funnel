<template>
  <div
    class="min-h-screen bg-primary-900 text-primary-100 flex flex-col font-sans"
  >
    <!-- 顶部导航栏 -->
    <header
      class="flex-none bg-primary-900/80 backdrop-blur-md border-b border-primary-700/50 px-4 py-3 sticky top-0 z-30"
    >
      <div class="max-w-[1600px] mx-auto flex items-center gap-3">
        <!-- 路径面包屑与返回上级 -->
        <div class="flex items-center gap-3 overflow-hidden">
          <button
            class="p-2 bg-primary-800 hover:bg-primary-700 rounded-lg border border-primary-700 hover:border-primary-600 transition-all text-primary-300 hover:text-white flex-none flex items-center justify-center"
            title="返回主页"
            @click="navigateToHome"
          >
            <svg class="w-5 h-5" viewBox="0 0 24 24">
              <path :d="mdiHome" fill="currentColor" />
            </svg>
          </button>

          <button
            v-if="currentDirectory && !currentDirectory.root"
            class="p-2 bg-primary-800 hover:bg-primary-700 rounded-lg border border-primary-700 hover:border-primary-600 transition-all text-primary-300 hover:text-white flex-none flex items-center justify-center"
            title="返回上一级"
            @click="goToParent"
          >
            <svg class="w-5 h-5" viewBox="0 0 24 24">
              <path :d="mdiArrowLeft" fill="currentColor" />
            </svg>
          </button>

          <!-- 磨砂面包屑路径 -->
          <div
            class="flex items-center gap-1.5 px-3 py-1.5 bg-black/20 rounded-lg border border-white/5 overflow-hidden text-sm"
          >
            <span class="text-primary-400 flex-none flex items-center gap-1">
              <svg class="w-4 h-4" viewBox="0 0 24 24">
                <path :d="mdiFolderOpen" fill="currentColor" />
              </svg>
              Root
            </span>
            <template v-if="currentDirectory?.relPath">
              <span class="text-primary-600">/</span>
              <span
                class="text-primary-200 font-medium truncate"
                :title="currentDirectory.relPath"
              >
                {{ currentDirectory.relPath }}
              </span>
            </template>
          </div>

          <!-- 上次会话按钮，显示文本与最后更新时间 -->
          <button
            v-if="lastSession"
            class="flex items-center gap-2 px-3 py-1.5 bg-primary-800 hover:bg-primary-700 rounded-lg border border-primary-700 hover:border-primary-600 transition-all text-primary-300 hover:text-white flex-none text-sm font-medium"
            title="返回最近会话"
            @click="goToLastSession"
          >
            <svg class="w-4 h-4" viewBox="0 0 24 24">
              <path :d="mdiHistory" fill="currentColor" />
            </svg>
            <span>上次会话：{{ formatDate(lastSession.updatedAt) }}</span>
          </button>
        </div>
      </div>
    </header>

    <main
      class="flex-1 w-full max-w-[1600px] mx-auto p-4 md:p-6 space-y-6 overflow-x-hidden"
    >
      <!-- 子目录容器区 -->
      <SubdirectoryGrid
        v-if="subDirectories.length > 0"
        :directories="subDirectories"
        :filter-rating="filterRating"
        @navigate="navigateToDir"
      />

      <!-- 笔记列表容器区 -->
      <section
        class="space-y-3 bg-primary-800/30 border border-primary-700/50 rounded-2xl p-4 sm:p-6 backdrop-blur-sm"
      >
        <div
          class="flex items-center justify-between border-b border-primary-700/50 pb-3"
        >
          <h2
            class="text-base font-bold text-primary-200 tracking-wider flex items-center gap-2 select-none"
          >
            <svg class="w-5 h-5 text-secondary-400" viewBox="0 0 24 24">
              <path :d="mdiNoteTextOutline" fill="currentColor" />
            </svg>
            笔记列表 ({{ memos.length }} 个)
          </h2>
          <!-- 显示隐藏笔记切换开关 -->
          <ToggleSwitch v-model="showHiddenMemos" label="显示隐藏笔记" />
        </div>

        <!-- 暂无笔记状态 -->
        <div
          v-if="memos.length === 0"
          class="py-6 text-center text-primary-500 text-sm italic"
        >
          暂无任何笔记
        </div>

        <!-- 笔记列表项 -->
        <div v-else class="space-y-2 max-h-60 overflow-y-auto pr-1">
          <div
            v-for="memoItem in memos"
            :key="memoItem.id"
            class="flex items-center justify-between p-3 rounded-xl bg-primary-800/20 hover:bg-primary-800/60 border border-primary-800/40 hover:border-secondary-500/30 transition-all duration-200 group cursor-pointer"
            @click="editMemo(memoItem)"
          >
            <div class="flex items-center gap-3 min-w-0 flex-1">
              <svg
                class="w-4 h-4 text-primary-400 group-hover:text-secondary-400 transition-colors shrink-0"
                viewBox="0 0 24 24"
              >
                <path :d="mdiNoteTextOutline" fill="currentColor" />
              </svg>
              <!-- 列表显示笔记关联的文件名与正文内容，单行 truncate -->
              <span
                class="text-[10px] text-primary-400 shrink-0 bg-primary-800/60 px-1.5 py-0.5 rounded border border-primary-700/50 font-mono select-none"
              >
                {{ memoItem.title }}
              </span>
              <span
                class="text-sm text-primary-200 group-hover:text-white transition-colors truncate font-medium"
              >
                {{ memoItem.content || "（空白笔记内容，点击编辑）" }}
              </span>
              <!-- 如果是隐藏笔记，显示标记 -->
              <span
                v-if="memoItem.hidden"
                class="px-1.5 py-0.5 text-[10px] bg-red-950/40 border border-red-900/50 text-red-400 rounded-md shrink-0 flex items-center gap-0.5"
                title="此笔记已通过 frontmatter 隐藏"
              >
                <svg class="w-3 h-3" viewBox="0 0 24 24">
                  <path :d="mdiEyeOff" fill="currentColor" />
                </svg>
                已隐藏
              </span>
            </div>
            <div class="flex items-center gap-3 shrink-0">
              <!-- 一键切换隐藏状态按钮 -->
              <button
                class="p-1.5 rounded-lg bg-primary-800/40 hover:bg-primary-700/60 border border-primary-700/50 text-primary-400 hover:text-white transition-all active:scale-95 flex items-center justify-center opacity-0 group-hover:opacity-100"
                :title="memoItem.hidden ? '取消隐藏此笔记' : '隐藏此笔记'"
                @click.stop="toggleMemoHidden(memoItem)"
              >
                <svg class="w-3.5 h-3.5" viewBox="0 0 24 24">
                  <path
                    :d="memoItem.hidden ? mdiEye : mdiEyeOff"
                    fill="currentColor"
                  />
                </svg>
              </button>
              <div
                class="text-xs text-primary-500 opacity-0 group-hover:opacity-100 transition-opacity duration-200 select-none flex items-center gap-1"
              >
                <span>编辑</span>
                <svg class="w-3.5 h-3.5" viewBox="0 0 24 24">
                  <path :d="mdiChevronRight" fill="currentColor" />
                </svg>
              </div>
            </div>
          </div>
        </div>
      </section>

      <!-- 图片网格展示区 -->
      <section
        class="space-y-3 bg-primary-800/30 border border-primary-700/50 rounded-2xl p-4 sm:p-6 backdrop-blur-sm"
      >
        <!-- 图片列表标题与图片专用的筛选过滤条件 -->
        <div
          class="flex flex-col sm:flex-row sm:items-center justify-between gap-3 border-b border-primary-700/50 pb-3"
        >
          <h2
            class="text-base font-bold text-primary-200 tracking-wider flex items-center gap-2 select-none"
          >
            <svg class="w-5 h-5 text-secondary-400" viewBox="0 0 24 24">
              <path :d="mdiImage" fill="currentColor" />
            </svg>
            图片列表 ({{ images.length }} 张)
          </h2>

          <div class="flex flex-wrap items-center gap-3">
            <!-- 搜索输入框 -->
            <div class="relative min-w-48 max-w-64 flex-1 md:flex-none">
              <input
                v-model="searchQuery"
                type="text"
                placeholder="搜索文件名..."
                class="w-full pl-9 pr-4 h-[34px] bg-primary-800 border border-primary-700 rounded-lg text-sm text-white focus:outline-none focus:ring-2 focus:ring-secondary-500/50 focus:border-secondary-500 transition-all"
              />
              <svg
                class="w-4 h-4 text-primary-400 absolute left-3 top-1/2 -translate-y-1/2"
                viewBox="0 0 24 24"
              >
                <path :d="mdiMagnify" fill="currentColor" />
              </svg>
            </div>

            <!-- 评星过滤器 -->
            <RatingFilter v-model="filterRating" />

            <!-- 颜色标签过滤器 -->
            <div
              class="flex items-center gap-1.5 bg-primary-800 border border-primary-700 px-3 h-[34px] rounded-lg overflow-x-auto"
            >
              <span class="text-xs text-primary-400 select-none">标签:</span>
              <div class="flex items-center gap-1">
                <button
                  v-for="(colorHex, colorName) in PRESET_COLORS"
                  :key="colorName"
                  class="w-3.5 h-3.5 rounded-full transition-all border border-white/20 relative"
                  :style="{
                    backgroundColor: colorHex,
                    borderColor: filterLabels.includes(colorName)
                      ? 'white'
                      : undefined,
                  }"
                  :class="[
                    filterLabels.includes(colorName)
                      ? 'scale-115 shadow-[0_0_8px_rgba(255,255,255,0.6)]'
                      : 'opacity-60 hover:opacity-100 hover:scale-110',
                  ]"
                  :title="colorName"
                  @click="toggleLabelFilter(colorName)"
                >
                  <!-- 选中指示点 -->
                  <span
                    v-if="filterLabels.includes(colorName)"
                    class="absolute inset-0.5 rounded-full border border-black/30"
                  ></span>
                </button>
              </div>
            </div>

            <!-- 清空过滤按钮 -->
            <button
              class="px-2.5 h-[34px] text-xs border rounded-lg transition-all flex items-center gap-1"
              :class="[
                hasActiveFilters
                  ? 'bg-red-950/40 hover:bg-red-900/40 border-red-900/50 text-red-300 cursor-pointer'
                  : 'bg-primary-900 border-primary-800 text-primary-500 cursor-not-allowed opacity-40',
              ]"
              :disabled="!hasActiveFilters"
              @click="clearFilters"
            >
              <span>清除</span>
            </button>
          </div>
        </div>

        <!-- 骨架图加载指示，避免布局抖动 -->
        <div
          v-if="loading && images.length === 0"
          class="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-6 xl:grid-cols-8 gap-4 animate-pulse"
        >
          <div
            v-for="n in 16"
            :key="n"
            class="aspect-square bg-primary-800/50 rounded-xl"
          ></div>
        </div>

        <!-- 无图片空状态 -->
        <div
          v-else-if="images.length === 0"
          class="flex flex-col items-center justify-center py-20 text-primary-500 gap-2"
        >
          <svg
            class="w-12 h-12 stroke-[1.5]"
            fill="none"
            stroke="currentColor"
            viewBox="0 0 24 24"
          >
            <path
              stroke-linecap="round"
              stroke-linejoin="round"
              d="M2.25 15.75l5.159-5.159a2.25 2.25 0 013.182 0l5.159 5.159m-1.5-1.5l1.409-1.409a2.25 2.25 0 013.182 0l2.909 2.909m-18 3.75h16.5a1.5 1.5 0 001.5-1.5V6a1.5 1.5 0 00-1.5-1.5H3.75A1.5 1.5 0 002.25 6v12a1.5 1.5 0 001.5 1.5zm10.5-11.25h.008v.008h-.008V8.25zm.375 0a.375.375 0 11-.75 0 .375.375 0 01.75 0z"
            />
          </svg>
          <span class="text-sm">该目录或过滤条件下未找到任何图片</span>
        </div>

        <!-- 网格列表 -->
        <div
          v-else
          class="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-6 xl:grid-cols-8 gap-4"
        >
          <div
            v-for="(img, index) in images"
            :key="img.id"
            class="group relative bg-primary-800/40 hover:bg-primary-800/90 border border-primary-800 hover:border-primary-600/80 rounded-xl overflow-hidden aspect-square cursor-pointer transition-all hover:scale-[1.02] hover:shadow-lg hover:shadow-black/40 flex flex-col justify-between"
            @click="openViewer(index)"
          >
            <!-- 缩略图加载 -->
            <div
              class="w-full h-full relative overflow-hidden bg-black/10 flex items-center justify-center"
            >
              <img
                :src="img.url256 || img.url"
                :alt="img.filename"
                loading="lazy"
                class="object-cover w-full h-full select-none"
              />

              <!-- 评星与标签的悬浮徽章 -->
              <div
                class="absolute bottom-2 left-2 right-2 flex items-center justify-between pointer-events-none opacity-90 group-hover:opacity-100 transition-opacity"
              >
                <!-- 评分图标 -->
                <span
                  v-if="img.currentRating"
                  class="flex items-center gap-0.5 px-1.5 py-0.5 rounded bg-black/70 backdrop-blur-md text-[10px] font-bold text-yellow-400 shadow-md border border-white/5"
                >
                  <svg class="w-3 h-3" viewBox="0 0 24 24" fill="currentColor">
                    <path :d="mdiStar" />
                  </svg>
                  {{ img.currentRating }}
                </span>

                <!-- 颜色标签 -->
                <span
                  v-if="img.label"
                  class="w-3.5 h-3.5 rounded-full shadow-md border border-white/20 ml-auto"
                  :style="{
                    backgroundColor: PRESET_COLORS[img.label] || '#94a3b8',
                  }"
                  :title="img.label"
                ></span>
              </div>
            </div>

            <!-- 卡片底部的文件名遮罩 -->
            <div
              class="absolute inset-x-0 top-0 bg-gradient-to-b from-black/80 to-transparent p-2 opacity-0 group-hover:opacity-100 transition-opacity duration-200 pointer-events-none"
            >
              <p
                class="text-[10px] text-white font-medium truncate"
                :title="img.filename"
              >
                {{ img.filename }}
              </p>
            </div>
          </div>
        </div>
      </section>

      <!-- 懒加载过渡区与加载更多按钮 -->
      <section v-if="hasNextPage" class="flex justify-center pt-4">
        <button
          :disabled="loading"
          class="px-6 py-2.5 bg-primary-800 hover:bg-primary-700 border border-primary-700 hover:border-primary-600 rounded-xl text-sm font-semibold transition-all flex items-center gap-2 text-primary-200 hover:text-white"
          @click="loadMore"
        >
          <!-- 加载中动画 -->
          <svg
            v-if="loading"
            class="w-4 h-4 animate-spin text-secondary-500"
            viewBox="0 0 24 24"
            fill="none"
          >
            <path
              :d="mdiLoading"
              fill="none"
              stroke="currentColor"
              stroke-width="3"
              stroke-linecap="round"
            />
          </svg>
          <span>{{ loading ? "正在加载..." : "加载更多图片" }}</span>
        </button>
      </section>
    </main>

    <!-- 全屏查看器遮罩层 -->
    <Transition
      enter-active-class="transition duration-200 ease-out"
      enter-from-class="opacity-0 scale-95"
      enter-to-class="opacity-100 scale-100"
      leave-active-class="transition duration-150 ease-in"
      leave-from-class="opacity-100 scale-100"
      leave-to-class="opacity-0 scale-95"
    >
      <div
        v-if="currentImageIndex !== undefined && currentImage"
        class="fixed inset-0 z-50 flex items-center justify-center bg-black/95 backdrop-blur-sm select-none"
      >
        <!-- 侧边关闭按钮 -->
        <button
          class="absolute top-4 right-4 z-[60] p-2 rounded-full bg-white/5 hover:bg-white/10 text-white/70 hover:text-white transition-colors border border-white/10"
          title="关闭查看器 (Esc)"
          @click="closeViewer"
        >
          <svg class="w-6 h-6" viewBox="0 0 24 24">
            <path :d="mdiClose" fill="currentColor" />
          </svg>
        </button>

        <!-- 上一张按钮 -->
        <button
          v-if="currentImageIndex > 0"
          class="absolute left-4 top-1/2 -translate-y-1/2 z-[60] p-3 rounded-xl bg-white/5 hover:bg-white/10 hover:scale-105 active:scale-95 text-white/80 hover:text-white transition-all border border-white/10"
          title="上一张图片 (ArrowLeft)"
          @click="prevImage"
        >
          <svg class="w-8 h-8" viewBox="0 0 24 24">
            <path :d="mdiChevronLeft" fill="currentColor" />
          </svg>
        </button>

        <!-- 下一张按钮 -->
        <button
          v-if="currentImageIndex < images.length - 1"
          class="absolute right-4 top-1/2 -translate-y-1/2 z-[60] p-3 rounded-xl bg-white/5 hover:bg-white/10 hover:scale-105 active:scale-95 text-white/80 hover:text-white transition-all border border-white/10"
          title="下一张图片 (ArrowRight)"
          @click="nextImage"
        >
          <svg class="w-8 h-8" viewBox="0 0 24 24">
            <path :d="mdiChevronRight" fill="currentColor" />
          </svg>
        </button>

        <!-- 图像查看器组件 -->
        <div class="w-full h-full flex flex-col justify-between">
          <ImageViewer :image="currentImage" class="w-full h-full flex-1">
            <!-- 插入底部信息 -->
            <template #info>
              <span
                class="truncate max-w-72 font-semibold"
                :title="currentImage.filename"
              >
                {{ currentImage.filename }}
              </span>
              <div class="w-px h-4 bg-white/30 mx-1"></div>
              <span>
                {{ (currentImageIndex || 0) + 1 }} / {{ images.length }}
              </span>
              <div class="w-px h-4 bg-white/30 mx-1"></div>
              <span class="text-white/60">
                {{ currentImage.width || 0 }}x{{ currentImage.height || 0 }}
              </span>
              <div class="w-px h-4 bg-white/30 mx-1"></div>
              <span class="text-white/60">
                {{ formatSize(currentImage.size) }}
              </span>
            </template>
          </ImageViewer>
        </div>
      </div>
    </Transition>

    <!-- 备忘录/笔记编辑对话框 -->
    <MemoEditorDialog
      v-if="selectedMemo"
      v-model="isMemoEditorOpen"
      :memo="selectedMemo"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from "vue";
import { useDirectoryState } from "../composables/useDirectoryState";
import { useRoute, useRouter } from "vue-router";
import {
  mdiArrowLeft,
  mdiChevronLeft,
  mdiChevronRight,
  mdiClose,
  mdiFolderOpen,
  mdiMagnify,
  mdiStar,
  mdiLoading,
  mdiNoteTextOutline,
  mdiEye,
  mdiEyeOff,
  mdiHome,
  mdiHistory,
  mdiImage,
} from "@mdi/js";
import useQuery from "../graphql/utils/useQuery";
import { formatDate } from "@/utils/date";
import { PRESET_COLORS } from "../composables/useImageLabel";
import useBrowseImages from "../composables/useBrowseImages";
import useBrowseMemos from "../composables/useBrowseMemos";
import {
  DirectoriesDocument,
  type MemoFragment,
  type BrowseImagesQueryVariables,
  type BrowseMemosQueryVariables,
  type ImageFiltersInput,
  type MemoFiltersInput,
} from "../graphql/generated";
import ImageViewer from "../components/ImageViewer.vue";
import RatingFilter from "../components/RatingFilter.vue";
import MemoEditorDialog from "../components/MemoEditorDialog.vue";
import SubdirectoryGrid from "../components/SubdirectoryGrid.vue";
import ToggleSwitch from "../components/ToggleSwitch.vue";

// #region 路由参数与导航
const route = useRoute();
const router = useRouter();

// 目录ID，默认从路由 query 中获取，否则为空字符串（即代表根目录）
const currentDirectoryId = computed(() => (route.query.dir as string) || "");

function navigateToDir(id: string) {
  router.push({
    path: "/browse",
    query: id ? { dir: id } : {},
  });
}

function navigateToHome() {
  router.push("/");
}

function goToLastSession() {
  if (lastSession.value) {
    router.push({
      name: "session",
      params: { id: lastSession.value.id },
    });
  }
}
// #endregion

// #region 过滤器与多选状态
const {
  filterRating,
  filterLabels,
  searchQuery,
  showHiddenMemos,
  hasActiveFilters,
  clearFilters,
  lastSession,
} = useDirectoryState(currentDirectoryId);

function toggleLabelFilter(label: string) {
  const nextLabels = [...filterLabels.value];
  const index = nextLabels.indexOf(label);
  if (index >= 0) {
    nextLabels.splice(index, 1);
  } else {
    nextLabels.push(label);
  }
  filterLabels.value = nextLabels;
}
// #endregion

// #region 目录与子目录查询
const loadingCount = ref(0);

const { data: directoriesData } = useQuery(DirectoriesDocument, {
  variables: () => ({
    id: currentDirectoryId.value,
  }),
  loadingCount,
});

const currentDirectory = computed(() => {
  const node = directoriesData.value?.node;
  return node?.__typename === "Directory" ? node : undefined;
});

const subDirectories = computed(() => {
  return currentDirectory.value?.directories || [];
});

function goToParent() {
  if (currentDirectory.value?.parentId) {
    navigateToDir(currentDirectory.value.parentId);
  } else {
    navigateToDir("");
  }
}
// #endregion

// #region 获取并管理实时更新的图片列表
// 构建图片查询 variables
const imagesVariables = computed<BrowseImagesQueryVariables>(() => {
  const filterBy: ImageFiltersInput = {
    rating: filterRating.value,
    label: filterLabels.value.length > 0 ? filterLabels.value : null,
    query: searchQuery.value || null,
  };
  return {
    id: currentDirectoryId.value,
    filterBy,
    first: 100, // 每页 100 张
    after: null,
  };
});

// 对 loading 状态的综合追踪
const loading = computed(() => loadingCount.value > 0);

// 调用 useBrowseImages 获取图片列表
const {
  images,
  hasNextPage,
  loadMore: imagesLoadMore,
} = useBrowseImages(imagesVariables, { loadingCount });

// 触发分页加载更多图片
function loadMore() {
  if (loading.value || !hasNextPage.value) return;
  imagesLoadMore();
}
// #endregion

// #region 获取并管理实时更新的备忘录/笔记列表
const selectedMemo = ref<MemoFragment | null>(null);
const isMemoEditorOpen = ref(false);

// 构建备忘录查询 variables
const memosVariables = computed<BrowseMemosQueryVariables>(() => {
  const filterBy: MemoFiltersInput = {
    directoryId: [currentDirectoryId.value],
  };
  if (!showHiddenMemos.value) {
    filterBy.hidden = false;
  }
  return {
    id: currentDirectoryId.value,
    filterBy,
    first: 100,
    after: null,
  };
});

// 调用 useBrowseMemos 获取备忘录列表与隐藏切换操作
const { memos, toggleMemoHidden } = useBrowseMemos(memosVariables, {
  loadingCount,
});

function editMemo(memoItem: MemoFragment) {
  selectedMemo.value = memoItem;
  isMemoEditorOpen.value = true;
}
// #endregion

// #region 全屏查看器模块
const currentImageIndex = ref<number | undefined>(undefined);
const currentImage = computed(() => {
  if (currentImageIndex.value === undefined) return undefined;
  return images.value[currentImageIndex.value];
});

function openViewer(index: number) {
  currentImageIndex.value = index;
}

function closeViewer() {
  currentImageIndex.value = undefined;
}

function prevImage() {
  if (currentImageIndex.value !== undefined && currentImageIndex.value > 0) {
    currentImageIndex.value--;
  }
}

function nextImage() {
  if (
    currentImageIndex.value !== undefined &&
    currentImageIndex.value < images.value.length - 1
  ) {
    currentImageIndex.value++;
  }
}

// 挂载和卸载查看器的键盘左右键以及 Esc 键监听
function handleGlobalKeydown(e: KeyboardEvent) {
  if (currentImageIndex.value === undefined) return;

  if (e.key === "ArrowLeft") {
    prevImage();
  } else if (e.key === "ArrowRight") {
    nextImage();
  } else if (e.key === "Escape") {
    closeViewer();
  }
}

onMounted(() => {
  window.addEventListener("keydown", handleGlobalKeydown);
});

onUnmounted(() => {
  window.removeEventListener("keydown", handleGlobalKeydown);
});
// #endregion

// #region 格式化辅助方法
function formatSize(bytes: number | undefined): string {
  if (!bytes) return "0 B";
  const k = 1024;
  const sizes = ["B", "KB", "MB", "GB"];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + " " + sizes[i];
}
// #endregion
</script>
