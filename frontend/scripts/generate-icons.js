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

/**
 * 去除白色背景：将接近白色的像素设为透明
 * 先在大图上处理，再缩小，避免抗锯齿边缘丢失
 */
async function removeWhiteBackground(inputBuffer) {
  // 确保有 alpha 通道
  const withAlpha = await sharp(inputBuffer).ensureAlpha().toBuffer();

  // 使用 raw 像素数据进行颜色替换
  const { data, info } = await sharp(withAlpha)
    .raw()
    .toBuffer({ resolveWithObject: true });

  // 遍历像素，将接近白色的设为透明
  const threshold = 230;
  for (let i = 0; i < data.length; i += info.channels) {
    const r = data[i];
    const g = data[i + 1];
    const b = data[i + 2];
    const brightness = (r + g + b) / 3;
    if (brightness > threshold) {
      data[i + 3] = 0;
    }
  }

  return sharp(data, {
    raw: {
      width: info.width,
      height: info.height,
      channels: info.channels,
    },
  });
}

async function generateIcons() {
  // 先在大图上处理背景，再缩小到目标尺寸
  const largeProcessed = await removeWhiteBackground(
    await sharp(sourceImage).resize(2048, 2048).png().toBuffer()
  );

  // 从处理后的透明大图往下缩生成 PNG
  for (const { name, size } of pngSizes) {
    await largeProcessed
      .clone()
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
