/**
 * check-sites.ts
 *
 * 检查浏览器扩展中的内置站点列表是否与 Go 项目中的站点定义一致。
 * 在扩展打包前自动运行，不一致时报错退出。
 *
 * 用法: node --experimental-strip-types scripts/check-sites.ts
 */

import { readdirSync, readFileSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const __dirname: string = dirname(fileURLToPath(import.meta.url));
const ROOT: string = resolve(__dirname, "..");

// 1. 从 Go site/v2/definitions/*.go 提取站点 ID
const definitionsDir: string = join(ROOT, "site", "v2", "definitions");
const goFiles: string[] = readdirSync(definitionsDir).filter(
  (f: string) => f.endsWith(".go") && !f.includes("_test") && !f.includes("_fixture"),
);

const goSiteIds: Set<string> = new Set();
const idPattern: RegExp = /ID:\s*"([^"]+)"/g;

for (const file of goFiles) {
  const content: string = readFileSync(join(definitionsDir, file), "utf-8");
  let match: RegExpExecArray | null;
  while ((match = idPattern.exec(content)) !== null) {
    goSiteIds.add(match[1]);
  }
}

// 2. 从扩展 constants.ts 提取站点 ID
const constantsPath: string = join(
  ROOT,
  "tools",
  "browser-extension",
  "src",
  "core",
  "constants.ts",
);
const constantsContent: string = readFileSync(constantsPath, "utf-8");

const extSiteIds: Set<string> = new Set();
const extIdPattern: RegExp = /id:\s*["']([^"']+)["']/g;
let extMatch: RegExpExecArray | null;
while ((extMatch = extIdPattern.exec(constantsContent)) !== null) {
  extSiteIds.add(extMatch[1]);
}

// 3. 比较
const missingInExtension: string[] = [...goSiteIds].filter((id: string) => !extSiteIds.has(id));
const extraInExtension: string[] = [...extSiteIds].filter((id: string) => !goSiteIds.has(id));

console.log(`Go definitions: ${[...goSiteIds].sort().join(", ")} (${goSiteIds.size})`);
console.log(`Extension:      ${[...extSiteIds].sort().join(", ")} (${extSiteIds.size})`);
console.log("");

let hasError: boolean = false;

if (missingInExtension.length > 0) {
  console.error(`❌ Sites in Go but MISSING in extension: ${missingInExtension.join(", ")}`);
  console.error(`   → Add these to tools/browser-extension/src/core/constants.ts KNOWN_SITES`);
  hasError = true;
}

if (extraInExtension.length > 0) {
  console.error(`❌ Sites in extension but NOT in Go: ${extraInExtension.join(", ")}`);
  console.error(
    `   → Remove these from tools/browser-extension/src/core/constants.ts or add Go definitions`,
  );
  hasError = true;
}

if (hasError) {
  process.exit(1);
} else {
  console.log("✅ Built-in sites are in sync.");
}
