import { sha256 } from "@noble/hashes/sha2.js";

const sha256HashBytes: (data: Uint8Array) => Promise<ArrayBufferLike> = (() => {
  if (
    typeof window !== "undefined" &&
    window.isSecureContext &&
    typeof crypto !== "undefined" &&
    typeof crypto.subtle?.digest === "function"
  ) {
    return async (data: Uint8Array) =>
      crypto.subtle.digest("SHA-256", data as Uint8Array<ArrayBuffer>);
  }
  return async (data: Uint8Array) => sha256(data).buffer;
})();

export default sha256HashBytes;
