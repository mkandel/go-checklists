import { staticAdapter } from "@builder.io/qwik-city/adapters/static/vite";
import { extendConfig } from "@builder.io/qwik-city/vite";
import baseConfig from "../../vite.config";

// SSG build: pre-renders every route to real static HTML at build time (see
// DESIGN.md's "qwik: Qwik SSG SPA" section for why — this is the whole
// reason this app doesn't need a live Node process alongside the Go
// binary). `origin` only matters for canonical/sitemap URLs, which this app
// doesn't use since it's served from an operator-configured domain decided
// at deploy time, not build time.
export default extendConfig(baseConfig, () => {
  return {
    build: {
      ssr: true,
      rollupOptions: {
        input: ["@qwik-city-plan"],
      },
      outDir: "dist",
    },
    plugins: [
      staticAdapter({
        origin: "http://localhost",
      }),
    ],
  };
});
