import { createRequire } from 'module';
const require = createRequire(import.meta.url);
const sharp = require('/root/.nvm/versions/node/v24.1.0/lib/node_modules/sharp');
import { writeFileSync } from 'fs';
import { fileURLToPath } from 'url';
import { dirname, join } from 'path';

const __filename = fileURLToPath(import.meta.url);
const __dirname = dirname(__filename);

// 直接读取已处理好的 PNG（已去背景、裁剪、居中）
const pngSource = join(__dirname, '../public/pwa-512x512.png');
const publicDir = join(__dirname, '../public');

const ANALYZE_SIZE = 256;
const BLUR_RADIUS = 1;

// #region 工具函数
function clamp(v, min, max) { return Math.max(min, Math.min(max, v)); }

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

function lineSegmentIntersect(ax, ay, bx, by, slope, intercept) {
  const denom = (by - ay) - slope * (bx - ax);
  if (Math.abs(denom) < 1e-10) return null;
  const t = (slope * ax + intercept - ay) / denom;
  if (t < 0 || t > 1) return null;
  return { x: ax + t * (bx - ax), y: ay + t * (by - ay), t };
}
// #endregion

async function analyzeImage() {
  // 1. 加载已处理的 PNG
  // 为了正确获取颜色，先 flatten 到白色背景（避免预乘 alpha 导致的颜色失真）
  const { data: flatData, info: flatInfo } = await sharp(pngSource)
    .resize(ANALYZE_SIZE, ANALYZE_SIZE, { fit: 'fill' })
    .blur(BLUR_RADIUS)
    .flatten({ background: { r: 255, g: 255, b: 255, alpha: 1 } }) // 透明→白色
    .raw()
    .toBuffer({ resolveWithObject: true });
  const w = flatInfo.width, h = flatInfo.height;
  const flatPixels = new Uint8Array(flatData);

  // 灰度图用于亮度分析（不 flatten，用 alpha 通道过滤背景）
  const { data: grayData } = await sharp(pngSource)
    .resize(ANALYZE_SIZE, ANALYZE_SIZE, { fit: 'fill' })
    .blur(BLUR_RADIUS)
    .grayscale()
    .raw()
    .toBuffer({ resolveWithObject: true });
  const pixels = new Uint8Array(grayData);

  // 单独获取 alpha 通道（不 flatten，保留原始透明信息）
  const { data: rgbaData } = await sharp(pngSource)
    .resize(ANALYZE_SIZE, ANALYZE_SIZE, { fit: 'fill' })
    .blur(BLUR_RADIUS)
    .ensureAlpha()
    .raw()
    .toBuffer({ resolveWithObject: true });
  const alphaPixels = new Uint8Array(rgbaData);

  console.log(`分析图像: ${w}x${h}，来源: pwa-512x512.png`);

  // 2. 找到三角形边界（非透明像素）
  let minX = w, minY = h, maxX = 0, maxY = 0;
  const edgeMargin = 2;
  for (let y = edgeMargin; y < h - edgeMargin; y++) {
    for (let x = edgeMargin; x < w - edgeMargin; x++) {
      const alpha = alphaPixels[(y * w + x) * 4 + 3]; // alpha 通道
      // 有像素且不是纯白背景
      if (alpha > 128) {
        if (x < minX) minX = x;
        if (y < minY) minY = y;
        if (x > maxX) maxX = x;
        if (y > maxY) maxY = y;
      }
    }
  }
  console.log(`三角形边界框: (${minX},${minY}) - (${maxX},${maxY})`);

  // 3. 找到三角形三个角点
  // 上边缘：找最左和最右的非透明像素
  let topLeftX = w, topRightX = 0;
  const topScanY = Math.min(minY + 4, h - 1);
  for (let x = minX; x <= maxX; x++) {
    const alpha = alphaPixels[(topScanY * w + x) * 4 + 3];
    if (alpha > 128) {
      if (x < topLeftX) topLeftX = x;
      if (x > topRightX) topRightX = x;
    }
  }

  // 左右边缘拟合
  const leftEdgePoints = [];
  const rightEdgePoints = [];
  for (let y = minY + 2; y <= maxY - 2; y++) {
    for (let x = minX; x <= maxX; x++) {
      const alpha = alphaPixels[(y * w + x) * 4 + 3];
      if (alpha > 128) {
        leftEdgePoints.push([x, y]);
        break;
      }
    }
    for (let x = maxX; x >= minX; x--) {
      const alpha = alphaPixels[(y * w + x) * 4 + 3];
      if (alpha > 128) {
        rightEdgePoints.push([x, y]);
        break;
      }
    }
  }

  const leftLine = linearRegression(leftEdgePoints);
  const rightLine = linearRegression(rightEdgePoints);

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

  // 4. 检测折痕线（在三角形内部找亮度跳变最大的位置，排除边缘像素）
  const foldCandidates = [];
  for (let y = Math.max(A_y + 8, minY + 8); y < Math.min(C_y, maxY - 4); y += 2) {
    let rowLeft = w, rowRight = 0;
    for (let x = Math.max(A_x, minX); x <= Math.min(B_x, maxX); x++) {
      const alpha = alphaPixels[(y * w + x) * 4 + 3];
      if (alpha > 128) {
        if (x < rowLeft) rowLeft = x;
        if (x > rowRight) rowRight = x;
      }
    }
    if (rowLeft >= rowRight) continue;

    // 排除边缘 15% 的像素，只分析内部区域
    const margin = Math.round((rowRight - rowLeft) * 0.15);
    const searchLeft = rowLeft + margin;
    const searchRight = rowRight - margin;
    if (searchLeft >= searchRight) continue;

    let maxDiff = 0, maxDiffX = searchLeft;
    for (let x = searchLeft + 4; x < searchRight - 4; x++) {
      const leftAvg = (pixels[y * w + (x - 4)] + pixels[y * w + (x - 3)] + pixels[y * w + (x - 2)]) / 3;
      const rightAvg = (pixels[y * w + (x + 2)] + pixels[y * w + (x + 3)] + pixels[y * w + (x + 4)]) / 3;
      const diff = Math.abs(rightAvg - leftAvg);
      if (diff > maxDiff) {
        maxDiff = diff;
        maxDiffX = x;
      }
    }

    if (maxDiff > 10) {
      foldCandidates.push([maxDiffX, y, maxDiff]);
    }
  }

  console.log(`折痕候选点: ${foldCandidates.length} 个`);

  // 5. 拟合折痕线
  foldCandidates.sort((a, b) => b[2] - a[2]);
  const topCandidates = foldCandidates.slice(0, Math.min(40, foldCandidates.length));
  topCandidates.slice(0, 5).forEach(([x, y, d], i) => {
    console.log(`  候选 #${i + 1}: (${x},${y}) diff=${d.toFixed(1)}`);
  });

  const foldLine = linearRegression(topCandidates.map(p => [p[0], p[1]]));
  console.log(`折痕线: y = ${foldLine.slope.toFixed(4)}x + ${foldLine.intercept.toFixed(4)}`);

  // 6. 计算关键点 E, D, F
  // D = 用户测量值：pwa-192x192 上 (134, 75) → 分析图 256x256 → (179, 100)
  const D_x = 179, D_y = 100;
  console.log(`D 固定为用户测量值: (${D_x},${D_y})`);

  // E = 折痕线（通过 D 和折痕候选点）与 AB 边（上边缘）的交点
  let E_x = Math.round((A_x + B_x) / 2), E_y = A_y;
  const upperCandidates = foldCandidates.filter(([cx, cy]) => cy < D_y);
  if (upperCandidates.length > 0) {
    // 计算从 D 到上方候选点的平均方向
    const avgDx = upperCandidates.reduce((s, [cx]) => s + (cx - D_x), 0) / upperCandidates.length;
    const avgDy = upperCandidates.reduce((s, [, cy]) => s + (cy - D_y), 0) / upperCandidates.length;
    const foldSlope = avgDy / avgDx;
    const eX = Math.round((A_y - D_y) / foldSlope + D_x);
    if (eX >= A_x && eX <= B_x) {
      E_x = eX; E_y = A_y;
      console.log(`E 通过 D→候选点方向计算: (${E_x},${E_y})`);
    }
  }
  if (E_x === Math.round((A_x + B_x) / 2) || E_x < A_x || E_x > B_x) {
    // 容错：使用折痕线回归
    const foldLine = linearRegression(foldCandidates.map(p => [p[0], p[1]]));
    const E_intersect = lineSegmentIntersect(A_x, A_y, B_x, B_y, foldLine.slope, foldLine.intercept);
    if (E_intersect) { E_x = Math.round(E_intersect.x); E_y = A_y; }
    console.log(`E 通过折痕线回归计算: (${E_x},${E_y})`);
  }

  // F = 沿 AC 边搜索亮度跳变最大的点（D→F 为阴影分界线）
  // 沿 AC 边采样，找亮度差异最大的位置
  let F_x = Math.round((A_x + C_x) / 2), F_y = Math.round((A_y + C_y) / 2);
  let bestFDiff = 0;
  for (let t = 0.05; t <= 0.95; t += 0.02) {
    const fx = Math.round(A_x + t * (C_x - A_x));
    const fy = Math.round(A_y + t * (C_y - A_y));
    const cx = clamp(fx, 3, w - 4);
    const cy = clamp(fy, 3, h - 4);
    const leftAvg = (pixels[cy * w + (cx - 3)] + pixels[cy * w + (cx - 2)] + pixels[cy * w + (cx - 1)]) / 3;
    const rightAvg = (pixels[cy * w + (cx + 1)] + pixels[cy * w + (cx + 2)] + pixels[cy * w + (cx + 3)]) / 3;
    const diff = Math.abs(rightAvg - leftAvg);
    if (diff > bestFDiff) {
      bestFDiff = diff;
      F_x = fx; F_y = fy;
    }
  }
  console.log(`F 沿 AC 搜索: (${F_x},${F_y}) diff=${bestFDiff.toFixed(1)}`);

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

  // 7. 区域分析
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
    const samples = [];
    const bbox = polygon.reduce((bb, [x, y]) => ({
      minX: Math.min(bb.minX, x), maxX: Math.max(bb.maxX, x),
      minY: Math.min(bb.minY, y), maxY: Math.max(bb.maxY, y),
    }), { minX: w, maxX: 0, minY: h, maxY: 0 });

    const step = 2;
    for (let y = Math.max(0, Math.floor(bbox.minY)); y <= Math.min(h - 1, Math.ceil(bbox.maxY)); y += step) {
      for (let x = Math.max(0, Math.floor(bbox.minX)); x <= Math.min(w - 1, Math.ceil(bbox.maxX)); x += step) {
        const alpha = alphaPixels[(y * w + x) * 4 + 3];
        if (alpha > 128 && pointInPolygon(x, y, polygon)) {
          samples.push({
            x, y,
            brightness: pixels[y * w + x],
            r: flatPixels[(y * w + x) * 3],
            g: flatPixels[(y * w + x) * 3 + 1],
            b: flatPixels[(y * w + x) * 3 + 2],
          });
        }
      }
    }

    if (samples.length < 10) {
      return { angle: 0, startColor: '#888888', endColor: '#888888' };
    }

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

    let bestAngle = 0, bestGradient = 0;
    for (let deg = 0; deg < 180; deg += 5) {
      const rad = (deg * Math.PI) / 180;
      const dirX = Math.cos(rad), dirY = Math.sin(rad);
      const projections = cells.map(c => ({ proj: c.cx * dirX + c.cy * dirY, avg: c.avg }));
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

    const rad = (bestAngle * Math.PI) / 180;
    const dirX = Math.cos(rad), dirY = Math.sin(rad);
    const projs = samples.map(s => ({ ...s, proj: s.x * dirX + s.y * dirY }));
    projs.sort((a, b) => a.proj - b.proj);

    const n = projs.length;
    const nStart = Math.max(3, Math.floor(n * 0.05));
    const nEnd = Math.max(3, Math.floor(n * 0.05));
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

    console.log(`  ${bestAngle}° 梯度=${bestGradient.toFixed(1)} 颜色: ${startColor} -> ${endColor}`);

    return { angle: Math.round(bestAngle), startColor, endColor };
  }

  console.log('\n区域渐变分析:');
  const regionResults = {};
  for (const r of regions) {
    console.log(`  ${r.key} (${r.desc}):`);
    if (r.key === 'EBCD') {
      // EBCD: 基于边 ED 和 DC 的阴影
      // 阴影方向：从折痕顶点 D 朝向 BC 边（最暗在折痕处，最亮在外侧）
      const bcMidX = (B_x + C_x) / 2;
      const bcMidY = (B_y + C_y) / 2;
      const shadowDx = bcMidX - D_x;
      const shadowDy = bcMidY - D_y;
      const shadowAngle = (Math.atan2(shadowDy, shadowDx) * 180 / Math.PI + 360) % 360;
      console.log(`  阴影方向: ${shadowAngle.toFixed(1)}° (D → BC中点)`);

      // 沿阴影方向采样颜色
      const rad = (shadowAngle * Math.PI) / 180;
      const dirX = Math.cos(rad), dirY = Math.sin(rad);
      const samples = [];
      const bbox = r.points.reduce((bb, [x, y]) => ({
        minX: Math.min(bb.minX, x), maxX: Math.max(bb.maxX, x),
        minY: Math.min(bb.minY, y), maxY: Math.max(bb.maxY, y),
      }), { minX: w, maxX: 0, minY: h, maxY: 0 });

      for (let y = Math.max(0, Math.floor(bbox.minY)); y <= Math.min(h - 1, Math.ceil(bbox.maxY)); y += 2) {
        for (let x = Math.max(0, Math.floor(bbox.minX)); x <= Math.min(w - 1, Math.ceil(bbox.maxX)); x += 2) {
          const alpha = alphaPixels[(y * w + x) * 4 + 3];
          if (alpha > 128 && pointInPolygon(x, y, r.points)) {
            samples.push({
              x, y,
              r: flatPixels[(y * w + x) * 3],
              g: flatPixels[(y * w + x) * 3 + 1],
              b: flatPixels[(y * w + x) * 3 + 2],
              proj: x * dirX + y * dirY,
            });
          }
        }
      }

      if (samples.length >= 10) {
        samples.sort((a, b) => a.proj - b.proj);
        const n = samples.length;
        const nStart = Math.max(3, Math.floor(n * 0.05));
        const nEnd = Math.max(3, Math.floor(n * 0.05));
        const startSamples = samples.slice(0, nStart);
        const endSamples = samples.slice(n - nEnd);

        const avgColor = (arr) => {
          const r = arr.reduce((s, p) => s + p.r, 0) / arr.length;
          const g = arr.reduce((s, p) => s + p.g, 0) / arr.length;
          const b = arr.reduce((s, p) => s + p.b, 0) / arr.length;
          const toHex = (v) => Math.round(clamp(v, 0, 255)).toString(16).padStart(2, '0');
          return `#${toHex(r)}${toHex(g)}${toHex(b)}`;
        };

        const startColor = avgColor(startSamples);
        const endColor = avgColor(endSamples);
        // SVG 旋转角度：默认上→下(0°)，阴影方向从 x 轴逆时针测量
        // 投影方向 (cos(θ), sin(θ)) 对应 SVG rotate(90-θ)
        const svgAngle = ((90 - shadowAngle) % 360 + 360) % 360;
        regionResults.EBCD = { angle: Math.round(svgAngle), startColor, endColor };
        console.log(`  ${svgAngle.toFixed(0)}° 阴影: ${startColor} -> ${endColor}`);
      } else {
        regionResults.EBCD = analyzeRegionGradient(r.points);
      }
    } else {
      regionResults[r.key] = analyzeRegionGradient(r.points);
    }
  }

  // 8. 映射到 SVG 坐标（处理后的 PNG 坐标直接线性映射到 SVG 0-100）

  // 直接按比例映射到 SVG 0-100 坐标
  function toSvg(px, py) {
    return {
      x: Math.round((px / w) * 100 * 10) / 10,
      y: Math.round((py / h) * 100 * 10) / 10,
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

  // 9. 生成 SVG
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

    <!-- EBCD: ${regions[3].desc}，基于边ED/DC的阴影，渐变方向 ${regionResults.EBCD.angle}° -->
    <linearGradient id="g-ebcd" x1="0" y1="0" x2="0" y2="1" gradientTransform="rotate(${regionResults.EBCD.angle})">
      <stop offset="0%" stop-color="${regionResults.EBCD.startColor}" />
      <stop offset="100%" stop-color="${regionResults.EBCD.endColor}" />
    </linearGradient>
  </defs>

  <!-- 先画整个三角形背景，消除区域间白边 -->
  <polygon points="${pA.x},${pA.y} ${pB.x},${pB.y} ${pC.x},${pC.y}" fill="#fc881f" />

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