export default function requireNever(...args: never[]): never {
  throw new Error(`requireNever: ${args.join(",")}`);
}
