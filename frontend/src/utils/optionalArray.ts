export default function optionalArray<T extends readonly unknown[]>(
  v: T | null | undefined,
): T | undefined {
  if (v == null || v.length === 0) {
    return;
  }
  return v;
}
