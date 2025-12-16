import js from '@eslint/js'
import configPrettier from 'eslint-config-prettier'
import globals from 'globals'
import pluginVue from 'eslint-plugin-vue'
import vueParser from 'vue-eslint-parser'
import tsParser from '@typescript-eslint/parser'

export default [
  // 基础 JavaScript 规则
  js.configs.recommended,

  // 全局变量配置
  {
    languageOptions: {
      ecmaVersion: 'latest',
      sourceType: 'module',
      globals: {
        ...globals.browser,
        ...globals.node,
        ...globals.es2021
      }
    }
  },

  // Vue 文件配置
  {
    files: ['**/*.vue'],
    languageOptions: {
      parser: vueParser,
      parserOptions: {
        parser: tsParser,
        ecmaVersion: 'latest',
        sourceType: 'module'
      }
    },
    plugins: {
      vue: pluginVue
    },
    rules: {
      ...pluginVue.configs['flat/essential'].rules,
      'no-unused-vars': 'off',
      'no-undef': 'off',
      'no-control-regex': 'off'
    }
  },

  // TypeScript 文件配置
  {
    files: ['**/*.ts', '**/*.tsx'],
    languageOptions: {
      parser: tsParser,
      parserOptions: {
        ecmaVersion: 'latest',
        sourceType: 'module'
      }
    },
    rules: {
      'no-unused-vars': 'off',
      'no-undef': 'off',
      'no-control-regex': 'off'
    }
  },

  // Prettier 配置（禁用与 Prettier 冲突的规则）
  configPrettier,

  // 全局忽略
  {
    ignores: ['node_modules/**', 'dist/**', '*.d.ts']
  }
]
