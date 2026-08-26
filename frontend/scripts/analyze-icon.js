import { createRequire } from 'module';
const require = createRequire(import.meta.url);
const sharp = require('/root/.nvm/versions/node/v24.1.0/lib/node_modules/sharp');
import { writeFileSync } from 'fs';
import { fileURLToPath } from 'url';
import { dirname, join } from 'path';

const __filename = fileURLToPath(import.meta.url);
const __dirname = dirname(__filename);

const sourceImage = join(__dirname, '../assets/icon-design-4.jpg');
const publicDir = join(__dirname, '../public');

// 分析尺寸
const ANALYZE_SIZE = 240;
// 高斯模糊半径：抑制JPEG噪点
const BLUR_RADIUS = 3;
// 背景阈值
const BG_THRESHOLD = 230;

// #region 工具函数
function clamp(v, min, max) { return Math.max(min, Math.min(max, v)); }

/** 线性回归：从点集拟合直线 y = slope * x + intercept */
function linearRegression(points) {
  const n = points.length;
  if (n < 2) return null;
  let sumX = 0, sumY = 0, sumXY = 0, sumX2 = 0;
  for (const [x, y] of points) {
    sumX += x; sumY += y;
    sumXY += x * y; sumX2 += x * x;
  }
  const slope = (n * sumXY - sumX * sumY) / (n * sumX2 - sumX * sumX);
  const intercept = (sumY - slope * sumX) / n;
  return { slope, intercept };
}

/** 计算直线与线段 AB 的交点 */
function lineSegmentIntersect(ax, ay, bx, by, slope, intercept) {
  const denom = (by - ay) - slope * (bx - ax);
  if (Math.abs(denom) < 1e-10) return null;
  const t = (slope * ax + intercept - ay) / denom;
  if (t < 0 || t > 1) return null;
  return { x: ax + t * (bx - ax), y: ay + t * (by - ay), t };
}
// #endregion

