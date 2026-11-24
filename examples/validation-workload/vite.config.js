import { defineConfig } from "vite";

// build.manifest writes dist/.vite/manifest.json, a hidden file. Real projects
// have such files, and artifact upload skips hidden files unless asked to keep
// them, so this exercises that path.
export default defineConfig({
  build: { manifest: true, outDir: "dist", emptyOutDir: true },
  test: { environment: "node" },
});
