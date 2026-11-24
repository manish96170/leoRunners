// A repository-specific lint, standing in for the custom checks real projects
// run next to their linter: every source file needs a test beside it.
import { readdirSync } from "node:fs";

const files = readdirSync("src");
const missing = files
  .filter(
    (f) => f.endsWith(".js") && !f.endsWith(".test.js") && f !== "main.js",
  )
  .filter((f) => !files.includes(f.replace(/\.js$/, ".test.js")));

if (missing.length > 0) {
  console.error(`missing tests for: ${missing.join(", ")}`);
  process.exit(1);
}
console.log("custom lint passed");
