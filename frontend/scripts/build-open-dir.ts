import * as fs from 'fs';
import * as path from 'path';
import { fileURLToPath } from 'url';
import { zipSync, strToU8 } from 'fflate';

// 获取当前脚本所在目录
const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);

const SCRIPT_DIR = __dirname;
const OPEN_DIR_SRC = path.join(SCRIPT_DIR, 'open-dir');

// 输出路径参数，默认放到前端的 public/static/open-dir/windows
const outputDir = path.resolve(SCRIPT_DIR, '../public/static/open-dir/windows');

console.log(`自动编译并打包本地路径协议插件到 ${outputDir} ...`);

if (!fs.existsSync(outputDir)) {
  fs.mkdirSync(outputDir, { recursive: true });
}

// 编译 ps1 文件为 CMD 文本的辅助函数
function buildPS1AsCmd(sourcePath: string, launchArgs: string = '-NoProfile -Sta'): string {
  const ps1Content = fs.readFileSync(sourcePath, 'utf-8');
  // 统一换行符为 \r\n
  const data = ps1Content.replace(/\r?\n/g, '\r\n');
  
  const wrapper = [
    '@echo off',
    'SETLOCAL EnableDelayedExpansion',
    'SET "ARG0=%0"',
    'SET /A index=1',
    'FOR %%i in (%*) DO (',
    '  SET "ARG!index!=%%i"',
    '  SET /A index+=1',
    ')',
    `PowerShell ${launchArgs} -Command "Get-Content '%~dpnx0' -Encoding UTF8 | Select-Object -Skip 12 | Out-String | Invoke-Expression"`,
    'IF ERRORLEVEL 1 PAUSE',
    'GOTO :EOF',
    '',
    ''
  ].join('\r\n');

  return wrapper + data;
}

try {
  const setupPs1Path = path.join(OPEN_DIR_SRC, 'setup.ps1');
  const handlePs1Path = path.join(OPEN_DIR_SRC, 'handle-protocol.ps1');
  const installCmdSrcPath = path.join(OPEN_DIR_SRC, '安装.cmd');

  // 编译出自包含 CMD 内容
  const setupCmdContent = buildPS1AsCmd(setupPs1Path);
  const handleCmdContent = buildPS1AsCmd(handlePs1Path);
  const installCmdContent = fs.readFileSync(installCmdSrcPath, 'utf-8').replace(/\r?\n/g, '\r\n');

  // 准备压缩文件数据结构：fflate 的 zipSync 接收一个对象，key 为文件名，value 为 Uint8Array
  const zipData: { [key: string]: Uint8Array } = {
    '安装.cmd': strToU8(installCmdContent),
    'setup.ps1.cmd': strToU8(setupCmdContent),
    'handle-protocol.ps1.cmd': strToU8(handleCmdContent),
    '!解压到单独文件夹后再安装，不支持直接在压缩软件中运行': strToU8('')
  };

  // 生成 zip 二进制数据
  const zipped = zipSync(zipData);

  // 写入最终目标路径
  const zipOutputPath = path.join(outputDir, 'setup.zip');
  fs.writeFileSync(zipOutputPath, zipped);

  console.log(`✅ 本地路径协议插件打包完成: ${zipOutputPath}`);
} catch (error) {
  console.error('❌ 协议自包含插件打包失败:', error);
  process.exit(1);
}
