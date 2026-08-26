import { createRequire } from 'module';
const require = createRequire(import.meta.url);
const sharp = require('/root/.nvm/versions/node/v24.1.0/lib/node_modules/sharp');
import { readFileSync } from 'fs';

// PNG 源图：AI 生成的带渐变细节的原图（1024x1024），往下缩到各尺寸
const sourceImage = '../.scratch/icon-design-4.jpg';

// SVG 源文件：简化版，用于 favicon.svg
const svg = readFileSync('./public/favicon.svg', 'utf-8');

const pngSizes = [
  { name: 'favicon-16x16.png', size: 16 },
  { name: 'favicon-32x32.png', size: 32 },
  { name: 'apple-touch-icon.png', size: 180 },
  { name: 'pwa-192x192.png', size: 192 },
  { name: 'pwa-512x512.png', size: 512 },
  { name: 'icon-1024x1024.png', size: 1024 },
];

async function generateIcons() {
  // 从 AI 原图往下缩生成 PNG（保留渐变细节）
  for (const { name, size } of pngSizes) {
    await sharp(sourceImage)
      .resize(size, size)
      .png()
      .toFile(`./public/${name}`);
    console.log(`✓ Generated ${name} (${size}x${size})`);
  }

  // favicon.svg 由开发者手动维护（简化几何版）
  console.log('✓ favicon.svg (保持手动维护的简化版)');

  // favicon.ico 从 32x32 PNG 生成
  await sharp('./public/favicon-32x32.png')
    .resize(32, 32)
    .png()
    .toFile('./public/favicon.ico');
  console.log('✓ Generated favicon.ico (32x32)');
}

generateIcons().catch(console.error);
