<template>
  <div class="space-y-6">
    <!-- 图片网格展示区 -->
    <section
      class="space-y-3 bg-primary-800/30 border border-primary-700/50 rounded-2xl p-4 sm:p-6 backdrop-blur-sm"
    >
      <!-- 图片列表标题与图片专用的筛选过滤条件 -->
      <div
        class="flex flex-wrap items-center justify-between gap-x-6 gap-y-3 border-b border-primary-700/50 pb-3"
      >
        <h2
          class="text-base font-bold text-primary-200 tracking-wider flex flex-wrap items-center gap-2 select-none"
        >
          <svg class="w-5 h-5 text-secondary-400" viewBox="0 0 24 24">
            <path :d="mdiImage" fill="currentColor" />
          </svg>
          <span>图片列表</span>

          <!-- 评星统计 -->
          <div
            v-if="stats && stats.ratingCounts.length > 0"
            class="flex items-center gap-2 ml-2 text-xs font-normal"
          >
            <button
              v-for="rc in sortedRatingCounts"
              :key="rc.rating"
              class="flex items-center gap-1 px-2 py-1 rounded bg-primary-700/50 hover:bg-primary-600/80 transition-colors cursor-pointer select-none"
              :title="
                filterRating.includes(rc.rating)
                  ? `取消筛选 ${rc.rating === 0 ? '无评分' : rc.rating + '星'}`
                  : `筛选 ${rc.rating === 0 ? '无评分' : rc.rating + '星'}`
              "
              @click="toggleRatingFilter(rc.rating)"
            >
              <RatingIcon
                :rating="rc.rating"
                :filled="filterRating.includes(rc.rating)"
              />
              <span class="text-xs">{{ rc.count }}</span>
            </button>
          </div>

          <!-- 删除低星级图片按钮 -->
          <button
            v-if="deleteUnmatchedInfo"
            :disabled="isDeletingUnmatched"
            class="px-3 h-8 text-xs font-normal rounded-lg transition-all flex items-center gap-1 bg-red-950/40 hover:bg-red-900/40 border border-red-900/50 text-red-300 cursor-pointer select-none hover:text-white"
            :class="isDeletingUnmatched ? 'opacity-50 cursor-not-allowed' : ''"
            :title="`删除该目录下所有评分在 ${deleteUnmatchedInfo.maxUnmatched} 星及以下的图片`"
            @click="handleDeleteUnmatched"
          >
            <svg
              v-if="isDeletingUnmatched"
              class="w-4 h-4 animate-spin"
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
            <svg v-else class="w-4 h-4" viewBox="0 0 24 24">
              <path :d="mdiDelete" fill="currentColor" />
            </svg>
            <span>删除</span>
            <RatingIcon
              :rating="deleteUnmatchedInfo.maxUnmatched"
              filled
              class="w-4 h-4"
            />
            <span>以下的图片</span>
          </button>

          <!-- 加载中即使有缓存数据也显示旋转加载提示 -->
          <svg
            v-if="loading"
            class="w-4 h-4 animate-spin text-secondary-500"
            viewBox="0 0 24 24"
            fill="none"
            title="正在加载最新数据..."
          >
            <path
              :d="mdiLoading"
              fill="none"
              stroke="currentColor"
              stroke-width="3"
              stroke-linecap="round"
            />
          </svg>
        </h2>

        <div class="flex flex-wrap items-center gap-3">
          <!-- 移动匹配图片按钮 -->
          <button
            v-if="images.length > 0 && !isBulkMode"
            class="px-3 h-8 text-xs border rounded-lg transition-all flex items-center gap-1 bg-primary-800 hover:bg-primary-700 border-primary-700 text-primary-200 cursor-pointer select-none"
            title="将当前过滤匹配的图片移动到新目录"
            @click="moveImagesDialog.open()"
          >
            <svg class="w-4 h-4 text-secondary-400" viewBox="0 0 24 24">
              <path :d="mdiFolderMove" fill="currentColor" />
            </svg>
            <span>移动匹配图片</span>
          </button>

          <!-- 批量管理按钮 -->
          <button
            v-if="images.length > 0"
            class="px-3 h-8 text-xs border rounded-lg transition-all flex items-center gap-1 cursor-pointer select-none"
            :class="[
              isBulkMode
                ? 'bg-secondary-600 hover:bg-secondary-700 border-secondary-500 text-white shadow-[0_0_10px_rgba(235,94,85,0.3)] font-semibold'
                : 'bg-primary-800 hover:bg-primary-700 border-primary-700 text-primary-200',
            ]"
            :title="
              isBulkMode
                ? '退出批量管理模式'
                : '进入批量管理模式，对多张图片执行操作'
            "
            @click="toggleBulkMode"
          >
            <svg class="w-4 h-4" viewBox="0 0 24 24">
              <path :d="mdiCheckboxMultipleMarkedOutline" fill="currentColor" />
            </svg>
            <span>{{ isBulkMode ? "退出批量" : "批量管理" }}</span>
          </button>

          <!-- 当用户激活了任何过滤器时，在最左侧显示一键清除筛选按钮 -->
          <button
            v-if="hasActiveFilters"
            class="px-3 h-8 text-xs border rounded-lg transition-all flex items-center gap-1 bg-red-950/40 hover:bg-red-900/40 border-red-900/50 text-red-300 cursor-pointer"
            @click="clearFilters"
          >
            <svg class="w-4 h-4" viewBox="0 0 24 24">
              <path :d="mdiFilterOff" fill="currentColor" />
            </svg>
            <span>清除筛选</span>
          </button>

          <!-- 重新筛选本地已修改且不再满足筛选条件的图片 -->
          <button
            v-if="outOfFilterImageIds.size > 0"
            class="px-3 h-8 text-xs border rounded-lg transition-all flex items-center gap-2 bg-amber-950/40 hover:bg-amber-900/40 border-amber-900/50 text-amber-300 cursor-pointer animate-pulse"
            title="隐藏不再符合当前过滤条件的图片"
            @click="handleApplyLocalFilter"
          >
            <svg class="w-4 h-4 text-amber-400" viewBox="0 0 24 24">
              <path :d="mdiRefresh" fill="currentColor" />
            </svg>
            <span>重新筛选 ({{ outOfFilterImageIds.size }}张已改)</span>
          </button>

          <!-- 搜索输入框 -->
          <div class="relative min-w-36 max-w-60 flex-1 sm:flex-none">
            <input
              v-model="searchQuery"
              type="text"
              placeholder="搜索文件名..."
              class="w-full pl-8 pr-8 h-8 bg-primary-800/80 border border-primary-700 hover:border-primary-600 focus:border-secondary-500 rounded-lg text-xs text-primary-100 placeholder-primary-500 focus:outline-none focus:ring-2 focus:ring-secondary-500/30 transition-all"
            />
            <svg
              class="w-4 h-4 text-primary-400 absolute left-2 top-1/2 -translate-y-1/2 pointer-events-none"
              viewBox="0 0 24 24"
            >
              <path :d="mdiMagnify" fill="currentColor" />
            </svg>
            <button
              v-if="searchQuery"
              class="absolute right-2 top-1/2 -translate-y-1/2 text-primary-400 hover:text-primary-200 transition-colors p-0.5 rounded-full hover:bg-primary-700/50 cursor-pointer"
              title="清空"
              @click="searchQuery = ''"
            >
              <svg class="w-3 h-3" viewBox="0 0 24 24">
                <path :d="mdiClose" fill="currentColor" />
              </svg>
            </button>
          </div>

          <!-- 评星过滤器 -->
          <RatingFilter v-model="filterRating" />

          <!-- 颜色标签过滤器 -->
          <div
            class="flex items-center gap-2 bg-primary-800 border border-primary-700 px-3 h-8 rounded-lg overflow-x-auto"
          >
            <span class="text-xs text-primary-400 select-none">标签:</span>
            <div class="flex items-center gap-1">
              <button
                v-for="(colorHex, colorName) in PRESET_COLORS"
                :key="colorName"
                class="w-3 h-3 rounded-full transition-all border border-white/20 relative"
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
                  class="absolute inset-px rounded-full border border-black/30"
                ></span>
              </button>
            </div>
          </div>
        </div>
      </div>

      <!-- 滚动容器：包裹列表、空状态、骨架图与加载更多按钮 -->
      <div
        ref="containerRef"
        class="max-h-[60vh] overflow-y-auto pr-1 space-y-4"
      >
        <!-- 骨架图加载指示，避免布局抖动 -->
        <div
          v-if="loading && images.length === 0"
          class="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-6 xl:grid-cols-8 gap-4 animate-pulse p-4"
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
            class="w-12 h-12 stroke-2"
            fill="none"
            stroke="currentColor"
            viewBox="0 0 24 24"
          >
            <path
              stroke-linecap="round"
              stroke-linejoin="round"
              d="M2.25 15.75l5.159-5.159a2.25 2.25 0 013.182 0l5.159 5.159m-1.5-1.5l1.409-1.409a2.25 2.25 0 013.182 0l2.909 2.909m-18 3.75h16.5a1.5 1.5 0 001.5-1.5V6a1.5 1.5 0 00-1.5-1.5H3.75A1.5 1.5 0 002.25 6v12a1.5 1.5 0 00-1.5 1.5zm10.5-11.25h.008v.008h-.008V8.25zm.375 0a.375.375 0 11-.75 0 .375.375 0 01.75 0z"
            />
          </svg>
          <span class="text-sm">该目录或过滤条件下未找到任何图片</span>
        </div>

        <!-- 网格列表 -->
        <div
          v-else
          class="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-6 xl:grid-cols-8 gap-4 p-4"
        >
          <div
            v-for="img in images"
            :key="img.id"
            class="group relative bg-primary-800/40 hover:bg-primary-800/90 border rounded-xl overflow-hidden aspect-square cursor-pointer transition-all hover:scale-105 hover:shadow-lg hover:shadow-black/40 flex flex-col justify-between"
            :class="[
              isBulkMode && isSelected(img.id)
                ? 'border-secondary-500 ring-2 ring-secondary-500/50 bg-primary-800/90 scale-105'
                : 'border-primary-800 hover:border-primary-600/80',
              outOfFilterImageIds.has(img.id)
                ? 'border-yellow-600 border-2 border-dashed'
                : '',
            ]"
            @click="handleImageClick(img, $event)"
          >
            <!-- 选中态的整体外框 overlay，防止被 overflow-hidden 裁剪或子元素遮挡 -->
            <div
              v-if="isBulkMode && isSelected(img.id)"
              class="absolute inset-0 border-2 border-secondary-500 rounded-xl pointer-events-none z-10"
            ></div>
            <!-- 缩略图加载 -->
            <div
              class="w-full h-full relative overflow-hidden bg-black/10 flex items-center justify-center"
            >
              <!-- 左上角勾选徽章 -->
              <div
                v-if="isBulkMode"
                class="absolute top-2 left-2 z-10 w-6 h-6 rounded-full flex items-center justify-center transition-all duration-200 border cursor-pointer"
                :class="[
                  isSelected(img.id)
                    ? 'bg-secondary-500 border-secondary-400 text-white shadow-[0_2px_8px_rgba(235,94,85,0.4)] scale-110'
                    : 'bg-black/40 border-white/20 text-white/50 opacity-0 group-hover:opacity-100 hover:scale-105',
                ]"
              >
                <svg class="w-4 h-4" viewBox="0 0 24 24">
                  <path
                    :d="mdiCheck"
                    fill="currentColor"
                    stroke="currentColor"
                    stroke-width="2"
                  />
                </svg>
              </div>

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
                <RatingIcon
                  v-if="img.currentRating"
                  :rating="img.currentRating"
                  filled
                />

                <!-- 颜色标签：使用白色边框 + 黑色描边 ring 增强对比度，以防与图片背景颜色融为一体 -->
                <span
                  v-if="img.label"
                  class="w-3 h-3 rounded-full shadow-md border border-white ml-auto ring-1 ring-black/30"
                  :style="{
                    backgroundColor: PRESET_COLORS[img.label] || '#94a3b8',
                  }"
                  :title="img.label"
                ></span>
              </div>
            </div>

            <!-- 卡片底部的文件名遮罩 -->
            <div
              class="absolute inset-x-0 top-0 bg-linear-to-b from-black/80 to-transparent p-2 opacity-0 group-hover:opacity-100 transition-opacity duration-200 pointer-events-none"
            >
              <p
                class="text-xs text-white font-medium truncate"
                :title="img.filename"
              >
                {{ img.filename }}
              </p>
            </div>
          </div>
        </div>

        <!-- 懒加载过渡区与加载更多按钮 -->
        <div v-if="hasNextPage" class="flex justify-center pt-2">
          <button
            :disabled="loading"
            class="px-6 py-2 bg-primary-800 hover:bg-primary-700 border border-primary-700 hover:border-primary-600 rounded-xl text-sm font-semibold transition-all flex items-center gap-2 text-primary-200 hover:text-white"
            @click="fetchMore"
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
        </div>
      </div>
    </section>

    <!-- 全屏查看器模态框 -->
    <imageViewerDialog.component
      v-if="currentImageId !== undefined && currentImage"
      @after-leave="handleViewerAfterLeave"
    >
      <div class="w-full h-full flex flex-col justify-between">
        <!-- 侧边关闭按钮 -->
        <button
          class="absolute top-4 right-4 z-60 p-2 rounded-full bg-white/5 hover:bg-white/10 text-white/70 hover:text-white transition-colors border border-white/10"
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
          class="absolute left-4 top-1/2 -translate-y-1/2 z-60 p-3 rounded-xl bg-white/5 hover:bg-white/10 hover:scale-105 active:scale-95 text-white/80 hover:text-white transition-all border border-white/10"
          title="上一张图片 (ArrowLeft)"
          @click="prevImage"
        >
          <svg class="w-8 h-8" viewBox="0 0 24 24">
            <path :d="mdiChevronLeft" fill="currentColor" />
          </svg>
        </button>

        <!-- 下一张按钮 -->
        <button
          v-if="
            currentImageIndex >= 0 &&
            (currentImageIndex < images.length - 1 || hasNextPage)
          "
          class="absolute right-4 top-1/2 -translate-y-1/2 z-60 p-3 rounded-xl bg-white/5 hover:bg-white/10 hover:scale-105 active:scale-95 text-white/80 hover:text-white transition-all border border-white/10"
          title="下一张图片 (ArrowRight)"
          @click="nextImage"
        >
          <!-- 浏览至当前页最后一张图且在加载下一页数据时，显示旋转加载提示 -->
          <svg
            v-if="currentImageIndex === images.length - 1 && loading"
            class="w-8 h-8 animate-spin text-secondary-500"
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
          <svg v-else class="w-8 h-8" viewBox="0 0 24 24">
            <path :d="mdiChevronRight" fill="currentColor" />
          </svg>
        </button>

        <!-- 图像查看器组件 -->
        <ImageViewer
          :image="currentImage"
          :preload-images="preloadImages"
          class="w-full h-full flex-1"
          @request-next="nextImage"
        >
          <!-- 插入底部信息 -->
          <template #info>
            <span
              class="truncate max-w-72 font-semibold"
              :title="currentImage.filename"
            >
              {{ currentImage.filename }}
            </span>
            <div class="w-px h-4 bg-white/30 mx-1"></div>
            <span> {{ currentImageIndex + 1 }} / {{ images.length }} </span>
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
    </imageViewerDialog.component>

    <!-- 移动图片模态框 -->
    <moveImagesDialog.component
      container-class="sm:max-w-md p-6"
      overflow-visible
    >
      <MoveImagesForm
        :directory-id="directoryId"
        :filter-by="selectedFilterBy || {}"
        :match-count="moveImagesMatchCount"
        :is-approximate="!isBulkMode && hasNextPage"
        @close="handleMoveClose"
      />
    </moveImagesDialog.component>

    <!-- 批量操作底栏 -->
    <div
      class="fixed bottom-6 left-1/2 -translate-x-1/2 z-50 w-[calc(100%-2rem)] max-w-4xl pointer-events-none"
    >
      <Transition name="slide-up">
        <div
          v-if="isBulkMode"
          class="pointer-events-auto bg-primary-900/90 backdrop-blur-xl border border-primary-700/80 rounded-2xl shadow-[0_10px_30px_-5px_rgba(0,0,0,0.8)] px-4 py-3 flex flex-col md:flex-row md:items-center justify-between gap-4 transition-all duration-300"
        >
          <!-- 左侧：选择状态与全选控制 -->
          <div class="flex items-center justify-between md:justify-start gap-4">
            <div class="flex items-center gap-2">
              <span
                class="inline-flex items-center justify-center w-max min-w-6 px-1.5 h-6 rounded-full bg-secondary-500/20 border border-secondary-500/30 text-xs font-bold text-secondary-400 animate-pulse"
              >
                {{ selectedCountText }}
              </span>
              <span class="text-xs text-primary-200 font-medium"
                >张图片已选中</span
              >
            </div>
            <div class="h-4 w-px bg-primary-700 hidden md:block"></div>
            <div class="flex items-center gap-2">
              <button
                v-if="!isAllMatchingSelected"
                class="px-2 py-1 text-xs text-primary-300 hover:text-white bg-primary-800 hover:bg-primary-700 border border-primary-700/60 rounded-lg transition-colors cursor-pointer select-none"
                @click="selectAll"
              >
                全选
              </button>
              <button
                v-if="isAllMatchingSelected"
                class="px-2 py-1 text-xs text-red-300 hover:text-white bg-red-950/40 hover:bg-red-900/40 border border-red-900/50 rounded-lg transition-colors cursor-pointer select-none font-semibold"
                @click="deselectAll"
              >
                清除
              </button>
              <button
                v-if="!isAllMatchingSelected"
                class="px-2 py-1 text-xs text-primary-300 hover:text-white bg-primary-800 hover:bg-primary-700 border border-primary-700/60 rounded-lg transition-colors cursor-pointer select-none"
                @click="invertSelection"
              >
                反选
              </button>
            </div>
          </div>

          <!-- 右侧：批量动作 -->
          <div class="flex flex-wrap items-center justify-end gap-3">
            <!-- 批量评分 -->
            <div class="relative group/rating">
              <button
                class="px-3 h-9 text-xs font-semibold bg-primary-800 hover:bg-primary-700 border border-primary-700/80 text-primary-200 rounded-xl transition-all flex items-center gap-2 cursor-pointer hover:border-secondary-500/50 select-none"
                :disabled="!selectedFilterBy || isUpdating"
                :class="[
                  !selectedFilterBy ? 'opacity-40 cursor-not-allowed' : '',
                  activeDropdown === 'rating'
                    ? 'border-secondary-500/50 text-white bg-primary-700'
                    : '',
                ]"
                @click="toggleDropdown('rating', $event)"
              >
                <svg class="w-4 h-4 text-yellow-400" viewBox="0 0 24 24">
                  <path :d="mdiStar" fill="currentColor" />
                </svg>
                <span>评星</span>
              </button>

              <!-- 评分悬浮窗 -->
              <div
                v-if="selectedFilterBy"
                class="absolute bottom-full right-0 mb-2 transition-all duration-200 bg-primary-900/95 backdrop-blur-md border border-primary-700/60 p-2 rounded-xl shadow-xl flex items-center gap-1 z-60 w-max"
                :class="[
                  activeDropdown === 'rating'
                    ? 'visible opacity-100'
                    : 'invisible group-hover/rating:visible opacity-0 group-hover/rating:opacity-100',
                ]"
                @click.stop
              >
                <RatingSelector v-model="bulkRating" />
              </div>
            </div>

            <!-- 批量标签 -->
            <div class="relative group/label">
              <button
                class="px-3 h-9 text-xs font-semibold bg-primary-800 hover:bg-primary-700 border border-primary-700/80 text-primary-200 rounded-xl transition-all flex items-center gap-2 cursor-pointer hover:border-secondary-500/50 select-none"
                :disabled="!selectedFilterBy || isUpdating"
                :class="[
                  !selectedFilterBy ? 'opacity-40 cursor-not-allowed' : '',
                  activeDropdown === 'label'
                    ? 'border-secondary-500/50 text-white bg-primary-700'
                    : '',
                ]"
                @click="toggleDropdown('label', $event)"
              >
                <span
                  class="w-3 h-3 rounded-full bg-linear-to-tr from-sky-400 via-green-400 to-yellow-400"
                ></span>
                <span>标签</span>
              </button>

              <!-- 标签悬浮窗 -->
              <div
                v-if="selectedFilterBy"
                class="absolute bottom-full right-0 mb-2 transition-all duration-200 bg-primary-900/95 backdrop-blur-md border border-primary-700/60 p-2 rounded-xl shadow-xl z-60 w-max"
                :class="[
                  activeDropdown === 'label'
                    ? 'visible opacity-100'
                    : 'invisible group-hover/label:visible opacity-0 group-hover/label:opacity-100',
                ]"
                @click.stop
              >
                <div class="flex items-center gap-2">
                  <button
                    v-for="(colorHex, colorName) in PRESET_COLORS"
                    :key="colorName"
                    class="w-6 h-6 rounded-full transition-all border border-white/20 hover:scale-120 cursor-pointer relative"
                    :style="{ backgroundColor: colorHex }"
                    :title="colorName"
                    @click="handleBulkSetLabel(colorName)"
                  ></button>
                  <div class="w-px h-5 bg-primary-700 mx-1"></div>
                  <button
                    class="px-2 py-1 text-xs hover:bg-primary-800 border border-primary-700/60 hover:text-white rounded-lg text-primary-300 transition-colors cursor-pointer select-none"
                    @click="handleBulkSetLabel('')"
                  >
                    清除
                  </button>
                </div>
              </div>
            </div>

            <!-- 批量移动 -->
            <button
              class="px-4 h-9 text-xs font-semibold bg-primary-800 hover:bg-primary-700 border border-primary-700/80 text-primary-200 rounded-xl transition-all flex items-center gap-2 cursor-pointer hover:border-secondary-500/50 select-none"
              :disabled="!selectedFilterBy || isUpdating"
              :class="!selectedFilterBy ? 'opacity-40 cursor-not-allowed' : ''"
              @click="moveImagesDialog.open()"
            >
              <svg class="w-4 h-4 text-secondary-400" viewBox="0 0 24 24">
                <path :d="mdiFolderMove" fill="currentColor" />
              </svg>
              <span>移动</span>
            </button>

            <!-- 批量复制 -->
            <button
              class="px-4 h-9 text-xs font-semibold bg-primary-800 hover:bg-primary-700 border border-primary-700/80 text-primary-200 rounded-xl transition-all flex items-center gap-2 select-none"
              :disabled="!selectedFilterBy || isCopying"
              :class="
                !selectedFilterBy || isCopying
                  ? 'opacity-40 cursor-not-allowed'
                  : 'cursor-pointer hover:border-secondary-500/50'
              "
              @click="copySelectedImages"
            >
              <svg
                v-if="isCopying"
                class="w-4 h-4 text-secondary-400 animate-spin"
                viewBox="0 0 24 24"
              >
                <path :d="mdiLoading" fill="currentColor" />
              </svg>
              <svg
                v-else
                class="w-4 h-4 text-secondary-400"
                viewBox="0 0 24 24"
              >
                <path :d="mdiContentCopy" fill="currentColor" />
              </svg>
              <span>{{ isCopying ? "复制中" : "复制" }}</span>
            </button>

            <!-- 批量动作 -->
            <div
              v-if="dispatchableHooks.length > 0"
              class="relative group/hook"
            >
              <button
                class="px-4 h-9 text-xs font-semibold bg-primary-800 hover:bg-primary-700 border border-primary-700/80 text-primary-200 rounded-xl transition-all flex items-center gap-2 select-none"
                :disabled="!selectedFilterBy || isBulkDispatching"
                :class="[
                  !selectedFilterBy || isBulkDispatching
                    ? 'opacity-40 cursor-not-allowed'
                    : 'cursor-pointer hover:border-secondary-500/50',
                  activeDropdown === 'hook'
                    ? 'border-secondary-500/50 text-white bg-primary-700'
                    : '',
                ]"
                @click="toggleDropdown('hook', $event)"
              >
                <svg
                  v-if="isBulkDispatching"
                  class="w-4 h-4 text-secondary-400 animate-spin"
                  viewBox="0 0 24 24"
                >
                  <path
                    :d="mdiLoading"
                    fill="none"
                    stroke="currentColor"
                    stroke-width="3"
                    stroke-linecap="round"
                  />
                </svg>
                <svg
                  v-else
                  class="w-4 h-4 text-secondary-400"
                  viewBox="0 0 24 24"
                >
                  <path :d="mdiPlayOutline" fill="currentColor" />
                </svg>
                <span>动作</span>
              </button>

              <!-- 动作悬浮窗 -->
              <div
                v-if="selectedFilterBy"
                class="absolute bottom-full right-0 mb-2 transition-all duration-200 bg-primary-900/95 backdrop-blur-md border border-primary-700/60 p-2 rounded-xl shadow-xl z-60 w-52 flex flex-col gap-1 text-left"
                :class="[
                  activeDropdown === 'hook'
                    ? 'visible opacity-100'
                    : 'invisible group-hover/hook:visible opacity-0 group-hover/hook:opacity-100',
                ]"
                @click.stop
              >
                <div
                  class="text-xs font-bold text-primary-400 tracking-wider uppercase select-none px-2 py-1"
                >
                  选择执行动作
                </div>
                <button
                  v-for="hook in dispatchableHooks"
                  :key="hook.id"
                  class="px-2 py-1 text-xs text-left text-primary-200 hover:text-white hover:bg-primary-800 rounded-lg transition-colors flex items-center justify-between cursor-pointer select-none"
                  :title="hook.description || hook.name"
                  @click="handleBulkDispatch(hook.id, hook.name)"
                >
                  <span class="truncate pr-2">{{ hook.name }}</span>
                  <svg
                    class="w-4 h-4 shrink-0 text-primary-500"
                    viewBox="0 0 24 24"
                    fill="currentColor"
                  >
                    <path :d="mdiPlayOutline" fill="currentColor" />
                  </svg>
                </button>
              </div>
            </div>

            <div class="h-5 w-px bg-primary-700"></div>

            <!-- 关闭批量管理模式 -->
            <button
              class="px-3 h-9 text-xs font-semibold bg-red-950/40 hover:bg-red-900/40 border border-red-900/50 text-red-300 rounded-xl transition-colors cursor-pointer flex items-center gap-1 select-none"
              @click="toggleBulkMode"
            >
              <svg class="w-4 h-4" viewBox="0 0 24 24">
                <path :d="mdiClose" fill="currentColor" />
              </svg>
              <span>退出</span>
            </button>
          </div>
        </div>
      </Transition>
    </div>
  </div>
