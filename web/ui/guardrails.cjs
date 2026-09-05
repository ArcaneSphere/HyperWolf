#!/usr/bin/env node
/* ============================================================
   Guardrails — canonical-ownership enforces for the HyperWolf
   front-end. Run: `npm run check` (or `node web/ui/guardrails.cjs`).

   Checks:
   1. Every component dir has the full triple (Name.js/.css/.stories.js).
   2. registry.json is in sync with the component dirs.
   3. index.html references every component exactly once as CSS link and
      once as script; referenced files exist; only the single allowed
      inline style (#sync-bar width:0%) remains.
   4. style.css braces are balanced.
   5. No component OWNED selector tokens (the subject class/id of each
      top-level component rule) appear as rules in style.css — excluding
      comments and a small ALLOW list of intentional page-level
      context rules / dependencies.
   6. Every .js under web/ passes `node --check`.
   7. (warn) registry.json is fresher than the newest stories.js.

   `--discover` prints the offending tokens without failing, so the
   ALLOW list below can be maintained empirically.
   Exit code is nonzero when any check fails.
   ============================================================ */
"use strict";

const fs = require("fs");
const path = require("path");
const cp = require("child_process");

const ROOT = path.join(__dirname, "..", "..");
const WEB = path.join(ROOT, "web");
const COMPONENTS = path.join(WEB, "ui", "components");
const STYLE_CSS = path.join(WEB, "style.css");
const INDEX_HTML = path.join(WEB, "index.html");
const REGISTRY_JSON = path.join(WEB, "ui", "registry.json");

/* Owned-looking selector tokens that intentionally still appear as rules
   in style.css after the dedupe: page-level layout regions (the sticky
   search bar, the sidebar) that components genuinely reference
   contextually. Extend deliberately, never silently. */
const ALLOW = new Set([
  "search-sticky",        // page-level sticky search area (components reference it)
  "sidebar",              // #sidebar page region (HexIcon owns .sidebar-item)
]);

const DISCOVER = process.argv.includes("--discover");

const errors = [];
const warns = [];
const passes = [];
const fail = (m) => errors.push(m);
const warn = (m) => warns.push(m);
const pass = (m) => passes.push(m);

function read(p) {
  return fs.readFileSync(p, "utf8");
}

function stripComments(css) {
  return css.replace(/\/\*[\s\S]*?\*\//g, "");
}

function listDirNames(dir) {
  return fs
    .readdirSync(dir, { withFileTypes: true })
    .filter((d) => d.isDirectory())
    .map((d) => d.name)
    .sort();
}

/* All rule selectors in a CSS file, brace-depth aware: at-rules and
   media/keyframe wrappers are skipped, inner rules are still reported. */
function ruleSelectors(css) {
  const out = [];
  let depth = 0;
  let start = 0;
  for (let i = 0; i < css.length; i++) {
    const ch = css[i];
    if (ch === "{") {
      const sel = css.slice(start, i).trim();
      if (sel && !sel.startsWith("@")) {
        out.push({ sel, line: css.slice(0, start).split("\n").length });
        depth++;
      } else {
        depth++;
      }
      start = i + 1;
    } else if (ch === "}") {
      depth = Math.max(0, depth - 1);
      start = i + 1;
    }
  }
  return out;
}

/* Owned/defined selector tokens: the FIRST .class/#id token of EVERY
   comma-separated simple selector of each rule — i.e. the things a CSS
   file actually claims to style (ancestor classes are skipped). */
function subjectTokens(css) {
  const tokens = new Set();
  for (const { sel } of ruleSelectors(stripComments(css))) {
    for (const simple of sel.split(",")) {
      const tm = simple.trim().match(/[\.#]([A-Za-z][\w-]*)/);
      if (tm) tokens.add(tm[1]);
    }
  }
  return tokens;
}

/* Same, but records {token, rule, line} for reporting on the page css. */
function subjectTokenRules(css) {
  const out = { tokenToRules: new Map() };
  const emit = (tok, sel, line) => {
    if (!out.tokenToRules.has(tok)) out.tokenToRules.set(tok, []);
    out.tokenToRules.get(tok).push(`${sel} (line ${line})`);
  };
  for (const { sel, line } of ruleSelectors(stripComments(css))) {
    for (const simple of sel.split(",")) {
      const tm = simple.trim().match(/[\.#]([A-Za-z][\w-]*)/);
      if (tm) emit(tm[1], sel, line);
    }
  }
  return out;
}

// ---------- 1. component triples ----------
const dirs = listDirNames(COMPONENTS);
const components = dirs.filter((d) =>
  [d + ".js", d + ".css", d + ".stories.js"].every((f) =>
    fs.existsSync(path.join(COMPONENTS, d, f))
  )
);
{
  const broken = dirs.filter((d) => !components.includes(d));
  if (broken.length) fail(`component dirs with a broken triple: ${broken.join(", ")}`);
  else pass(`components (${components.length})`);
}

// ---------- 2. registry sync ----------
{
  let reg;
  try {
    reg = JSON.parse(read(REGISTRY_JSON));
  } catch {
    fail("registry.json unreadable");
  }
  if (reg) {
    const names = (reg.components || []).map((c) => (typeof c === "string" ? c : c.name));
    if (JSON.stringify([...names].sort()) !== JSON.stringify(components))
      fail(`registry.json out of sync (${names.length} listed, ${components.length} dirs) — run npm run registry`);
    else if (reg.componentCount !== components.length)
      fail(`registry.componentCount ${reg.componentCount} != ${components.length}`);
    else pass(`registry.json in sync (${components.length})`);
    const newest = dirs
      .map((d) => fs.statSync(path.join(COMPONENTS, d, d + ".stories.js")).mtimeMs)
      .reduce((a, b) => Math.max(a, b), 0);
    if (new Date(reg.generatedAt).getTime() < newest)
      warn("registry.json older than newest stories.js — run npm run registry");
  }
}

// ---------- 3. index.html references ----------
{
  const html = read(INDEX_HTML);
  const jsRefs = [];
  const cssRefs = [];
  let m;
  const scriptRgx = /<script[^>]+src="([^"]+)"/g;
  const linkRgx = /<link[^>]+href="([^"]+)"/g;
  while ((m = scriptRgx.exec(html))) if (m[1].startsWith("ui/components/")) jsRefs.push(m[1]);
  while ((m = linkRgx.exec(html))) if (m[1].startsWith("ui/components/")) cssRefs.push(m[1]);

  components.forEach((c) => {
    const js = `ui/components/${c}/${c}.js`;
    const css = `ui/components/${c}/${c}.css`;
    if (jsRefs.filter((r) => r === js).length !== 1) fail(`index.html must include ${js} exactly once`);
    if (cssRefs.filter((r) => r === css).length !== 1) fail(`index.html must include ${css} exactly once`);
  });

  const missing = [];
  {
    const allRefs = [];
    const sr = /<script[^>]+src="([^"]+)"/g;
    while ((m = sr.exec(html))) allRefs.push(m[1]);
    const lr = /<link[^>]+href="([^"]+)"/g;
    while ((m = lr.exec(html))) allRefs.push(m[1]);
    for (const r of allRefs) {
      if (/^https?:/.test(r)) continue;
      if (!fs.existsSync(path.join(WEB, r))) missing.push(r);
    }
  }
  if (missing.length) fail(`referenced files missing: ${missing.join(", ")}`);

  if (jsRefs.length === components.length && cssRefs.length === components.length)
    pass(`index.html refs: ${cssRefs.length} component css + ${jsRefs.length} scripts`);
  else
    fail(`index.html component refs mismatch (${cssRefs.length} css, ${jsRefs.length} js, ${components.length} components)`);

  const inlineStyle = (html.match(/<[a-zA-Z0-9]+[^>]*style=/g) || []).length;
  const allowedInline = (html.match(/style="width:0%"/g) || []).length;
  if (inlineStyle === 1 && allowedInline === 1) pass("inline styles: only #sync-bar width:0%");
  else if (inlineStyle === 0) pass("inline styles: none");
  else fail(`inline styles: expected 1 (#sync-bar), found ${inlineStyle}`);
}

