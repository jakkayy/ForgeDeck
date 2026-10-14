// Fails the build when the dashboard grows past its gzip budget.
// Run after `vite build`: `bun scripts/check-size.ts`.
import { readdirSync, readFileSync } from "node:fs";
import { join } from "node:path";

const KB = 1024;
const budget = {
  initialJs: 130 * KB, // everything loaded before the first route renders
  // Grows about 1–2 KB per HeroUI component in use; see docs/08 before raising.
  css: 22 * KB,
  routeChunk: 60 * KB, // any lazily loaded chunk
};

const dist = "dist";
const gzipSize = (file: string) => Bun.gzipSync(readFileSync(join(dist, file))).length;
const kb = (bytes: number) => `${(bytes / KB).toFixed(1)} KB`;

const html = readFileSync(join(dist, "index.html"), "utf8");
const initial = [...html.matchAll(/(?:src|href)="\/(assets\/[^"]+\.(?:js|css))"/g)].map(
  (m) => m[1] ?? "",
);
const initialJs = initial.filter((f) => f.endsWith(".js"));
const assets = readdirSync(join(dist, "assets")).map((f) => `assets/${f}`);

const failures: string[] = [];
const check = (label: string, size: number, limit: number) => {
  const ok = size <= limit;
  console.log(`${ok ? "ok  " : "FAIL"} ${label.padEnd(44)} ${kb(size).padStart(9)} / ${kb(limit)}`);
  if (!ok) failures.push(label);
};

check(
  "initial JS",
  initialJs.reduce((sum, f) => sum + gzipSize(f), 0),
  budget.initialJs,
);
check(
  "CSS",
  assets.filter((f) => f.endsWith(".css")).reduce((sum, f) => sum + gzipSize(f), 0),
  budget.css,
);
for (const chunk of assets.filter((f) => f.endsWith(".js") && !initialJs.includes(f))) {
  check(chunk, gzipSize(chunk), budget.routeChunk);
}

if (failures.length > 0) {
  console.error(
    `\nBundle budget exceeded: ${failures.join(", ")}. See docs/08-frontend-ux.md#13-performance.`,
  );
  process.exit(1);
}
