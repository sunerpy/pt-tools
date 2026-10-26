/*
 * 路径输入框的下拉选项：下载器在「下载器设置」里预设的目录（别名、是不是默认）与下载器的名字。
 * 同一时刻只读一次（同一页上几个输入框共用一次请求）；每有输入框挂上就重新读，设置里刚加的目录不用刷新网页。
 */
import {
  type DownloaderDirectory,
  type DownloaderSetting,
  downloaderDirectoriesApi,
  downloadersApi,
} from "@/api";
import { onMounted, ref } from "vue";

export interface PathOption {
  path: string;
  alias: string;
  isDefault: boolean;
  downloaderId: number;
  downloader: string;
}

const directories = ref<Record<number, DownloaderDirectory[]>>({});
const downloaders = ref<DownloaderSetting[]>([]);
let pending: Promise<void> | null = null;

/** loadDownloaderPaths 读预设目录与下载器列表；哪一样读不到就保留上次读到的 */
export function loadDownloaderPaths(): Promise<void> {
  if (!pending) {
    pending = (async () => {
      const [dirs, dls] = await Promise.allSettled([
        downloaderDirectoriesApi.listAll(),
        downloadersApi.list(),
      ]);
      if (dirs.status === "fulfilled") directories.value = dirs.value ?? {};
      if (dls.status === "fulfilled") downloaders.value = dls.value ?? [];
    })().finally(() => {
      pending = null;
    });
  }
  return pending;
}

/** pathOptionsFor 是这些下载器的预设目录（每台的默认目录在前）；没指定下载器（或是 0）时用 pt-tools 的默认下载器 */
export function pathOptionsFor(ids: number | number[] | null | undefined): PathOption[] {
  let list = (Array.isArray(ids) ? ids : [ids]).filter(
    (id): id is number => typeof id === "number" && id > 0,
  );
  if (!list.length) {
    const def =
      downloaders.value.find((d) => d.is_default && d.enabled) ??
      downloaders.value.find((d) => d.is_default);
    if (def?.id) list = [def.id];
  }
  const out: PathOption[] = [];
  for (const id of new Set(list)) {
    const name = downloaders.value.find((d) => d.id === id)?.name ?? `下载器 ${id}`;
    const dirs = [...(directories.value[id] ?? [])].sort(
      (a, b) => Number(b.is_default) - Number(a.is_default),
    );
    for (const d of dirs) {
      out.push({
        path: d.path,
        alias: d.alias ?? "",
        isDefault: d.is_default,
        downloaderId: id,
        downloader: name,
      });
    }
  }
  return out;
}

/** allPathOptions 是所有下载器的预设目录（同一个路径只列一次），给不限定下载器的地方用 */
export function allPathOptions(): PathOption[] {
  const ids = downloaders.value.map((d) => d.id).filter((id): id is number => !!id);
  const seen = new Set<string>();
  return pathOptionsFor(ids.length ? ids : Object.keys(directories.value).map(Number)).filter(
    (o) => !seen.has(o.path) && seen.add(o.path),
  );
}

/** useDownloaderPaths 在用到路径选项的组件里调用：挂上时读一次 */
export function useDownloaderPaths() {
  onMounted(() => {
    void loadDownloaderPaths();
  });
  return { pathOptionsFor, allPathOptions };
}
