export default function basename(unixOrWindowsPath: string): string {
  return unixOrWindowsPath.split(/[\\/]/).pop() || "";
}
