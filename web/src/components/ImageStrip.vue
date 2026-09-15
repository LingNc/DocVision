<script setup lang="ts">
// 图片条：一条缩略图一行（附件段 / 看图调用的输出 / 轨迹图片步共用）。
// 点缩略图进灯箱；ref 原样作 alt 与悬浮说明（与旧页 thumbImg 一致）。
import { openLightbox } from '../state'
import { mediaURL } from '../legacy/timeline'

defineProps<{ images?: string[] }>()

function show(ref: string) {
  openLightbox(mediaURL(ref), ref)
}
</script>

<template>
  <div class="images">
    <img
      v-for="img in images || []"
      :key="img"
      class="thumb"
      :src="mediaURL(img)"
      :alt="img"
      loading="lazy"
      :title="img"
      @click="show(img)"
    >
  </div>
</template>
