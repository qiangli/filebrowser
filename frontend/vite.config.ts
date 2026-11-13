import fs from "node:fs";
import path from "node:path";
import { defineConfig, type Plugin } from "vite";
import vue from "@vitejs/plugin-vue";
import VueI18nPlugin from "@intlify/unplugin-vue-i18n/vite";
import { compression } from "vite-plugin-compression2";

// aceModules ships the Ace editor's runtime-loaded modules (language modes,
// themes, syntax workers and snippets) with the build so the editor loads
// them from this origin (see Editor.vue's basePath) instead of a public CDN:
// no phone-home, and the editor works offline. Registered before the
// compression plugin so the emitted files are pre-gzipped like every other
// script.
function aceModules(): Plugin {
  const srcDir = path.resolve(
    __dirname,
    "node_modules/ace-builds/src-min-noconflict",
  );
  return {
    name: "ace-modules",
    apply: "build",
    generateBundle() {
      const emit = (file: string) =>
        this.emitFile({
          type: "asset",
          fileName: `ace/${file}`,
          source: fs.readFileSync(path.join(srcDir, file)),
        });
      for (const file of fs.readdirSync(srcDir)) {
        if (/^(mode|theme|worker)-[\w-]+\.js$/.test(file)) emit(file);
      }
      for (const file of fs.readdirSync(path.join(srcDir, "snippets"))) {
        if (/\.js$/.test(file)) emit(`snippets/${file}`);
      }
    },
  };
}

// No @vitejs/plugin-legacy: this app ships embedded in outpost (and as a
// self-hosted standalone) for modern browsers only. The legacy nomodule
// bundles roughly doubled the JS payload (~1 MB gz) for IE/pre-2018
// support nobody here needs — dropping them ~halves the embedded
// fbembed/dist.zip. Modern ESM bundles are unchanged.
const plugins = [
  vue(),
  aceModules(),
  VueI18nPlugin({
    include: [path.resolve(__dirname, "./src/i18n/**/*.json")],
  }),
  compression({ include: /\.js$/, deleteOriginalAssets: false }),
];

const resolve = {
  alias: {
    // vue: "@vue/compat",
    "@/": `${path.resolve(__dirname, "src")}/`,
  },
};

// https://vitejs.dev/config/
export default defineConfig(({ command }) => {
  if (command === "serve") {
    return {
      plugins,
      resolve,
      server: {
        proxy: {
          "/api/command": {
            target: "ws://127.0.0.1:8080",
            ws: true,
          },
          "/api": "http://127.0.0.1:8080",
        },
      },
    };
  } else {
    // command === 'build'
    return {
      plugins,
      resolve,
      base: "",
      build: {
        rollupOptions: {
          input: {
            index: path.resolve(__dirname, "./public/index.html"),
          },
          output: {
            manualChunks: (id) => {
              // bundle dayjs files in a single chunk
              // this avoids having small files for each locale
              if (id.includes("dayjs/")) {
                return "dayjs";
                // bundle i18n in a separate chunk
              } else if (id.includes("i18n/")) {
                return "i18n";
              }
            },
          },
        },
      },
      experimental: {
        renderBuiltUrl(filename, { hostType }) {
          if (hostType === "js") {
            return { runtime: `window.__prependStaticUrl("${filename}")` };
          } else if (hostType === "html") {
            return `[{[ .StaticURL ]}]/${filename}`;
          } else {
            return { relative: true };
          }
        },
      },
    };
  }
});