// #region 图像预处理：去背景、裁剪、居中（与 generate-icons.js 一致）
async function preprocessImage(inputBuffer, padding = 0.05) {
  const withAlpha = await sharp(inputBuffer).ensureAlpha().toBuffer();
  const { data, info } = await sharp(withAlpha).raw().toBuffer({ resolveWithObject: true });
  const w = info.width, h = info.height, ch = info.channels;

  // 标记透明/不透明
  const edgeMargin = 20;
  for (let y = 0; y < h; y++) {
    for (let x = 0; x < w; x++) {
      const i = (y * w + x) * ch;
      const brightness = (data[i] + data[i + 1] + data[i + 2]) / 3;
      data[i + 3] = brightness <= BG_THRESHOLD ? 255 : 0;
    }
  }

  // 找边界框
  let minX = w, minY = h, maxX = 0, maxY = 0;
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

  const croppedBuf = await sharp(data, {
    raw: { width: w, height: h, channels: ch },
  })
    .extract({ left: minX, top: minY, width: cropW, height: cropH })
    .png()
    .toBuffer();

  // 居中到正方形
  const pad = Math.round(Math.max(cropW, cropH) * padding);
  const squareSize = Math.max(cropW, cropH) + pad * 2;
  const offsetX = Math.round((squareSize - cropW) / 2);
  const offsetY = Math.round((squareSize - cropH) * 0.6); // 视觉补偿

  const centered = await sharp({
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

  return { buffer: centered, size: squareSize };
}
// #endregion

// #region 图像分析
// 模拟预处理过程：计算裁剪边界和居中后的坐标映射
async function computePreprocessMapping() {
  // 用原始图像计算裁剪边界
  const rawBuf = await sharp(sourceImage).resize(1024, 1024).png().toBuffer();
  const withAlpha = await sharp(rawBuf).ensureAlpha().toBuffer();
  const { data, info } = await sharp(withAlpha).raw().toBuffer({ resolveWithObject: true });
  const w = info.width, h = info.height, ch = info.channels;

  let minX = w, minY = h, maxX = 0, maxY = 0;
  const edgeMargin = 20;
  for (let y = edgeMargin; y < h - edgeMargin; y++) {
    for (let x = edgeMargin; x < w - edgeMargin; x++) {
      const i = (y * w + x) * ch;
      const brightness = (data[i] + data[i + 1] + data[i + 2]) / 3;
      if (brightness <= BG_THRESHOLD) {
        if (x < minX) minX = x;
        if (y < minY) minY = y;
        if (x > maxX) maxX = x;
        if (y > maxY) maxY = y;
      }
    }
  }

  const cropW = maxX - minX + 1;
  const cropH = maxY - minY + 1;
  const padding = 0.05;
  const pad = Math.round(Math.max(cropW, cropH) * padding);
  const squareSize = Math.max(cropW, cropH) + pad * 2;
  const offsetX = Math.round((squareSize - cropW) / 2);
  const offsetY = Math.round((squareSize - cropH) * 0.6);

  return {
    cropX: minX, cropY: minY, cropW, cropH,
    squareSize, offsetX, offsetY, pad,
  };
}

/** 将原始图像坐标映射到预处理后的居中图像坐标 */
function mapToCentered(px, py, mapping) {
  const { cropX, cropY, cropW, cropH, squareSize, offsetX, offsetY } = mapping;
  // 原始坐标相对于裁剪区域
  const relX = px - cropX;
  const relY = py - cropY;
  // 居中后的坐标
  const cx = offsetX + relX;
  const cy = offsetY + relY;
  // 归一化到 [0, 1]
  return { x: cx / squareSize, y: cy / squareSize };
}

async function analyzeImage() {
  // 1. 预处理映射：计算裁剪和居中参数
  const mapping = await computePreprocessMapping();
  console.log(`预处理映射: crop=(${mapping.cropX},${mapping.cropY}) ${mapping.cropW}x${mapping.cropH}, ` +
    `square=${mapping.squareSize}, offset=(${mapping.offsetX},${mapping.offsetY})`);

  // 2. 直接在原始图像上分析（缩放到分析尺寸并降噪）
  const { data, info } = await sharp(sourceImage)
    .resize(ANALYZE_SIZE, ANALYZE_SIZE, { fit: 'fill' })
    .blur(BLUR_RADIUS)
    .grayscale()
    .raw()
    .toBuffer({ resolveWithObject: true });

  const w = info.width, h = info.height;
  const pixels = new Uint8Array(data);

  // 彩色版本
  const { data: colorData } = await sharp(sourceImage)
    .resize(ANALYZE_SIZE, ANALYZE_SIZE, { fit: 'fill' })
    .blur(BLUR_RADIUS)
    .raw()
    .toBuffer({ resolveWithObject: true });
  const colorPixels = new Uint8Array(colorData);

  console.log(`分析图像: ${w}x${h}`);

  // 3. 找到三角形边界
  let minX = w, minY = h, maxX = 0, maxY = 0;
  const edgeMargin = 2;
  for (let y = edgeMargin; y < h - edgeMargin; y++) {
    for (let x = edgeMargin; x < w - edgeMargin; x++) {
      if (pixels[y * w + x] < BG_THRESHOLD) {
        if (x < minX) minX = x;
        if (y < minY) minY = y;
        if (x > maxX) maxX = x;
        if (y > maxY) maxY = y;
      }
    }
  }
  console.log(`三角形边界框: (${minX},${minY}) - (${maxX},${maxY})`);

  // 4. 找到三角形三个角点
  // 上边缘扫描
  let topLeftX = w, topRightX = 0;
  const topScanY = Math.min(minY + 4, h - 1);
  for (let x = minX; x <= maxX; x++) {
    if (pixels[topScanY * w + x] < BG_THRESHOLD) {
      if (x < topLeftX) topLeftX = x;
      if (x > topRightX) topRightX = x;
    }
  }

  // 左右边缘拟合
  const leftEdgePoints = [];
  const rightEdgePoints = [];
  for (let y = minY + 2; y <= maxY - 2; y++) {
    for (let x = minX; x <= maxX; x++) {
      if (pixels[y * w + x] < BG_THRESHOLD) {
        leftEdgePoints.push([x, y]);
        break;
      }
    }
    for (let x = maxX; x >= minX; x--) {
      if (pixels[y * w + x] < BG_THRESHOLD) {
        rightEdgePoints.push([x, y]);
        break;
      }
    }
  }

  const leftLine = linearRegression(leftEdgePoints);
  const rightLine = linearRegression(rightEdgePoints);

  // 角点 A, B, C
  const A_y = minY;
  const A_x = leftLine ? Math.round((A_y - leftLine.intercept) / leftLine.slope) : topLeftX;
  const B_y = minY;
  const B_x = rightLine ? Math.round((B_y - rightLine.intercept) / rightLine.slope) : topRightX;

  let C_x, C_y;
  if (leftLine && rightLine) {
    C_x = Math.round((rightLine.intercept - leftLine.intercept) / (leftLine.slope - rightLine.slope));
    C_y = Math.round(leftLine.slope * C_x + leftLine.intercept);
  } else {
    C_x = Math.round((minX + maxX) / 2);
    C_y = maxY;
  }

  console.log(`三角形角点: A(${A_x},${A_y}) B(${B_x},${B_y}) C(${C_x},${C_y})`);

  // 5. 检测折痕线
  const foldCandidates = [];
  for (let y = Math.max(A_y + 5, minY + 5); y < Math.min(C_y, maxY - 2); y += 2) {
    // 当前行三角形内左右边界
    let rowLeft = w, rowRight = 0;
    for (let x = Math.max(A_x, minX); x <= Math.min(B_x, maxX); x++) {
      if (pixels[y * w + x] < BG_THRESHOLD) {
        if (x < rowLeft) rowLeft = x;
        if (x > rowRight) rowRight = x;
      }
    }
    if (rowLeft >= rowRight) continue;

    // 亮度二阶差分找折痕
    let maxDiff = 0, maxDiffX = rowLeft;
    for (let x = rowLeft + 4; x < rowRight - 4; x++) {
      const leftAvg = (pixels[y * w + (x - 4)] + pixels[y * w + (x - 3)] + pixels[y * w + (x - 2)]) / 3;
      const rightAvg = (pixels[y * w + (x + 2)] + pixels[y * w + (x + 3)] + pixels[y * w + (x + 4)]) / 3;
      const diff = Math.abs(rightAvg - leftAvg);
      if (diff > maxDiff) {
        maxDiff = diff;
        maxDiffX = x;
      }
    }

    if (maxDiff > 12) {
      foldCandidates.push([maxDiffX, y, maxDiff]);
    }
  }

  console.log(`折痕候选点: ${foldCandidates.length} 个`);

  // 6. 拟合折痕线
  foldCandidates.sort((a, b) => b[2] - a[2]);
  const topCandidates = foldCandidates.slice(0, Math.min(40, foldCandidates.length));
  // 打印前 5 个候选点用于调试
  topCandidates.slice(0, 5).forEach(([x, y, d], i) => {
    console.log(`  候选 #${i + 1}: (${x},${y}) diff=${d.toFixed(1)}`);
  });

  const foldLine = linearRegression(topCandidates.map(p => [p[0], p[1]]));
  console.log(`折痕线: y = ${foldLine.slope.toFixed(4)}x + ${foldLine.intercept.toFixed(4)}`);

  // 7. 计算关键点 E, D, F
  // E = 折痕线与 AB 的交点
  const E_intersect = lineSegmentIntersect(A_x, A_y, B_x, B_y, foldLine.slope, foldLine.intercept);
  let E_x = E_intersect ? Math.round(E_intersect.x) : Math.round((A_x + B_x) / 2);
  let E_y = E_intersect ? Math.round(E_intersect.y) : A_y;

  // D = 折痕候选点中亮度差异最大的点（折痕顶点，在三角形内部）
  // 但不是太靠近边缘的点，取差异最大的候选点
  let D_x = 0, D_y = 0, bestDiff = 0;
  for (const [x, y, diff] of topCandidates) {
    if (diff > bestDiff) {
      bestDiff = diff;
      D_x = x;
      D_y = y;
    }
  }
  console.log(`D 选为差异最大的候选点: (${D_x},${D_y}) diff=${bestDiff.toFixed(1)}`);

  // F = 折痕线与 AC 的交点，如果不在三角形内则沿 AC 搜索
  const F_intersect = lineSegmentIntersect(A_x, A_y, C_x, C_y, foldLine.slope, foldLine.intercept);
  let F_x = F_intersect ? Math.round(F_intersect.x) : Math.round((A_x + C_x) / 2);
  let F_y = F_intersect ? Math.round(F_intersect.y) : Math.round((A_y + C_y) / 2);

  console.log(`关键点: E(${E_x},${E_y}) D(${D_x},${D_y}) F(${F_x},${F_y})`);

  // 验证点在三角形内
  function pointInTriangle(px, py, ax, ay, bx, by, cx, cy) {
    const d1 = (px - bx) * (ay - by) - (ax - bx) * (py - by);
    const d2 = (px - cx) * (by - cy) - (bx - cx) * (py - cy);
    const d3 = (px - ax) * (cy - ay) - (cx - ax) * (py - ay);
    const hasNeg = (d1 < 0) || (d2 < 0) || (d3 < 0);
    const hasPos = (d1 > 0) || (d2 > 0) || (d3 > 0);
    return !(hasNeg && hasPos);
  }

  console.log(`D在三角形内: ${pointInTriangle(D_x, D_y, A_x, A_y, B_x, B_y, C_x, C_y)}`);
  console.log(`F在三角形内: ${pointInTriangle(F_x, F_y, A_x, A_y, B_x, B_y, C_x, C_y)}`);

  // 如果 F 不在三角形内，沿 AC 边搜索
  if (!pointInTriangle(F_x, F_y, A_x, A_y, B_x, B_y, C_x, C_y)) {
    let bestDiff = 0, bestX = F_x, bestY = F_y;
    for (let t = 0.05; t <= 0.95; t += 0.02) {
      const fx = Math.round(A_x + t * (C_x - A_x));
      const fy = Math.round(A_y + t * (C_y - A_y));
      const cx = clamp(fx, 3, w - 4);
      const cy = clamp(fy, 3, h - 4);
      const leftAvg = (pixels[cy * w + (cx - 3)] + pixels[cy * w + (cx - 2)] + pixels[cy * w + (cx - 1)]) / 3;
      const rightAvg = (pixels[cy * w + (cx + 1)] + pixels[cy * w + (cx + 2)] + pixels[cy * w + (cx + 3)]) / 3;
      const diff = Math.abs(rightAvg - leftAvg);
      if (diff > bestDiff) {
        bestDiff = diff;
        bestX = fx; bestY = fy;
      }
    }
    F_x = bestX; F_y = bestY;
    console.log(`F调整到: (${F_x},${F_y})`);
  }

  // 8. 区域分析
  const regions = [
    { key: 'ADE', points: [[A_x, A_y], [D_x, D_y], [E_x, E_y]], desc: '顶部受光面' },
    { key: 'ADF', points: [[A_x, A_y], [D_x, D_y], [F_x, F_y]], desc: '左侧面' },
    { key: 'FDC', points: [[F_x, F_y], [D_x, D_y], [C_x, C_y]], desc: '底部左侧' },
    { key: 'EBCD', points: [[E_x, E_y], [B_x, B_y], [C_x, C_y], [D_x, D_y]], desc: '右侧阴影面' },
  ];

  function pointInPolygon(px, py, polygon) {
    let inside = false;
    const n = polygon.length;
    for (let i = 0, j = n - 1; i < n; j = i++) {
      const xi = polygon[i][0], yi = polygon[i][1];
      const xj = polygon[j][0], yj = polygon[j][1];
      if ((yi > py) !== (yj > py) && px < (xj - xi) * (py - yi) / (yj - yi) + xi) {
        inside = !inside;
      }
    }
    return inside;
  }

  function analyzeRegionGradient(polygon) {
    // 采样区域内像素
    const samples = [];
    const bbox = polygon.reduce((bb, [x, y]) => ({
      minX: Math.min(bb.minX, x), maxX: Math.max(bb.maxX, x),
      minY: Math.min(bb.minY, y), maxY: Math.max(bb.maxY, y),
    }), { minX: w, maxX: 0, minY: h, maxY: 0 });

    const step = 2;
    for (let y = Math.max(0, Math.floor(bbox.minY)); y <= Math.min(h - 1, Math.ceil(bbox.maxY)); y += step) {
      for (let x = Math.max(0, Math.floor(bbox.minX)); x <= Math.min(w - 1, Math.ceil(bbox.maxX)); x += step) {
        if (pointInPolygon(x, y, polygon)) {
          samples.push({
            x, y,
            brightness: pixels[y * w + x],
            r: colorPixels[(y * w + x) * 3],
            g: colorPixels[(y * w + x) * 3 + 1],
            b: colorPixels[(y * w + x) * 3 + 2],
          });
        }
      }
    }

    if (samples.length < 10) {
      return { angle: 0, startColor: '#888888', endColor: '#888888' };
    }

    // 网格化计算亮度梯度方向
    const gridSize = Math.max(3, Math.floor(Math.sqrt(samples.length) / 4));
    const gridMap = new Map();
    for (const s of samples) {
      const gx = Math.floor(s.x / gridSize);
      const gy = Math.floor(s.y / gridSize);
      const key = `${gx},${gy}`;
      const cell = gridMap.get(key) || { sum: 0, count: 0, cx: 0, cy: 0 };
      cell.sum += s.brightness;
      cell.count++;
      cell.cx += s.x;
      cell.cy += s.y;
      gridMap.set(key, cell);
    }

    const cells = Array.from(gridMap.entries()).map(([k, v]) => {
      const [gx, gy] = k.split(',').map(Number);
      return { gx, gy, avg: v.sum / v.count, cx: v.cx / v.count, cy: v.cy / v.count };
    });

    if (cells.length < 4) {
      const avgR = samples.reduce((s, p) => s + p.r, 0) / samples.length;
      const avgG = samples.reduce((s, p) => s + p.g, 0) / samples.length;
      const avgB = samples.reduce((s, p) => s + p.b, 0) / samples.length;
      const toHex = (v) => Math.round(clamp(v, 0, 255)).toString(16).padStart(2, '0');
      return { angle: 0, startColor: `#${toHex(avgR)}${toHex(avgG)}${toHex(avgB)}`, endColor: `#${toHex(avgR)}${toHex(avgG)}${toHex(avgB)}` };
    }

    // 扫描 36 个方向找亮度变化最大的方向
    let bestAngle = 0, bestGradient = 0;
    for (let deg = 0; deg < 180; deg += 5) {
      const rad = (deg * Math.PI) / 180;
      const dirX = Math.cos(rad), dirY = Math.sin(rad);

      const projections = cells.map(c => ({
        proj: c.cx * dirX + c.cy * dirY,
        avg: c.avg,
      }));
      projections.sort((a, b) => a.proj - b.proj);

      const mid = Math.floor(projections.length / 2);
      const frontAvg = projections.slice(0, mid).reduce((s, p) => s + p.avg, 0) / mid;
      const backAvg = projections.slice(mid).reduce((s, p) => s + p.avg, 0) / (projections.length - mid);
      const gradient = Math.abs(frontAvg - backAvg);

      if (gradient > bestGradient) {
        bestGradient = gradient;
        bestAngle = frontAvg > backAvg ? (deg + 180) % 360 : deg;
      }
    }

    // 沿梯度方向采样两端颜色
    const rad = (bestAngle * Math.PI) / 180;
    const dirX = Math.cos(rad), dirY = Math.sin(rad);

    const projs = samples.map(s => ({ ...s, proj: s.x * dirX + s.y * dirY }));
    projs.sort((a, b) => a.proj - b.proj);

    const n = projs.length;
    const nStart = Math.max(3, Math.floor(n * 0.12));
    const nEnd = Math.max(3, Math.floor(n * 0.12));
    const startSamples = projs.slice(0, nStart);
    const endSamples = projs.slice(n - nEnd);

    function avgColor(arr) {
      const r = arr.reduce((s, p) => s + p.r, 0) / arr.length;
      const g = arr.reduce((s, p) => s + p.g, 0) / arr.length;
      const b = arr.reduce((s, p) => s + p.b, 0) / arr.length;
      const toHex = (v) => Math.round(clamp(v, 0, 255)).toString(16).padStart(2, '0');
      return `#${toHex(r)}${toHex(g)}${toHex(b)}`;
    }

    const startColor = avgColor(startSamples);
    const endColor = avgColor(endSamples);

    console.log(`  ${deg}° 梯度=${bestGradient.toFixed(1)} 颜色: ${startColor} -> ${endColor}`);

    return { angle: Math.round(bestAngle), startColor, endColor };
  }

  console.log('\n区域渐变分析:');
  const regionResults = {};
  for (const r of regions) {
    console.log(`  ${r.key} (${r.desc}):`);
    regionResults[r.key] = analyzeRegionGradient(r.points);
  }

  // 9. 生成 SVG
  // 预处理后的图像已经是居中到正方形的，所以 SVG 边距就是 padding
  // 预处理时 padding=0.05 (5%)，再加上垂直偏移
  // 三角形在预处理图像中的位置映射到 SVG 0-100 坐标
  const svgMargin = 5; // 5% padding
  const svgW = 100 - svgMargin * 2;
  const svgH = 100 - svgMargin * 2;

  // 垂直偏移：预处理使用 0.6 偏移，对应垂直方向上的额外偏移
  // 三角形的垂直位置在预处理图像中会偏下
  // 在 SVG 中不需要额外垂直偏移，因为三角形自身就应该居中
  
  // 但对于视觉重心补偿，SVG 中也需要微调
  // 计算三角形在预处理图像中的垂直位置
  const triCenterY = (A_y + B_y + C_y) / 3;
  const triCenterX = (A_x + B_x + C_x) / 3;
  const imageCenterY = h / 2;
  const vertOffset = (imageCenterY - triCenterY) / h; // 负值表示三角形偏上

  function toSvg(x, y) {
    const sx = svgMargin + (x / w) * svgW;
    // 应用垂直偏移补偿
    const sy = svgMargin + (y / h) * svgH;
    return {
      x: Math.round(sx * 10) / 10,
      y: Math.round(sy * 10) / 10,
    };
  }

  const pA = toSvg(A_x, A_y);
  const pB = toSvg(B_x, B_y);
  const pC = toSvg(C_x, C_y);
  const pD = toSvg(D_x, D_y);
  const pE = toSvg(E_x, E_y);
  const pF = toSvg(F_x, F_y);

  console.log(`\nSVG坐标:`);
  console.log(`  A: (${pA.x}, ${pA.y})`);
  console.log(`  B: (${pB.x}, ${pB.y})`);
  console.log(`  C: (${pC.x}, ${pC.y})`);
  console.log(`  D: (${pD.x}, ${pD.y})`);
  console.log(`  E: (${pE.x}, ${pE.y})`);
  console.log(`  F: (${pF.x}, ${pF.y})`);

  const svg = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100" fill="none">
  <defs>
    <!-- ADE: ${regions[0].desc}，渐变方向 ${regionResults.ADE.angle}° -->
    <linearGradient id="g-ade" x1="0" y1="0" x2="0" y2="1" gradientTransform="rotate(${regionResults.ADE.angle})">
      <stop offset="0%" stop-color="${regionResults.ADE.startColor}" />
      <stop offset="100%" stop-color="${regionResults.ADE.endColor}" />
    </linearGradient>

    <!-- ADF: ${regions[1].desc}，渐变方向 ${regionResults.ADF.angle}° -->
    <linearGradient id="g-adf" x1="0" y1="0" x2="0" y2="1" gradientTransform="rotate(${regionResults.ADF.angle})">
      <stop offset="0%" stop-color="${regionResults.ADF.startColor}" />
      <stop offset="100%" stop-color="${regionResults.ADF.endColor}" />
    </linearGradient>

    <!-- FDC: ${regions[2].desc}，渐变方向 ${regionResults.FDC.angle}° -->
    <linearGradient id="g-fdc" x1="0" y1="0" x2="0" y2="1" gradientTransform="rotate(${regionResults.FDC.angle})">
      <stop offset="0%" stop-color="${regionResults.FDC.startColor}" />
      <stop offset="100%" stop-color="${regionResults.FDC.endColor}" />
    </linearGradient>

    <!-- EBCD: ${regions[3].desc}，渐变方向 ${regionResults.EBCD.angle}° -->
    <linearGradient id="g-ebcd" x1="0" y1="0" x2="0" y2="1" gradientTransform="rotate(${regionResults.EBCD.angle})">
      <stop offset="0%" stop-color="${regionResults.EBCD.startColor}" />
      <stop offset="100%" stop-color="${regionResults.EBCD.endColor}" />
    </linearGradient>
  </defs>

  <!-- ADE: ${regions[0].desc} -->
  <polygon points="${pA.x},${pA.y} ${pD.x},${pD.y} ${pE.x},${pE.y}" fill="url(#g-ade)" />

  <!-- ADF: ${regions[1].desc} -->
  <polygon points="${pA.x},${pA.y} ${pD.x},${pD.y} ${pF.x},${pF.y}" fill="url(#g-adf)" />

  <!-- FDC: ${regions[2].desc} -->
  <polygon points="${pF.x},${pF.y} ${pD.x},${pD.y} ${pC.x},${pC.y}" fill="url(#g-fdc)" />

  <!-- EBCD: ${regions[3].desc} -->
  <polygon points="${pE.x},${pE.y} ${pB.x},${pB.y} ${pC.x},${pC.y} ${pD.x},${pD.y}" fill="url(#g-ebcd)" />
</svg>`;

  writeFileSync(join(publicDir, 'favicon.svg'), svg);
  console.log(`\n✓ SVG 已生成: ${join(publicDir, 'favicon.svg')}`);

  console.log('\n=== 坐标摘要 ===');
  console.log(`A=${pA.x},${pA.y} B=${pB.x},${pB.y} C=${pC.x},${pC.y}`);
  console.log(`D=${pD.x},${pD.y} E=${pE.x},${pE.y} F=${pF.x},${pF.y}`);
  console.log(`ADE=${regionResults.ADE.angle}° ADF=${regionResults.ADF.angle}° FDC=${regionResults.FDC.angle}° EBCD=${regionResults.EBCD.angle}°`);
}

analyzeImage().catch(console.error);