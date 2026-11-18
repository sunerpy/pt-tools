import { computed, defineComponent, h } from "vue";

import { useThemeStore } from "../stores/theme";

/**
 * pt-tools 品牌标识：超椭圆底板上的种子徽标（docs/brand.md）。
 *
 * 图形取自 public/logo.svg 与 public/logo-dark.svg（由 firlab/brand/pt-tools/build.py 生成，逐字节复制，不得手改）。
 * 按表面明暗选变体：明亮主题用墨黑底板的 logo.svg，暗黑主题用纯白底板的 logo-dark.svg，跟着主题切换。
 * 两张图带渐变与裁切，直接用 <img> 引用文件：同一页面里侧栏与导航各挂一个，内联的话渐变的 id 会互相覆盖。
 * 两个文件在 server.go 的根路径清单里，未登录也能取到。
 */
export default defineComponent({
  name: "PtLogo",
  props: {
    size: { type: [Number, String], default: 24 },
    /** 给出可访问名称；不给就当装饰图处理（外层通常已经有文字或 aria-label） */
    title: { type: String, default: "" },
  },
  setup(props) {
    const theme = useThemeStore();
    const px = computed(() => {
      const n = typeof props.size === "number" ? props.size : Number.parseFloat(props.size);
      return Number.isFinite(n) && n > 0 ? n : 24;
    });

    return () =>
      h("img", {
        class: "pt-logo",
        src: theme.isDark ? "/logo-dark.svg" : "/logo.svg",
        width: px.value,
        height: px.value,
        alt: props.title,
        "aria-hidden": props.title ? undefined : "true",
        draggable: "false",
      });
  },
});
