<script setup lang="ts">
import type { FacetGroup, FacetKey, FacetSelection } from "@/utils/searchFacets";

/**
 * 手机筛选面板里的筛选（画板 10）：每组一个组名，选项是 36 高的胶囊「2160p  8」；
 * 选中的主色底、主色描边；条数为 0 的变灰、不能点。
 */
defineProps<{ groups: FacetGroup[]; sel: FacetSelection }>();

const emit = defineEmits<{ toggle: [key: FacetKey, value: string] }>();
</script>

<template>
  <div class="es-pills">
    <section
      v-for="g in groups"
      :key="g.key"
      class="es-pills__group"
      :data-testid="`es-mfacet-${g.key}`">
      <h3 class="es-pills__label">{{ g.label }}</h3>
      <div class="es-pills__row">
        <button
          v-for="o in g.options"
          :key="o.value"
          type="button"
          class="es-pills__pill"
          :class="{ 'is-on': sel[g.key]?.includes(o.value) }"
          :aria-pressed="Boolean(sel[g.key]?.includes(o.value))"
          :disabled="o.count === 0 && !sel[g.key]?.includes(o.value)"
          @click="emit('toggle', g.key, o.value)">
          <span>{{ o.label }}</span>
          <span class="es-pills__n">{{ o.count }}</span>
        </button>
      </div>
    </section>
  </div>
</template>

<style scoped>
.es-pills__group {
  display: flex;
  flex-direction: column;
  min-width: 0;
}

.es-pills {
  display: flex;
  flex-direction: column;
  gap: 18px;
}

.es-pills__label {
  margin: 0 0 8px;
  font-size: var(--pt-fz-sm);
  font-weight: 700;
  color: var(--pt-t3);
}

.es-pills__row {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

/* 胶囊 h36 r18、左右 14、12.5（落地 13）字；名字与条数之间两个空格宽 */
.es-pills__pill {
  display: inline-flex;
  gap: 8px;
  align-items: center;
  height: 36px;
  padding: 0 14px;
  font: inherit;
  font-size: var(--pt-fz-body);
  color: var(--pt-t2);
  cursor: pointer;
  background: var(--pt-surface);
  border: 1px solid var(--pt-border);
  border-radius: 18px;
}

.es-pills__pill.is-on {
  font-weight: 500;
  color: var(--pt-p);
  background: var(--pt-p-soft);
  border-color: color-mix(in srgb, var(--pt-p) 55%, transparent);
}

.es-pills__pill:disabled {
  cursor: not-allowed;
  opacity: 0.45;
}

.es-pills__n {
  font-variant-numeric: tabular-nums;
  color: var(--pt-t3);
}

.es-pills__pill.is-on .es-pills__n {
  color: inherit;
}
</style>
