import hexEncode from "./hexEncode";
import sha256HashBytes from "./sha256HashBytes";

export default async function sha256Hash(s: string): Promise<string> {
  const encoder = new TextEncoder();
  const data = encoder.encode(s);
  return hexEncode(new Uint8Array(await sha256HashBytes(data)));
}
