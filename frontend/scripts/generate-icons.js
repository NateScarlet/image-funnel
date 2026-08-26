import { createRequire } from 'module';
const require = createRequire(import.meta.url);
const sharp = require('/root/.nvm/versions/node/v24.1.0/lib/node_modules/sharp');

// PNG 源图：AI 生成的带渐变细节的原图（1024x1024），往下缩到各尺寸
const sourceImage = '../.scratch/icon-design-4.jpg';

const pngSizes = [
  { name: 'favicon-16x16.png', size: 16 },
  { name: 'favicon-32x32.png', size: 32 },
  { name: 'apple-touch-icon.png', size: 180 },
  { name: 'pwa-192x192.png', size: 192 },
  { name: 'pwa-512x512.png', size: 512 },
  { name: 'icon-1024x1024.png', size: 1024 },
];

/**
 * 去除白色背景，裁剪到内容区域，居中到正方形透明画布
 */
async function removeWhiteBackgroundAndCrop(inputBuffer, padding = 0.1) {
  // 使用 sharp 内置 trim 自动裁剪白色背景
  let trimmed = await sharp(inputBuffer).trim(20).toBuffer();
  const meta = await sharp(trimmed).metadata();
  const cropW = meta.width;
  const cropH = meta.height;

  // 居中到正方形透明画布，留出 padding
  const pad = Math.round(Math.max(cropW, cropH) * padding);
  const squareSize = Math.max(cropW, cropH) + pad * 2;
  const offsetX = Math.round((squareSize - cropW) / 2);
  const offsetY = Math.round((squareSize - cropH) / 2);

  return sharp({
    create: {
      width: squareSize,
      height: squareSize,
      channels: 4,
      background: { r: 0, g: 0, b: 0, alpha: 0 },
    },
  })
    .composite([{ input: trimmed, left: offsetX, top: offsetY }])
    .png()
    .toBuffer();
}

async function generateIcons() {
  // 先放大处理背景 + 裁剪 + 居中到正方形，再缩小到目标尺寸
  const processed = await removeWhiteBackgroundAndCrop(
    await sharp(sourceImage).resize(2048, 2048).png().toBuffer(),
    0.05 // 5% 留白，让图标在小尺寸下更大
  );

  // 从处理后的透明图往下缩生成 PNG
  for (const { name, size } of pngSizes) {
    await processed
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
