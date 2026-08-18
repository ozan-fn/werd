import { defineConfig } from '@rsbuild/core';
import { pluginPreact } from '@rsbuild/plugin-preact';
import { pluginTailwindcss } from '@rsbuild/plugin-tailwindcss';
import { pluginCompression } from 'rsbuild-plugin-compression';

// Docs: https://rsbuild.rs/config/
export default defineConfig({
  plugins: [
    pluginPreact(),
    pluginTailwindcss(),
    pluginCompression({
      algorithms: [{ name: 'brotli', options: { quality: 11 } }],
    }),
  ],
});
