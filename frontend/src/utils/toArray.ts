function toArray<T>(v: T[] | T | null | undefined): T[];
function toArray<T>(v: readonly T[] | T | null | undefined): readonly T[];
function toArray<T>(v: T[] | T | null | undefined): T[] {
  if (v == null) {
    return [];
  }
  if (Array.isArray(v)) {
    return v;
  }
  return [v];
}

export default toArray;