// ---------- 4. style.css braces ----------
{
  const css = read(STYLE_CSS);
  const o = (css.match(/{/g) || []).length;
  const c = (css.match(/}/g) || []).length;
  if (o !== c) fail(`style.css braces ${o} != ${c}`);
  else pass(`style.css braces balanced (${o})`);
}

// ---------- 5. owned selectors absent from style.css ----------
{
  const ownerOf = {};
  const ownedAll = new Set();
  components.forEach((c) => {
    const f = path.join(COMPONENTS, c, c + ".css");
    if (!fs.existsSync(f)) return;
    for (const t of subjectTokens(read(f))) {
      if (!ownerOf[t]) ownerOf[t] = [];
      ownerOf[t].push(c);
      ownedAll.add(t);
    }
  });

  const used = subjectTokenRules(read(STYLE_CSS));
  const offenders = [];
  for (const tok of [...ownedAll].sort()) {
    if (ALLOW.has(tok)) continue;
    for (const ctx of used.tokenToRules.get(tok) || []) {
      offenders.push(`${tok} (owned by ${ownerOf[tok].join("/")}) — ${ctx}`);
    }
  }
  const distinct = [...new Set(offenders.map((o) => o.split(" (owned by")[0]))];
  if (distinct.length) {
    if (DISCOVER)
      warn(`DISCOVER (${offenders.length} rules):\n${offenders.map((o) => "    " + o).join("\n")}`);
    else fail(`owned selectors still rules in style.css: ${distinct.join(", ")}`);
  } else {
    pass("no component-owned selectors in style.css rules");
  }
}

// ---------- 6. JS syntax ----------
{
  const jsFiles = [];
  (function walk(dir) {
    for (const e of fs.readdirSync(dir, { withFileTypes: true })) {
      if (e.isDirectory()) walk(path.join(dir, e.name));
      else if (e.name.endsWith(".js")) jsFiles.push(path.join(dir, e.name));
    }
  })(WEB);
  const bad = [];
  for (const f of jsFiles) {
    const r = cp.spawnSync(process.execPath, ["--check", f], { encoding: "utf8" });
    if (r.status !== 0) bad.push(`${path.relative(ROOT, f)}: ${(r.stderr || "").trim().split("\n")[0]}`);
  }
  if (bad.length) fail(`JS syntax errors:\n${bad.join("\n")}`);
  else pass(`node --check: ${jsFiles.length} files OK`);
}

// ---------- report ----------
const summary = [
  `GUARDRAILS ${errors.length ? "FAIL" : "OK"} (${passes.length} pass, ${errors.length} fail, ${warns.length} warn)`,
  ...passes.map((p) => `  ✓ ${p}`),
  ...errors.map((e) => `  ✗ ${e}`),
  ...warns.map((w) => `  ⚠ ${w}`),
].join("\n");
console.log(summary);
if (!errors.length && warns.length) console.log("\n(Fix warnings before committing.)");
process.exit(errors.length ? 1 : 0);