</template>

<script setup lang="ts">
import {
  ref,
  computed,
  watchEffect,
  onMounted,
  nextTick,
  useTemplateRef,
  watch,
} from "vue";
import useInfiniteScroll from "@/composables/useInfiniteScroll";
import useEventListeners from "@/composables/useEventListeners";
import {
  mdiImage,
  mdiFilterOff,
  mdiMagnify,
  mdiClose,
  mdiLoading,
  mdiChevronLeft,
  mdiChevronRight,
  mdiFolderMove,
  mdiCheck,
  mdiCheckboxMultipleMarkedOutline,
  mdiStar,
  mdiRefresh,
  mdiContentCopy,
  mdiDelete,
  mdiPlayOutline,
} from "@mdi/js";
import { PRESET_COLORS, COLOR_NAMES_CN } from "@/composables/useImageLabel";
import RatingIcon from "./RatingIcon.vue";
import RatingSelector from "./RatingSelector.vue";
import useBulkOperations from "@/composables/useBulkOperations";
import RatingFilter from "./RatingFilter.vue";
import ImageViewer from "./ImageViewer.vue";
import { useHotkeys } from "@/composables/useHotkeys";
import { useDirectoryState } from "@/composables/useDirectoryState";
import useBrowseImages from "@/composables/useBrowseImages";
import useDirectoryStats from "@/composables/useDirectoryStats";
import { formatSize } from "@/utils/formatSize";
import type {
  BrowseImagesQueryVariables,
  ImageFiltersInput,
  ImageFragment,
} from "@/graphql/generated";
import MoveImagesForm from "./MoveImagesForm.vue";
import useModalDialog from "@/composables/useModalDialog";
import useModalFullscreen from "@/composables/useModalFullscreen";
import { openImageViewerByFilename } from "@/events";
import useLocationHash from "@/composables/useLocationHash";
import optionalArray from "@/utils/optionalArray.ts";
import useImageHooks from "@/composables/useImageHooks";
import { useClipboard } from "@/composables/useClipboard";
import useNotification from "@/composables/useNotification";
import useTrashImages from "@/composables/useTrashImages";

