<script setup lang="ts">
import PtIcon from "@/components/PtIcon";
import type { FacetGroup, FacetKey, FacetSelection } from "@/utils/searchFacets";

/**
 * 带计数的筛选（画板 06 左栏 facet）：每组一个组名，下面一行一个选项——勾选框、名字、右边的条数。
 * 条数为 0 的选项变灰、不能勾；勾上的名字加粗。关键字搜索的「筛选」浮层也用它。
 */
defineProps<{ groups: FacetGroup[]; sel: FacetSelection }>();

const emit = defineEmits<{ toggle: [key: FacetKey, value: string] }>();
</script>

<template>
  <div class="es-facets">
    <section
      v-for="g in groups"
      :key="g.key"
      class="es-facets__group"
      :data-testid="`es-facet-${g.key}`">
      <h3 class="es-facets__label">{{ g.label }}</h3>
      <button
        v-for="o in g.options"
        :key="o.value"
        type="button"
        role="checkbox"
        class="es-facets__opt"
        :class="{ 'is-on': sel[g.key]?.includes(o.value) }"
        :aria-checked="Boolean(sel[g.key]?.includes(o.value))"
        :disabled="o.count === 0 && !sel[g.key]?.includes(o.value)"
        :data-testid="`es-facet-${g.key}-${o.value}`"
        @click="emit('toggle', g.key, o.value)">
        <span class="es-facets__box" aria-hidden="true">
          <PtIcon v-if="sel[g.key]?.includes(o.value)" name="check" :size="10" :stroke-width="3" />
        </span>
        <span class="es-facets__name">{{ o.label }}</span>
        <span class="es-facets__n">{{ o.count }}</span>
      </button>
    </section>
  </div>
</template>

<style scoped>
.es-facets {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.es-facets__group {
  display: flex;
  flex-direction: column;
  min-width: 0;
}

.es-facets__label {
  margin: 0 0 3px;
  font-size: var(--pt-fz-label);
  font-weight: 700;
  line-height: var(--pt-lh-row);
  color: var(--pt-t3);
}

/* 选项一行 24 高：勾选框 14 在左，名字 12.5（落地 13）在 22，条数右对齐 */
.es-facets__opt {
  display: flex;
  gap: 8px;
  align-items: center;
  width: 100%;
  height: 24px;
  padding: 0;
  font: inherit;
  color: var(--pt-t2);
  text-align: left;
  cursor: pointer;
  background: transparent;
  border: 0;
}

.es-facets__opt:disabled {
  cursor: not-allowed;
  opacity: 0.45;
}

.es-facets__box {
  display: inline-flex;
  flex: 0 0 auto;
  align-items: center;
  justify-content: center;
  width: 14px;
  height: 14px;
  color: var(--pt-on-p);
  background: var(--pt-surface);
  border: 1px solid var(--pt-border-strong);
  border-radius: 3px;
}

.es-facets__opt.is-on .es-facets__box {
  background: var(--pt-p);
  border-color: var(--pt-p);
}

.es-facets__name {
  flex: 1 1 auto;
  min-width: 0;
  overflow: hidden;
  font-size: var(--pt-fz-body);
  line-height: 1.4;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.es-facets__opt.is-on .es-facets__name {
  font-weight: 500;
  color: var(--pt-t1);
}

.es-facets__n {
  flex: 0 0 auto;
  font-size: var(--pt-fz-label);
  font-variant-numeric: tabular-nums;
  color: var(--pt-t3);
}

.es-facets__opt:not(:disabled):hover .es-facets__name {
  color: var(--pt-t1);
}
</style>
