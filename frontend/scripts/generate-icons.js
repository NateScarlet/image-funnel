import { createRequire } from 'module';
const require = createRequire(import.meta.url);
const sharp = require('/root/.nvm/versions/node/v24.1.0/lib/node_modules/sharp');
import { readFileSync, writeFileSync, copyFileSync } from 'fs';
import { fileURLToPath } from 'url';
import { dirname, join } from 'path';

const __filename = fileURLToPath(import.meta.url);
const __dirname = dirname(__filename);

// PNG 源图：AI 生成的带渐变细节的原图
const sourceImage = join(__dirname, '../assets/icon-design-4.jpg');

// SVG 源文件：简化几何版，用于小尺寸 favicon
const svgPath = join(__dirname, '../public/favicon.svg');

// 输出目录
const publicDir = join(__dirname, '../public');

// 大尺寸 PNG：从 AI 原图去背景往下缩
const largePngSizes = [
  { name: 'apple-touch-icon.png', size: 180 },
  { name: 'pwa-192x192.png', size: 192 },
  { name: 'pwa-512x512.png', size: 512 },
];

/**
 * 去除白色背景，裁剪到内容区域，居中到正方形透明画布
 */
async function removeWhiteBackgroundAndCrop(inputBuffer, padding = 0.1) {
  const withAlpha = await sharp(inputBuffer).ensureAlpha().toBuffer();
  const { data, info } = await sharp(withAlpha).raw().toBuffer({ resolveWithObject: true });
  const w = info.width, h = info.height, ch = info.channels;

  // 找到非白色像素的边界框（忽略边缘 JPEG 噪点）
  let minX = w, minY = h, maxX = 0, maxY = 0;
  const threshold = 230;
  const edgeMargin = 20; // 忽略边缘 20px 内的噪点

  // 第一遍：标记透明/不透明
  for (let y = 0; y < h; y++) {
    for (let x = 0; x < w; x++) {
      const i = (y * w + x) * ch;
      const brightness = (data[i] + data[i + 1] + data[i + 2]) / 3;
      data[i + 3] = brightness <= threshold ? 255 : 0;
    }
  }

  // 第二遍：找边界框，忽略边缘区域
  for (let y = edgeMargin; y < h - edgeMargin; y++) {
    for (let x = edgeMargin; x < w - edgeMargin; x++) {
      const i = (y * w + x) * ch;
      if (data[i + 3] === 255) {
        if (x < minX) minX = x;
        if (y < minY) minY = y;
        if (x > maxX) maxX = x;
        if (y > maxY) maxY = y;
      }
    }
  }

  const cropW = maxX - minX + 1;
  const cropH = maxY - minY + 1;

  // 裁剪内容
  const croppedBuf = await sharp(data, {
    raw: { width: w, height: h, channels: ch },
  })
    .extract({ left: minX, top: minY, width: cropW, height: cropH })
    .png()
    .toBuffer();

  // 居中到正方形透明画布，垂直向下偏移底边距的 20% 以补偿三角形视觉重心偏上
  const pad = Math.round(Math.max(cropW, cropH) * padding);
  const squareSize = Math.max(cropW, cropH) + pad * 2;
  const offsetX = Math.round((squareSize - cropW) / 2);
  const offsetY = Math.round((squareSize - cropH) * 0.6);

  return sharp({
    create: {
      width: squareSize,
      height: squareSize,
      channels: 4,
      background: { r: 0, g: 0, b: 0, alpha: 0 },
    },
  })
    .composite([{ input: croppedBuf, left: offsetX, top: offsetY }])
    .png()
    .toBuffer();
}

async function generateIcons() {
  // 大尺寸 PNG：从 AI 原图去背景 + 居中 + 往下缩
  const processed = await removeWhiteBackgroundAndCrop(
    await sharp(sourceImage).resize(2048, 2048).png().toBuffer(),
    0.05
  );

  for (const { name, size } of largePngSizes) {
    await sharp(processed)
      .resize(size, size)
      .png()
      .toFile(join(publicDir, name));
    console.log(`✓ Generated ${name} (${size}x${size})`);
  }

  // favicon.ico 从 SVG 渲染（兼容旧浏览器）
  const svg = readFileSync(svgPath, 'utf-8');
  await sharp(Buffer.from(svg))
    .resize(32, 32)
    .png()
    .toFile(join(publicDir, 'favicon.ico'));
  console.log('✓ Generated favicon.ico (from SVG)');

  console.log('✓ favicon.svg (保持手动维护的简化几何版)');
}

generateIcons().catch(console.error);
