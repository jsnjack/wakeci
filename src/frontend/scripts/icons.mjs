// Writes src/assets/fonts/material-symbols.woff2: Material Symbols Outlined
// cut to the icons the app uses, a few kilobytes instead of the whole font.
// Run it after adding or removing an icon:
//
//     npm run icons
//
// Google Fonts cuts the font (icon_names). It keeps the FILL axis, which the
// status icons use, and leaves weight, grade and optical size at 400, 0 and
// 24. e2e/icons.spec.js fails while the font lacks an icon of the source.
//
// The names come from the source, written in one of these ways:
// - the text of <i class="icon">name</i>, or what a {{ }} there results in
// - icon: "name", icon="name", or default: "name" of an icon prop
// - the values of an object named ICONS or ..._ICONS
// A name built at run time in another way is missed, and shows as text.
import { mkdirSync, readFileSync, readdirSync, writeFileSync } from "node:fs";
import { dirname, extname, join } from "node:path";
import { fileURLToPath } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const SRC = join(here, "..", "src");
export const FONT = join(SRC, "assets", "fonts", "material-symbols.woff2");

const NAME = /^[a-z0-9_]+$/;
// A name that a {{ }} gives: the whole expression, or a result of ?:, || or ??
const RESULT = /(?:^|\?\??|:|\|\|)\s*["']([a-z0-9_]+)["']/g;
const TAG = /<i\b[^>]*\bclass="[^"]*\bicon\b[^"]*"[^>]*>([\s\S]*?)<\/i\s*>/g;
const MUSTACHE = /\{\{([\s\S]*?)\}\}/g;
const KEY = /(?<![\w.:-])icon\s*[:=]\s*["']([a-z0-9_]+)["']/g;
const PROP_DEFAULT =
  /(?<![\w.:-])icon\s*:\s*\{[^}]*?\bdefault\s*:\s*["']([a-z0-9_]+)["']/g;
const MAP = /\b[A-Z_]*ICONS\s*=\s*\{([^}]*)\}/g;
const MAP_VALUE = /:\s*["']([a-z0-9_]+)["']/g;

function* sourceFiles(dir) {
  for (const entry of readdirSync(dir, { withFileTypes: true })) {
    const path = join(dir, entry.name);
    if (entry.isDirectory()) {
      yield* sourceFiles(path);
    } else if ([".vue", ".js"].includes(extname(entry.name))) {
      yield path;
    }
  }
}

// iconNames returns the icon names in the source, sorted
export function iconNames(dir = SRC) {
  const names = new Set();
  const add = (text, re) => {
    for (const m of text.matchAll(re)) names.add(m[1]);
  };
  for (const file of sourceFiles(dir)) {
    const text = readFileSync(file, "utf8");
    for (const [, content] of text.matchAll(TAG)) {
      const name = content.trim();
      if (NAME.test(name)) {
        names.add(name);
      }
      for (const [, expression] of content.matchAll(MUSTACHE)) {
        add(expression.trim(), RESULT);
      }
    }
    add(text, KEY);
    add(text, PROP_DEFAULT);
    for (const [, body] of text.matchAll(MAP)) {
      add(body, MAP_VALUE);
    }
  }
  return [...names].sort();
}

// Google Fonts sends WOFF2 only to browsers that read it
const BROWSER =
  "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36";

async function get(url) {
  const response = await fetch(url, { headers: { "User-Agent": BROWSER } });
  if (!response.ok) {
    throw new Error(`${url}: ${response.status} ${await response.text()}`);
  }
  return response;
}

async function writeFont() {
  const names = iconNames();
  // Google Fonts wants the names sorted
  const css = await (
    await get(
      `https://fonts.googleapis.com/css2?family=Material+Symbols+Outlined:FILL@0..1&icon_names=${names.join(",")}`,
    )
  ).text();
  const url = css.match(/url\((https:[^)]+)\)\s*format\(['"]woff2['"]\)/)?.[1];
  if (!url) {
    throw new Error(`Google Fonts sent no WOFF2 font:\n${css}`);
  }
  const font = Buffer.from(await (await get(url)).arrayBuffer());
  mkdirSync(dirname(FONT), { recursive: true });
  writeFileSync(FONT, font);
  console.log(`Wrote ${FONT}: ${names.length} icons, ${font.length} bytes`);
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  await writeFont();
}