// #region 属性与事件定义
const props = defineProps<{
  directoryId: string;
}>();
// #endregion

// #region 状态管理
const {
  filterRating,
  filterLabels,
  searchQuery,
  hasActiveFilters,
  clearFilters,
} = useDirectoryState(() => props.directoryId);

// 切换特定星级的筛选状态（点击统计区评星时，清除其他筛选，仅保留当前星级）
function toggleRatingFilter(rating: number) {
  const index = filterRating.value.indexOf(rating);
  if (index >= 0) {
    filterRating.value = filterRating.value.filter((r) => r !== rating);
  } else {
    const previousRating = filterRating.value;
    clearFilters();
    filterRating.value = [...previousRating, rating].sort((a, b) => a - b);
  }
}

// 目录统计信息
const { useStats } = useDirectoryStats();
const statsData = useStats(() => props.directoryId);
const stats = computed(() => {
  const node = statsData.value?.node;
  return node?.__typename === "Directory" ? node.stats : undefined;
});

// 对评分统计进行排序
const sortedRatingCounts = computed(() => {
  if (!stats.value?.ratingCounts) return [];
  return [...stats.value.ratingCounts].sort((a, b) => a.rating - b.rating);
});

// #region 删除低星级图片功能
const { show: showNotification } = useNotification();
const { trashImages } = useTrashImages();
const deletingUnmatchedBuffer = ref({
  directoryId: props.directoryId,
  value: false,
});
const isDeletingUnmatched = computed({
  get: () =>
    deletingUnmatchedBuffer.value.directoryId === props.directoryId
      ? deletingUnmatchedBuffer.value.value
      : false,
  set: (val) => {
    deletingUnmatchedBuffer.value = {
      directoryId: props.directoryId,
      value: val,
    };
  },
});

