export default function isUnion<T>(
  v: unknown,
  ...typeGuards: ((v: unknown) => v is T)[]
): v is T {
  return typeGuards.some((i) => i(v));
}
