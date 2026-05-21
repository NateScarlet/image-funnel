export default function assertNever(...args: never[]): void {
  if (import.meta.env.DEV) {
    console.warn("assertNever", ...args);
  }
}