const deleteUnmatchedInfo = computed(() => {
  if (!stats.value?.ratingCounts || filterRating.value.length === 0)
    return null;

  // 过滤出该目录下 count > 0 的星级
  const existingRatingsWithCount = stats.value.ratingCounts.filter(
    (rc) => rc.count > 0,
  );
  if (existingRatingsWithCount.length === 0) return null;

  const existingRatings = existingRatingsWithCount.map((rc) => rc.rating);
  const matchedRatings = existingRatings.filter((r) =>
    filterRating.value.includes(r),
  );
  const unmatchedRatings = existingRatings.filter(
    (r) => !filterRating.value.includes(r),
  );

  if (matchedRatings.length === 0 || unmatchedRatings.length === 0) return null;

  const minMatched = Math.min(...matchedRatings);
  const maxUnmatched = Math.max(...unmatchedRatings);

  // 在所有不符合筛选的图片星级都小于筛选匹配图片中最低星时满足条件
  if (maxUnmatched < minMatched) {
    const totalCount = existingRatingsWithCount
      .filter((rc) => unmatchedRatings.includes(rc.rating))
      .reduce((sum, rc) => sum + rc.count, 0);

    return {
      maxUnmatched,
      totalCount,
    };
  }

  return null;
});

async function handleDeleteUnmatched() {
  const info = deleteUnmatchedInfo.value;
  if (!info || isDeletingUnmatched.value) return;

  isDeletingUnmatched.value = true;
  try {
    // 构造不符合筛选的星级列表 [0, 1, ..., maxUnmatched]
    const ratingsToDelete = Array.from(
      { length: info.maxUnmatched + 1 },
      (_, i) => i,
    );

    await trashImages(props.directoryId, {
      rating: ratingsToDelete,
    });
  } catch (err) {
    showNotification(
      err instanceof Error ? err.message : "删除图片失败",
      "error",
    );
  } finally {
    isDeletingUnmatched.value = false;
  }
}
// #endregion

