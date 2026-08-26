import { createRequire } from 'module';
const require = createRequire(import.meta.url);
const sharp = require('/root/.nvm/versions/node/v24.1.0/lib/node_modules/sharp');
import { readFileSync } from 'fs';

const svg = readFileSync('./public/favicon.svg', 'utf-8');

const sizes = [
  { name: 'favicon-16x16.png', size: 16 },
  { name: 'favicon-32x32.png', size: 32 },
  { name: 'apple-touch-icon.png', size: 180 },
  { name: 'pwa-192x192.png', size: 192 },
  { name: 'pwa-512x512.png', size: 512 },
  { name: 'icon-1024x1024.png', size: 1024 },
];

async function generateIcons() {
  for (const { name, size } of sizes) {
    await sharp(Buffer.from(svg))
      .resize(size, size)
      .png()
      .toFile(`./public/${name}`);
    console.log(`✓ Generated ${name} (${size}x${size})`);
  }

  // 生成 favicon.ico（直接使用 32x32 PNG，现代浏览器支持 PNG 格式的 ico）
  await sharp(Buffer.from(svg))
    .resize(32, 32)
    .png()
    .toFile('./public/favicon.ico');

  console.log('✓ Generated favicon.ico (32x32)');
}

generateIcons().catch(console.error);
