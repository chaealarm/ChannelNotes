import { defineConfig } from "vite";
import { resolve } from "path";

export default defineConfig({
  build: {
    rollupOptions: {
      external: ["/wails/runtime.js"],
      input: {
        main: resolve(import.meta.dirname, "index.html"),
        detached: resolve(import.meta.dirname, "detached.html"),
      },
    },
  },
});