// 提取加载状态计数，以实现精细的骨架图切换与加载动画
const loadingCount = ref(1);
onMounted(() => {
  nextTick(() => {
    loadingCount.value -= 1;
  });
});

// 构建图片查询 variables
const imagesVariables = computed<BrowseImagesQueryVariables>(() => {
  const filterBy: ImageFiltersInput = {
    rating: optionalArray(filterRating.value),
    label: optionalArray(filterLabels.value),
    query: searchQuery.value || undefined,
  };
  return {
    id: props.directoryId,
    filterBy,
    first: 20,
  };
});

// 对 loading 状态的综合追踪
const loading = computed(() => loadingCount.value > 0);

// 调用 useBrowseImages 获取图片列表
const {
  images,
  hasNextPage,
  fetchMore,
  outOfFilterImageIds,
  applyLocalFilter,
} = useBrowseImages(imagesVariables, {
  loadingCount,
});

const containerRef = useTemplateRef<HTMLElement>("containerRef");

useInfiniteScroll(containerRef, async () => {
  if (hasNextPage.value && !loading.value) {
    await fetchMore();
  }
});

// #region 批量操作状态与逻辑管理
const {
  isBulkMode,
  selectedFilterBy,
  selectedImages,
  isSelected,
  selectedCountText,
  isUpdating,
  isAllMatchingSelected,
  toggleBulkMode,
  toggleSelectImage,
  selectAll,
  deselectAll,
  invertSelection,
  bulkSetRating,
  bulkSetLabel,
} = useBulkOperations(
  images,
  () => props.directoryId,
  () => imagesVariables.value.filterBy || {},
  () => hasNextPage.value || false,
);

