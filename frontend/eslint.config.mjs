import { defineConfig, globalIgnores } from "eslint/config";
import nextVitals from "eslint-config-next/core-web-vitals";
import nextTs from "eslint-config-next/typescript";
import prettier from "eslint-config-prettier/flat";

export default defineConfig([
  ...nextVitals,
  ...nextTs,
  prettier,
  {
    files: [
      "src/app/App.tsx",
      "src/features/events/EventAnalysis.tsx",
      "src/features/knowledge/KnowledgeSearch.tsx",
    ],
    rules: {
      // Existing data-loading effects update loading state before awaiting I/O.
      "react-hooks/set-state-in-effect": "off",
    },
  },
  globalIgnores([
    ".next/**",
    "out/**",
    "build/**",
    "dist/**",
    "coverage/**",
    "test-results/**",
    "playwright-report/**",
    "next-env.d.ts",
    "src/api/**",
    "vite.config.ts",
  ]),
]);
