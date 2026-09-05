#!/usr/bin/env node
import { readFileSync, writeFileSync, readdirSync, existsSync } from "node:fs";
import { join, relative } from "node:path";
import { fileURLToPath } from "node:url";

const compRoot = fileURLToPath(new URL("../ui/components/", import.meta.url));
const outPath = fileURLToPath(new URL("../ui/registry.json", import.meta.url));

function extractDefaultObject(src) {
  const start = src.indexOf("export default {");
  if (start === -1) return null;
  let i = start + "export default {".length;
  let depth = 1;
  let inStr = false;
  let quote = "";
  for (; i < src.length; i++) {
    const ch = src[i];
    if (inStr) {
      if (ch === "\\") i++;
      else if (ch === quote) inStr = false;
      continue;
    }
    if (ch === '"' || ch === "'" || ch === "`") {
      inStr = true;
      quote = ch;
      continue;
    }
    if (ch === "{") depth++;
    else if (ch === "}") {
      depth--;
      if (depth === 0) return src.slice(start + "export default {".length, i);
    }
  }
  return null;
}

function storyNames(src) {
  const names = [];
  const re = /export\s+const\s+([A-Za-z_$][\w$]*)\s*=\s*\(?\s*\(\)\s*=>/g;
  let m;
  while ((m = re.exec(src)) !== null) names.push(m[1]);
  return names;
}

function parseStoriesPath(dir) {
  const file = join(dir, dir.split("/").pop() + ".stories.js");
  return existsSync(file) ? file : null;
}

const components = [];
const dirs = readdirSync(compRoot, { withFileTypes: true })
  .filter((d) => d.isDirectory())
  .map((d) => join(compRoot, d.name))
  .sort();

for (const dir of dirs) {
  const file = parseStoriesPath(dir);
  if (!file) continue;
  const src = readFileSync(file, "utf8");
  const lit = extractDefaultObject(src);
  if (!lit) {
    console.error(`skip ${file}: no export default object`);
    continue;
  }
  const def = new Function(`return ({${lit}\n});`)();
  const llm = (def.parameters && def.parameters.llm) || {};
  const name = def.title.split("/").pop();
  components.push({
    name,
    group: def.title.split("/")[0] || "Components",
    title: def.title,
    path: relative(join(compRoot, "../.."), file).replace(/\\/g, "/"),
    description: llm.description || "",
    useWhen: llm.useWhen || [],
    avoidWhen: llm.avoidWhen || [],
    related: (llm.related || []).sort(),
    stories: storyNames(src),
    module: `UI.${name}`,
  });
}

const registry = {
  generatedAt: new Date().toISOString(),
  version: 1,
  purpose: "LLM UI registry: which component to use when, and what it relates to.",
  componentCount: components.length,
  components,
};

writeFileSync(outPath, JSON.stringify(registry, null, 2) + "\n");
console.log(`registry.json: ${components.length} components -> ${outPath}`);