const moveImagesMatchCount = computed(() => {
  if (isBulkMode.value) {
    return selectedImages.value.length;
  }
  return images.value.length;
});

// #region 批量触发动作
const {
  dispatchableHooks,
  isDispatching: isBulkDispatching,
  dispatch,
} = useImageHooks({
  selectedFilterBy,
});

async function bulkDispatch(hookId: string, hookName: string) {
  if (!selectedFilterBy.value) return;
  await dispatch(hookId, hookName, selectedFilterBy.value);
}
// #endregion

// 批量操作下拉菜单状态
const activeDropdown = ref<"rating" | "label" | "hook" | null>(null);

function toggleDropdown(menu: "rating" | "label" | "hook", event: Event) {
  event.stopPropagation();
  if (activeDropdown.value === menu) {
    activeDropdown.value = null;
  } else {
    activeDropdown.value = menu;
  }
}

function closeDropdowns() {
  activeDropdown.value = null;
}

// 批量评分计算属性，用于绑定 RatingSelector 组件
const bulkRating = computed<number>({
  get() {
    if (selectedImages.value.length === 0) return 0;
    const firstImg = selectedImages.value[0];
    const rating = firstImg.currentRating || 0;
    const allSame = selectedImages.value.every(
      (img) => (img.currentRating || 0) === rating,
    );
    return allSame ? rating : 0;
  },
  set(val) {
    if (typeof val === "number") {
      void handleBulkSetRating(val);
    }
  },
});

// 代理批量操作并自动关闭下拉菜单
async function handleBulkSetRating(rating: number) {
  await bulkSetRating(rating);
  closeDropdowns();
}

async function handleBulkSetLabel(label: string) {
  await bulkSetLabel(label);
  closeDropdowns();
}

async function handleBulkDispatch(hookId: string, hookName: string) {
  await bulkDispatch(hookId, hookName);
  closeDropdowns();
}

useEventListeners(document, ({ on }) => {
  on("click", closeDropdowns);
});

// 监听批量状态，如果退出批量模式或没有选中图片，则关闭下拉菜单
watch([isBulkMode, selectedFilterBy], () => {
  if (!isBulkMode.value || !selectedFilterBy.value) {
    closeDropdowns();
  }
});

