# 文档截图用的图片

`docs-screens.mjs` 拍首页演示截图时，条目「长夜将尽」的海报与背景图用的是这两张。这部剧是编的，两张图是
2026-11-09 用 Z-Image Turbo（Apache-2.0）生成的，再缩小、转成 WebP。截图时页面向一个不存在的主机
（`http://ptt-docs.invalid`）请求图片，CDP 拦下请求、用这里的文件回答，不会访问 TMDB。

`long-night-poster.webp`：生成尺寸 768 × 1152，种子 20261109，提示词：

> Cinematic TV crime drama key art, vertical poster composition, a lone detective in a dark
> raincoat standing at the end of a wet harbor pier at night, seen from behind, a lighthouse beam
> cutting through heavy rain, a small coastal town with a few warm windows behind him, moody teal
> and amber color grade, fine film grain, no text, no letters, no logo

`long-night-backdrop.webp`：生成尺寸 1280 × 720，种子 20261110，提示词：

> Wide cinematic film still of a rainy coastal harbor town at night, a lighthouse on a distant
> cliff with its beam sweeping through low clouds, wet street reflecting amber streetlights, small
> fishing boats moored in dark water, teal and amber color grade, fine film grain, no people, no
> text, no letters
