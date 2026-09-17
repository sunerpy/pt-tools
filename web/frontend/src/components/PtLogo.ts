import { computed, defineComponent, h, type PropType } from "vue";

/**
 * pt-tools 品牌标识。
 *
 * 几何完全照抄 public/logo.svg（由 brand/build.py 生成，不得手改），
 * 这里只是把同样的两个图元写成组件，好处是能跟着主题换色、也不用为一个
 * 16px 的图标发一次 HTTP 请求。1024 的画布保持不变，浏览器按 viewBox
 * 等比缩放，描边宽和圆角都不需要像 Penpot 那样手工修正。
 *
 * 两种变体，按 `docs/brand.md`「用哪个变体」选，判断标准是**表面明暗**：
 *   mono   —— 单色，继承 currentColor，用在深色表面（rail 取 --pt-chrome-t1、
 *             深色模式下的导航头与移动顶栏取 --pt-t1）
 *   plated —— logo.svg 的等价物：自带深色底板 + 品牌原色，用在浅色表面
 *
 * brand.md 的硬性规则里禁止改色，所以这里没有「跟着配色走」的双色变体：
 * 人字恒为 #14B8A6、光标恒为 #F97316，只有 mono 这一种整体单色的例外。
 */
export type PtLogoVariant = "mono" | "plated";

const CHEVRON_D = "M192 320 L384 512 L192 704";

export default defineComponent({
  name: "PtLogo",
  props: {
    size: { type: [Number, String], default: 24 },
    variant: { type: String as PropType<PtLogoVariant>, default: "mono" },
    /** 给出可访问名称；不给就当装饰图处理（外层通常已经有文字或 aria-label） */
    title: { type: String, default: "" },
  },
  setup(props) {
    const px = computed(() => {
      const n = typeof props.size === "number" ? props.size : Number.parseFloat(props.size);
      return Number.isFinite(n) && n > 0 ? n : 24;
    });

    const plated = computed(() => props.variant === "plated");
    const chevronColor = computed(() => (plated.value ? "#14b8a6" : "currentColor"));
    const cursorColor = computed(() => (plated.value ? "#f97316" : "currentColor"));

    return () => {
      const labelled = props.title.length > 0;
      const children = [];

      if (labelled) children.push(h("title", null, props.title));

      if (plated.value) {
        children.push(
          h("rect", {
            x: 0,
            y: 0,
            width: 1024,
            height: 1024,
            rx: 204.8,
            ry: 204.8,
            fill: "#0b1220",
          }),
        );
      }

      children.push(
        h("path", {
          d: CHEVRON_D,
          fill: "none",
          stroke: chevronColor.value,
          "stroke-width": 128,
          "stroke-linecap": "round",
          "stroke-linejoin": "round",
        }),
        h("rect", { x: 576, y: 384, width: 320, height: 256, fill: cursorColor.value }),
      );

      return h(
        "svg",
        {
          class: ["pt-logo", `pt-logo--${props.variant}`],
          width: px.value,
          height: px.value,
          viewBox: "0 0 1024 1024",
          fill: "none",
          role: labelled ? "img" : undefined,
          "aria-label": labelled ? props.title : undefined,
          "aria-hidden": labelled ? undefined : "true",
          focusable: "false",
        },
        children,
      );
    };
  },
});