const copyLoadingCount = ref(0);
const { copyFiles } = useClipboard({
  loadingCount: copyLoadingCount,
});

const isCopying = computed(() => copyLoadingCount.value > 0);

// 复制所有选中的图片到剪贴板
async function copySelectedImages() {
  if (isCopying.value || !selectedFilterBy.value) return;

  // 如果是全选匹配状态，且还有未加载的页面
  if (isAllMatchingSelected.value && hasNextPage.value) {
    let fetchCount = 0;

    // 循环加载所有页面直到加载到底，每加载 3 页询问一次用户，防止大数据量卡死
    let limit = 2;
    while (hasNextPage.value) {
      await fetchMore();
      fetchCount++;

      if (fetchCount >= limit) {
        const ok = confirm(
          `已自动加载 ${fetchCount} 页，是否继续加载更多图片并复制？\n\n点击【确定】继续加载；\n点击【取消】仅复制当前已加载的 ${images.value.length} 张图片。`,
        );
        if (!ok) {
          break; // 用户取消，停止加载，直接复制已加载的
        }
        limit = Math.max(8, limit * 2);
      }
    }
  }

  const paths = selectedImages.value.map((img) => img.relPath);
  if (paths.length === 0) return;
  await copyFiles(...paths);
}

// 批量模式下移动成功后，关闭弹框，并清空选择状态
function handleMoveClose() {
  moveImagesDialog.close();
  if (isBulkMode.value) {
    deselectAll();
  }
}

// 批量模式下点击图片执行选择，或者按下 Ctrl/Meta 键点击图片自动进入批量模式并选中该图片。正常模式打开大图查看器
function handleImageClick(img: ImageFragment, event?: MouseEvent) {
  const isCtrlPressed = event ? event.ctrlKey || event.metaKey : false;

  if (isCtrlPressed) {
    if (!isBulkMode.value) {
      deselectAll(); // Ctrl+点击进入批量模式时先清空选择，确保只选中当前图片
      isBulkMode.value = true;
    }
    toggleSelectImage(img.id);
  } else if (isBulkMode.value) {
    toggleSelectImage(img.id);
  } else {
    openViewer(img);
  }
}

// 重新应用本地筛选
function handleApplyLocalFilter() {
  applyLocalFilter();
}
// #endregion
// #endregion

// #region 过滤器操作逻辑
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

// #region 全屏查看器模块
const currentImageId = ref<string | undefined>(undefined);
const currentImage = computed(() => {
  if (currentImageId.value === undefined) return undefined;
  return images.value.find((img) => img.id === currentImageId.value);
});

// 计算当前图片在列表中的索引，用于 UI 进度和边界判断
const currentImageIndex = computed(() => {
  if (currentImageId.value === undefined) return -1;
  return images.value.findIndex((img) => img.id === currentImageId.value);
});

// 构造交替预载图片列表（后1张、前1张、后2张、前2张……）以支持反向预载
const preloadImages = computed(() => {
  const index = currentImageIndex.value;
  if (index === -1) return [];

  const list: ImageFragment[] = [];
  const len = images.value.length;
  const maxOffset = Math.max(index, len - 1 - index);

  for (let offset = 1; offset <= maxOffset; offset++) {
    const nextIdx = index + offset;
    const prevIdx = index - offset;

    if (nextIdx < len) {
      list.push(images.value[nextIdx]);
    }
    if (prevIdx >= 0) {
      list.push(images.value[prevIdx]);
    }
  }
  return list;
});

function prevImage() {
  const index = currentImageIndex.value;
  if (index > 0) {
    const img = images.value[index - 1];
    if (img) {
      currentImageId.value = img.id;
      viewerHash.value = img.filename;
    }
  }
}

// 预载下一页：若目标图片是当前列表的最后一张，且有后续页面未加载，则在后台静默发起加载请求
function checkAndFetchMore(index: number) {
  if (
    index !== -1 &&
    index === images.value.length - 1 &&
    hasNextPage.value &&
    !loading.value
  ) {
    fetchMore();
  }
}

async function nextImage() {
  const index = currentImageIndex.value;
  if (index === -1) return;

  if (index < images.value.length - 1) {
    const nextIdx = index + 1;
    const img = images.value[nextIdx];
    if (img) {
      currentImageId.value = img.id;
      viewerHash.value = img.filename;
      checkAndFetchMore(nextIdx);
    }
  } else if (hasNextPage.value && !loading.value) {
    // 浏览到当前页最后一张且还有下一页时，触发分页加载并等待数据追加
    const prevLength = images.value.length;
    await fetchMore();
    await nextTick();
    // 确保有新图片加载进来后，自动过渡跳转到新页面的第一张图片
    if (images.value.length > prevLength) {
      const img = images.value[prevLength];
      if (img) {
        currentImageId.value = img.id;
        viewerHash.value = img.filename;
        checkAndFetchMore(prevLength);
      }
    }
  }
}

// #region 移动匹配图片模块
const moveImagesDialog = useModalDialog();
const imageViewerDialog = useModalFullscreen();
// #endregion

// 查看器打开时：左右方向键切换图片，Esc 关闭查看器

useHotkeys(
  {
    arrowleft: () => {
      prevImage();
    },
    arrowright: () => {
      nextImage();
    },
    home: () => {
      if (images.value.length > 0) {
        const img = images.value[0];
        currentImageId.value = img.id;
        viewerHash.value = img.filename;
      }
    },
    end: async () => {
      let pageCount = 0;
      while (hasNextPage.value) {
        if (pageCount > 0 && pageCount % 10 === 0) {
          const shouldContinue = confirm(
            `已经自动加载了 ${pageCount} 页图片，是否继续加载？`,
          );
          if (!shouldContinue) {
            break;
          }
        }
        const prevLength = images.value.length;
        await fetchMore();
        await nextTick();
        if (images.value.length <= prevLength) {
          break;
        }
        pageCount++;
      }
      if (images.value.length > 0) {
        const img = images.value[images.value.length - 1];
        currentImageId.value = img.id;
        viewerHash.value = img.filename;
      }
    },
  },
  {
    allowInInputs: true,
    scope: imageViewerDialog.scopeId,
    category: "图片浏览",
  },
);

