import { computed, defineComponent, h, type PropType } from "vue";
import { LUCIDE_ICONS, type LucideIconName } from "../icons/lucide";

/**
 * lucide 图标。
 *
 * 写成渲染函数而不是 .vue 模板，是因为图标体是一段 SVG 子元素字符串，
 * 模板里只能靠 v-html 注入；渲染函数直接给 `innerHTML` DOM prop，少一层
 * 模板编译，也不用为一条 lint 规则开例外。数据来自本地常量表，不接受
 * 外部输入，所以这里没有注入面。
 *
 * 描边宽固定 1.75（设计系统 §6），单位是 viewBox 的用户单位而不是 px。
 * 浏览器按 viewBox 把 24 个单位映射到 size px，描边跟着一起缩，所以这里
 * **不能**再自己乘一遍 size/24 —— 那会让比例变成平方，13px 图标的实际
 * 描边只有 0.51px（设计值 1.75 × 13/24 ≈ 0.95px），细到几乎看不见。
 * Penpot 里要手写回去是因为 createShapeFromSvg 丢掉了 viewBox 缩放。
 */
/** 设计系统 §6 的描边宽，viewBox 用户单位（lucide 原生是 2，稿子上收到 1.75）。 */
const PT_ICON_STROKE = 1.75;

export default defineComponent({
  name: "PtIcon",
  props: {
    name: { type: String as PropType<LucideIconName | string>, required: true },
    size: { type: [Number, String], default: 16 },
    /** 覆盖自动计算的描边宽。只在个别需要更重笔画的场合用。 */
    strokeWidth: { type: Number, default: 0 },
  },
  setup(props) {
    const px = computed(() => {
      const n = typeof props.size === "number" ? props.size : Number.parseFloat(props.size);
      return Number.isFinite(n) && n > 0 ? n : 16;
    });

    const sw = computed(() => (props.strokeWidth > 0 ? props.strokeWidth : PT_ICON_STROKE));

    const body = computed(() => {
      const found = LUCIDE_ICONS[props.name as LucideIconName];
      if (found) return found;
      if (import.meta.env.DEV) {
        // 缺图标时不静默画空白：设计稿用过的名字必须都在表里
        console.warn(`[PtIcon] 图标表里没有 "${props.name}"，请检查 src/icons/lucide.ts`);
      }
      return "";
    });

    return () =>
      h("svg", {
        class: "pt-icon",
        width: px.value,
        height: px.value,
        viewBox: "0 0 24 24",
        fill: "none",
        stroke: "currentColor",
        "stroke-width": sw.value,
        "stroke-linecap": "round",
        "stroke-linejoin": "round",
        // 图标一律按装饰处理；需要无障碍名称时由外层的按钮/链接带 aria-label
        "aria-hidden": "true",
        focusable: "false",
        innerHTML: body.value,
      });
  },
});
