import js from "@eslint/js";
import vue from "eslint-plugin-vue";
import tseslint from "typescript-eslint";
import prettier from "eslint-config-prettier";
import prettierPlugin from "eslint-plugin-prettier/recommended";
import graphql from "@graphql-eslint/eslint-plugin";
import { defineConfig } from "eslint/config";
import vueParser from "vue-eslint-parser";

export default defineConfig(
  {
    ignores: [
      "dist/**",
      "node_modules/**",
      "*.config.cjs",
      "*.config.mts",
      "codegen.ts",
      "**/generated.ts",
      "scripts/**",
    ],
  },
  {
    files: ["**/*.{js,mjs,cjs,ts,tsx}"],
    extends: [
      js.configs.recommended,
      ...tseslint.configs.strict,
      ...tseslint.configs.stylistic,
    ],
    languageOptions: {
      ecmaVersion: "latest",
      sourceType: "module",
      globals: {
        document: "readonly",
        window: "readonly",
        console: "readonly",
        setTimeout: "readonly",
        setInterval: "readonly",
        clearTimeout: "readonly",
        clearInterval: "readonly",
        localStorage: "readonly",
        confirm: "readonly",
      },
      parser: tseslint.parser,
      parserOptions: {
        project: "./src/tsconfig.json",
      },
    },
    rules: {},
  },
  {
    files: ["**/*.vue"],
    extends: [
      js.configs.recommended,
      ...tseslint.configs.strict,
      ...tseslint.configs.stylistic,
      ...vue.configs["flat/recommended"],
    ],
    languageOptions: {
      ecmaVersion: "latest",
      sourceType: "module",
      globals: {
        document: "readonly",
        window: "readonly",
        console: "readonly",
        setTimeout: "readonly",
        setInterval: "readonly",
        clearTimeout: "readonly",
        clearInterval: "readonly",
        localStorage: "readonly",
        confirm: "readonly",
      },
      parser: vueParser,
      parserOptions: {
        parser: tseslint.parser,
        project: "./src/tsconfig.json",
        extraFileExtensions: [".vue"],
      },
    },
    rules: {},
  },
  {
    files: ["**/*.gql", "**/*.graphql"],
    extends: [graphql.configs["flat/operations-recommended"]],
    plugins: {
      // @ts-ignore-next-line 类型过时，实际能用
      "@graphql-eslint": graphql,
    },
    languageOptions: {
      parser: graphql.parser,
    },
  },
  prettier,
  prettierPlugin,
);
