export default function equalArray<T extends unknown[]>(
  a: readonly [...T],
  b: readonly [...T],
  {
    equal = (a, b) => a === b,
  }: {
    equal?: (a: T[number], b: T[number]) => boolean;
  } = {},
): boolean {
  if (a.length !== b.length) {
    return false;
  }

  return a.every((v, index) => equal(v, b[index]));
}
