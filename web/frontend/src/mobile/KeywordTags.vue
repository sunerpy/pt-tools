<script setup lang="ts">
/*
 * 标题包含、标题排除（追剧方案 F3，3.3.1）：每个词显示成一个标签，点 × 删掉，下面的输入框加词。
 * 底层仍是一行一个词的文字，保存的值和以前一样；加词的规则（去重、长度）同网页（utils/keywordList.ts）。
 */
import { ElMessage } from "element-plus";
import { computed, ref } from "vue";
import { MAX_KEYWORDS_BYTES, addKeyword } from "@/utils/keywordList";
import { useWords } from "./i18n";
import { rawWords } from "./subscriptionText";

const model = defineModel<string>({ required: true });
const props = defineProps<{
  label: string;
  placeholder: string;
  testid: string;
  disabled?: boolean;
}>();
const t = useWords();
const typed = ref("");
const words = computed(() => rawWords(model.value));

function remove(i: number) {
  const next = [...words.value];
  next.splice(i, 1);
  model.value = next.join("\n");
}

/** 输入框里写的词加进去：逗号或换行分开的算几个词，已经有的不加，加了超过上限时不加 */
function add() {
  let text = model.value;
  let warned = false;
  for (const w of typed.value
    .split(/[\n,，]/)
    .map((x) => x.trim())
    .filter(Boolean)) {
    const r = addKeyword(text, w);
    if (r.ok) {
      text = r.text;
      continue;
    }
    if (warned) continue;
    warned = true;
    if (r.reason === "duplicate")
      ElMessage.warning(t(`已经有「${w}」了`, `"${w}" is already there`));
    else if (r.reason === "too_long")
      ElMessage.warning(
        t(
          `加了以后超过 ${MAX_KEYWORDS_BYTES} 字节：先删掉几个词`,
          `More than ${MAX_KEYWORDS_BYTES} bytes: remove some words first`,
        ),
      );
  }
  model.value = text;
  typed.value = "";
}
</script>

<template>
  <el-form-item :label="label" :data-testid="testid">
    <div class="kw">
      <div v-if="words.length" class="kw__tags">
        <el-tag
          v-for="(w, i) in words"
          :key="`${i}:${w}`"
          :closable="!props.disabled"
          disable-transitions
          :data-testid="`${testid}-tag-${i}`"
          @close="remove(i)">
          {{ w }}
        </el-tag>
      </div>
      <p v-else class="mobile-note kw__empty">{{ t("没有", "None") }}</p>
      <div class="kw__add">
        <el-input
          v-model="typed"
          :placeholder="placeholder"
          :data-testid="`${testid}-input`"
          @keydown.enter.prevent="add" />
        <el-button :disabled="!typed.trim()" :data-testid="`${testid}-add`" @click="add">{{
          t("添加", "Add")
        }}</el-button>
      </div>
    </div>
  </el-form-item>
</template>

<style scoped>
.kw {
  display: flex;
  flex-direction: column;
  gap: 8px;
  width: 100%;
  min-width: 0;
}
.kw__tags {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}
.kw__tags .el-tag {
  height: auto;
  min-height: 32px;
  max-width: 100%;
  white-space: normal;
  overflow-wrap: anywhere;
  font-size: var(--pt-fz-sm);
}
.kw__empty {
  margin: 0;
}
.kw__add {
  display: flex;
  gap: 8px;
}
.kw__add .el-input {
  flex: 1;
  min-width: 0;
}
.kw__add .el-button {
  margin: 0;
}
</style>
