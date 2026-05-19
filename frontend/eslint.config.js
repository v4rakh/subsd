import jsRecommendedLib from '@eslint/js';
import typescriptPlugin from '@typescript-eslint/eslint-plugin';
import typescriptParser from '@typescript-eslint/parser';
import importPlugin from 'eslint-plugin-import';
import jsxA11yPlugin from 'eslint-plugin-jsx-a11y';
import prettierPlugin from 'eslint-plugin-prettier';
import reactPlugin from 'eslint-plugin-react';
import reactHooksPlugin from 'eslint-plugin-react-hooks';
import sonarjsPlugin from 'eslint-plugin-sonarjs';
import testingLibPlugin from 'eslint-plugin-testing-library';
import { fixupPluginRules } from '@eslint/compat';

// eslint.config.js
export default [
	jsRecommendedLib.configs.recommended,
	{
		...reactHooksPlugin.configs.flat.recommended,
		...importPlugin.flatConfigs.recommended,
		...jsxA11yPlugin.flatConfigs.recommended,
		files: ['**/styles/**', '**/__tests__/**', '**/*.test.tsx', '**/*.test.ts', '*.css', 'src/**/*.tsx'],
		languageOptions: {
			parserOptions: {
				ecmaFeatures: {
					jsx: true
				},
				project: './tsconfig.json'
			},
			parser: typescriptParser
		},
		plugins: {
			react: reactPlugin,
			'react-hooks': fixupPluginRules(reactHooksPlugin),
			sonarjs: sonarjsPlugin,
			import: fixupPluginRules(importPlugin),
			'jsx-a11y': jsxA11yPlugin,
			'@typescript-eslint': typescriptPlugin,
			prettier: prettierPlugin,
			js: jsRecommendedLib,
			'testing-library': fixupPluginRules(testingLibPlugin)
		},
		settings: {
			react: {
				version: 'detect'
			},
			'import/resolver': {
				typescript: {},
				node: {
					paths: ['src'],
					extensions: ['.js', '.jsx', '.ts', '.tsx']
				}
			}
		},
		rules: {
			// Imports which don't support flat config, directly inject recommended rules
			...reactPlugin.configs.recommended.rules,
			...reactPlugin.configs['jsx-runtime'].rules,

			// Disable ESLint 10 incompatible React rules which should be fine to disable (mostly class without TS)
			'react/display-name': 'off',
			'react/no-direct-mutation-state': 'off',
			'react/no-render-return-value': 'off',
			'react/no-string-refs': 'off',
			'react/no-unknown-property': 'off',
			'react/prop-types': 'off',
			'react/require-render-return': 'off',

			// Explicit React rules
			'react/jsx-uses-react': 'off',
			'react/react-in-jsx-scope': 'off',

			// Explicit React Hooks rules
			'react-hooks/exhaustive-deps': 'warn',
			'react-hooks/rules-of-hooks': 'error',
			'react-hooks/set-state-in-effect': 'off',

			// Explicit TypeScript rules
			'@typescript-eslint/consistent-type-assertions': 'error',
			'@typescript-eslint/consistent-type-definitions': ['error', 'interface'],
			'@typescript-eslint/explicit-function-return-type': 'off',
			'@typescript-eslint/explicit-member-accessibility': 'warn',
			'@typescript-eslint/no-empty-function': 'warn',
			'@typescript-eslint/no-empty-interface': 'warn',
			'@typescript-eslint/no-explicit-any': 'off',
			'@typescript-eslint/no-inferrable-types': 'warn',
			'@typescript-eslint/no-misused-promises': 'off',
			'@typescript-eslint/no-non-null-assertion': 'warn',
			'@typescript-eslint/no-unnecessary-type-assertion': 'warn',
			'@typescript-eslint/no-unsafe-argument': 'off',
			'@typescript-eslint/no-unsafe-assignment': 'off',
			'@typescript-eslint/no-unsafe-call': 'warn',
			'@typescript-eslint/no-unsafe-member-access': 'off',
			'@typescript-eslint/no-unsafe-return': 'warn',
			'@typescript-eslint/no-unused-expressions': 'warn',
			'@typescript-eslint/no-unused-vars': 'error',
			'@typescript-eslint/strict-boolean-expressions': 'off',

			// Explicit other rules
			'no-console': 'warn',
			'no-duplicate-imports': 'warn',
			'no-undef': 'off',
			'no-unused-vars': 'off',
			'prefer-const': 'error',
			'testing-library/no-debugging-utils': 'warn',
			'testing-library/no-dom-import': 'off',
			semi: 'error',

			// Explicit SonarJS rules
			'sonarjs/cognitive-complexity': 'off',
			'sonarjs/elseif-without-else': 'off',
			'sonarjs/max-switch-cases': 'error',
			'sonarjs/no-all-duplicated-branches': 'error',
			'sonarjs/no-collapsible-if': 'error',
			'sonarjs/no-collection-size-mischeck': 'error',
			'sonarjs/no-duplicate-string': 'off',
			'sonarjs/no-duplicated-branches': 'error',
			'sonarjs/no-element-overwrite': 'error',
			'sonarjs/no-empty-collection': 'error',
			'sonarjs/no-extra-arguments': 'error',
			'sonarjs/no-gratuitous-expressions': 'error',
			'sonarjs/no-identical-conditions': 'error',
			'sonarjs/no-identical-expressions': 'error',
			'sonarjs/no-identical-functions': 'error',
			'sonarjs/no-ignored-return': 'error',
			'sonarjs/no-inverted-boolean-check': 'error',
			'sonarjs/no-nested-switch': 'error',
			'sonarjs/no-nested-template-literals': 'error',
			'sonarjs/no-redundant-boolean': 'error',
			'sonarjs/no-redundant-jump': 'error',
			'sonarjs/no-same-line-conditional': 'error',
			'sonarjs/no-small-switch': 'error',
			'sonarjs/no-unused-collection': 'error',
			'sonarjs/no-use-of-empty-return-value': 'error',
			'sonarjs/no-useless-catch': 'error',
			'sonarjs/non-existent-operator': 'error',
			'sonarjs/prefer-immediate-return': 'error',
			'sonarjs/prefer-object-literal': 'error',
			'sonarjs/prefer-single-boolean-return': 'error',
			'sonarjs/prefer-while': 'error',

			// Explicit import rules
			'import/default': 'error',
			'import/export': 'error',
			'import/named': 'error',
			'import/namespace': 'error',
			'import/no-duplicates': 'error',
			'import/no-named-as-default-member': 'warn',
			'import/no-named-as-default': 'warn',
			'import/no-unresolved': 'error',
			'import/order': [
				'warn',
				{
					alphabetize: {
						caseInsensitive: true,
						order: 'asc'
					},
					groups: [['builtin', 'external', 'index', 'sibling', 'parent', 'internal']],
					'newlines-between': 'always',
					pathGroups: [
						{
							pattern: '*.css',
							group: 'index',
							patternOptions: {
								matchBase: true
							},
							position: 'before'
						},
						{
							pattern: '*.json',
							group: 'index',
							patternOptions: {
								matchBase: true
							},
							position: 'after'
						}
					]
				}
			]
		}
	}
];
