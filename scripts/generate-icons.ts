/**
 * generate-icons.ts
 *
 * 从 public/pt-tools.png (1024x1024) 一键生成所有尺寸的图标:
 *   - web/frontend/public/  → favicon.ico, favicon-16x16.png, favicon-32x32.png,
 *                             apple-touch-icon.png (180), android-chrome-192.png,
 *                             android-chrome-512.png
 *   - tools/browser-extension/public/icons/ → icon16.png, icon32.png, icon48.png, icon128.png
 *
 * 用法: node --experimental-strip-types scripts/generate-icons.ts
 * 依赖: sharp (自动安装)
 */

import { execSync } from "node:child_process";
import { existsSync, mkdirSync, writeFileSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const __dirname: string = dirname(fileURLToPath(import.meta.url));
const ROOT: string = resolve(__dirname, "..");
const SOURCE: string = join(ROOT, "public", "pt-tools.png");

interface IconTarget {
  output: string;
  size: number;
}

interface IcoImage {
  size: number;
  data: Buffer;
}

// 确保 sharp 可用
type SharpFn = (input: string) => {
  resize: (
    w: number,
    h: number,
    opts?: Record<string, unknown>,
  ) => {
    png: () => {
      toFile: (path: string) => Promise<void>;
      toBuffer: () => Promise<Buffer>;
    };
  };
};

let sharp: SharpFn;
try {
  sharp = (await import("sharp")).default as SharpFn;
} catch {
  console.log("⏳ Installing sharp...");
  execSync("npm install --no-save sharp", { cwd: ROOT, stdio: "inherit" });
  sharp = (await import("sharp")).default as SharpFn;
}

if (!existsSync(SOURCE)) {
  console.error(`❌ Source icon not found: ${SOURCE}`);
  process.exit(1);
}

const targets: IconTarget[] = [
  // Frontend
  { output: "web/frontend/public/favicon-16x16.png", size: 16 },
  { output: "web/frontend/public/favicon-32x32.png", size: 32 },
  { output: "web/frontend/public/apple-touch-icon.png", size: 180 },
  { output: "web/frontend/public/android-chrome-192x192.png", size: 192 },
  { output: "web/frontend/public/android-chrome-512x512.png", size: 512 },
  { output: "web/frontend/public/logo.png", size: 512 },

  // Browser extension
  { output: "tools/browser-extension/public/icons/icon16.png", size: 16 },
  { output: "tools/browser-extension/public/icons/icon32.png", size: 32 },
  { output: "tools/browser-extension/public/icons/icon48.png", size: 48 },
  { output: "tools/browser-extension/public/icons/icon128.png", size: 128 },
];

console.log(`🎨 Source: ${SOURCE} (1024x1024)`);
console.log("");

for (const target of targets) {
  const outputPath: string = join(ROOT, target.output);
  const outputDir: string = dirname(outputPath);
  if (!existsSync(outputDir)) {
    mkdirSync(outputDir, { recursive: true });
  }

  await sharp(SOURCE)
    .resize(target.size, target.size, {
      fit: "contain",
      background: { r: 0, g: 0, b: 0, alpha: 0 },
    })
    .png()
    .toFile(outputPath);

  console.log(`  ✅ ${target.output} (${target.size}x${target.size})`);
}

// Generate favicon.ico (multi-size ICO: 16 + 32)
const ico16: Buffer = await sharp(SOURCE).resize(16, 16).png().toBuffer();
const ico32: Buffer = await sharp(SOURCE).resize(32, 32).png().toBuffer();

function createIco(images: IcoImage[]): Buffer {
  const count: number = images.length;
  const headerSize: number = 6;
  const entrySize: number = 16;
  const dataOffset: number = headerSize + count * entrySize;

  let totalSize: number = dataOffset;
  for (const img of images) {
    totalSize += img.data.length;
  }

  const buffer: Buffer = Buffer.alloc(totalSize);
  // ICO header
  buffer.writeUInt16LE(0, 0); // reserved
  buffer.writeUInt16LE(1, 2); // type: ICO
  buffer.writeUInt16LE(count, 4); // count

  let offset: number = dataOffset;
  for (let i = 0; i < count; i++) {
    const img: IcoImage = images[i];
    const entryOffset: number = headerSize + i * entrySize;
    buffer.writeUInt8(img.size >= 256 ? 0 : img.size, entryOffset); // width
    buffer.writeUInt8(img.size >= 256 ? 0 : img.size, entryOffset + 1); // height
    buffer.writeUInt8(0, entryOffset + 2); // color palette
    buffer.writeUInt8(0, entryOffset + 3); // reserved
    buffer.writeUInt16LE(1, entryOffset + 4); // color planes
    buffer.writeUInt16LE(32, entryOffset + 6); // bits per pixel
    buffer.writeUInt32LE(img.data.length, entryOffset + 8); // data size
    buffer.writeUInt32LE(offset, entryOffset + 12); // data offset
    img.data.copy(buffer, offset);
    offset += img.data.length;
  }

  return buffer;
}

const icoBuffer: Buffer = createIco([
  { size: 16, data: ico16 },
  { size: 32, data: ico32 },
]);

const faviconPath: string = join(ROOT, "web/frontend/public/favicon.ico");
writeFileSync(faviconPath, icoBuffer);
console.log(`  ✅ web/frontend/public/favicon.ico (16+32 multi-size)`);

console.log("");
console.log("🎉 All icons generated successfully.");
