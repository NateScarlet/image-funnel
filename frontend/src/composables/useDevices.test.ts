import { describe, test, expect, vi } from "vitest";

vi.mock("./domain/useDevice", () => {
  return {
    default: () => ({
      pairingRequests: { value: [] },
      devices: { value: [] },
      isTrustedDevice: { value: false },
      refreshDevices: vi.fn(),
      refreshAuthStatus: vi.fn(),
      refreshPairingRequests: vi.fn(),
      onSaved: vi.fn(),
      onDeleted: vi.fn(),
    }),
  };
});

import { useDevices } from "./useDevices";

describe("useDevices composable", () => {
  test("shares visible state between callers and opens drawer", () => {
    const d1 = useDevices();
    const d2 = useDevices();

    expect(d1.visible.value).toBe(false);
    expect(d2.visible.value).toBe(false);

    d1.open();

    expect(d1.visible.value).toBe(true);
    expect(d2.visible.value).toBe(true);

    d2.close();

    expect(d1.visible.value).toBe(false);
    expect(d2.visible.value).toBe(false);
  });
});