useHotkeys(
  {
    "ctrl+a": (e) => {
      const selection = window.getSelection()?.toString();
      if (selection) {
        return;
      }
      e.preventDefault();
      e.stopPropagation();
      if (!isBulkMode.value) {
        isBulkMode.value = true;
      }
      selectAll();
    },
  },
  {
    preventDefault: false,
    stopPropagation: false,
    description: "全选所有图片",
    category: "批量操作",
  },
);

useHotkeys(
  {
    "ctrl+c": (e) => {
      const selection = window.getSelection()?.toString();
      if (selection) {
        return;
      }
      e.preventDefault();
      e.stopPropagation();
      copySelectedImages();
    },
  },
  {
    preventDefault: false,
    stopPropagation: false,
    description: "复制选中的图片文件",
    enabled: computed(() => isBulkMode.value && !!selectedFilterBy.value),
    category: "批量操作",
  },
);

useHotkeys(
  {
    escape: () => {
      isBulkMode.value = false;
    },
  },
  {
    allowInInputs: false,
    description: "退出批量模式",
    enabled: isBulkMode,
    category: "批量操作",
  },
);

// 批量模式下删除选中图片（Delete 键）
const isBulkDeleting = ref(false);

useHotkeys(
  {
    delete: async () => {
      if (!selectedFilterBy.value || isBulkDeleting.value) return;

      isBulkDeleting.value = true;
      try {
        await trashImages(props.directoryId, selectedFilterBy.value);
        // 删除成功后取消选中，避免对已删除图片的后续操作
        deselectAll();
      } catch (err) {
        showNotification(
          err instanceof Error ? err.message : "批量删除图片失败",
          "error",
        );
      } finally {
        isBulkDeleting.value = false;
      }
    },
  },
  {
    allowInInputs: false,
    description: "删除选中的图片及其配套文件",
    enabled: computed(
      () =>
        isBulkMode.value &&
        !!selectedFilterBy.value &&
        !isBulkDeleting.value &&
        !imageViewerDialog.visible.value &&
        !moveImagesDialog.visible.value,
    ),
    category: "批量操作",
  },
);

// 批量模式下 Delete 键删除 Rar 状态也纳入考量
const isBulkActionEnabled = computed(() => {
  return (
    isBulkMode.value &&
    !!selectedFilterBy.value &&
    !isBulkDeleting.value &&
    !imageViewerDialog.visible.value &&
    !moveImagesDialog.visible.value
  );
});

// 绑定快捷键 Ctrl+0~5 以及 小键盘 0~5 用于批量修改评分
for (let r = 0; r <= 5; r++) {
  useHotkeys(
    [
      {
        keys: [`ctrl+digit${r}`, `numpad${r}`],
        handler: (e) => {
          e.preventDefault();
          e.stopPropagation();
          bulkSetRating(r);
        },
      },
    ],
    {
      description: `批量设置评分为 ${r} 星`,
      category: "批量操作",
      enabled: isBulkActionEnabled,
    },
  );
}

// 批量设置颜色标签快捷键 Ctrl+Shift+1~9，以及清除标签 Ctrl+Shift+0
const colorNames = Object.keys(PRESET_COLORS);
for (let i = 0; i < 9; i++) {
  const colorName = colorNames[i];
  const colorCn = COLOR_NAMES_CN[colorName] || colorName;
  useHotkeys(
    {
      [`ctrl+shift+${i + 1}`]: (e) => {
        e.preventDefault();
        e.stopPropagation();
        bulkSetLabel(colorName);
      },
    },
    {
      description: `批量设置标签为 ${colorCn}`,
      category: "批量操作",
      enabled: isBulkActionEnabled,
    },
  );
}

useHotkeys(
  {
    "ctrl+shift+0": (e) => {
      e.preventDefault();
      e.stopPropagation();
      bulkSetLabel("");
    },
  },
  {
    description: "批量清除图片标签",
    category: "批量操作",
    enabled: isBulkActionEnabled,
  },
);
// #endregion

// #region URL Hash 状态持久化（文件名方式，便于跨筛选条件搜索）
const viewerHash = useLocationHash();

function openViewer(image: ImageFragment) {
  currentImageId.value = image.id;
  viewerHash.value = image.filename;
  imageViewerDialog.open();

  // 开启查看器时，检查目标图片是否为当前列表的最后一张
  const index = images.value.findIndex((img) => img.id === image.id);
  checkAndFetchMore(index);
}

function closeViewer() {
  imageViewerDialog.close();
}

function handleViewerAfterLeave() {
  currentImageId.value = undefined;
  viewerHash.value = "";
}

function tryOpenViewerByFilename(filename: string): boolean {
  console.log("try open", filename);
  const image = images.value.find(
    (img: ImageFragment) => img.filename === filename,
  );
  if (image) {
    openViewer(image);
    return true;
  }
  return false;
}

async function searchAndOpenViewer(filename: string) {
  if (tryOpenViewerByFilename(filename)) {
    return;
  }
  clearFilters();
  searchQuery.value = filename;
  await waitLoading();
  tryOpenViewerByFilename(filename);
}

async function waitLoading() {
  await nextTick();
  using stack = new DisposableStack();
  await new Promise<void>((resolve) => {
    stack.defer(
      watchEffect(() => {
        if (!loading.value) {
          resolve();
        }
      }),
    );
  });
}

onMounted(async () => {
  if (viewerHash.value) {
    await waitLoading();
    searchAndOpenViewer(viewerHash.value);
  }
});

// #endregion

// #region 响应 NoteList 打开图片查看器的事件
watchEffect((onCleanup) => {
  const unsubscribe = openImageViewerByFilename.subscribe((event) => {
    searchAndOpenViewer(event.detail.filename);
  });
  onCleanup(unsubscribe);
});
// #endregion
</script>

<style scoped>
/* 底部批量管理栏的升降过渡动画 */
.slide-up-enter-active,
.slide-up-leave-active {
  transition: all 0.3s cubic-bezier(0.16, 1, 0.3, 1);
}
.slide-up-enter-from {
  opacity: 0;
  transform: translateY(20px) scale(0.95);
}
.slide-up-leave-to {
  opacity: 0;
  transform: translateY(20px) scale(0.95);
}
</style>
