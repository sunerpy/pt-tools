/*
 * 海报用的 TMDB 图片地址：「媒体识别」里换了图片地址时，网页上的海报也从换的地址加载。
 * 同一个页面里只读一次设置；读不到（没有登录、接口出错）时保持官方地址，下次调用再试。
 */
import { mediaApi } from "@/api";
import { setTmdbImageBase } from "@/utils/media";

let loading: Promise<void> | null = null;

/** loadTmdbImageBase 读设置里的图片地址并让 posterURL 用它 */
export function loadTmdbImageBase(): Promise<void> {
  if (!loading) {
    loading = (async () => {
      try {
        const s = await mediaApi.settings();
        setTmdbImageBase(s?.image_base_url);
      } catch {
        loading = null;
      }
    })();
  }
  return loading;
}

/** useTmdbImageBase 在显示海报的组件里调用：触发一次读取（不等它，读到以后海报自己换地址） */
export function useTmdbImageBase(): void {
  void loadTmdbImageBase();
}
