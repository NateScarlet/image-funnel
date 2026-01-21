export type TimeInput = Date | Time | number | string;

export default class Time {
  constructor(
    private readonly unix: number,
    private readonly monotonic?: number,
  ) {}

  public static from(input: TimeInput): Time | undefined {
    if (input instanceof Time) {
      return input;
    }
    if (input instanceof Date) {
      const t = input.getTime();
      if (!Number.isFinite(t)) {
        return;
      }
      return new Time(t);
    }
    return this.from(new Date(input));
  }

  public static now() {
    return new this(Date.now(), performance.timeOrigin + performance.now());
  }

  public toDate() {
    return new Date(this.unix);
  }

  public [Symbol.toPrimitive]() {
    return this.getTime();
  }

  public getTime(): number {
    return this.unix;
  }

  public sub(other: Time): number {
    if (this.monotonic != null && other.monotonic != null) {
      return this.monotonic - other.monotonic;
    }
    return this.unix - other.unix;
  }

  public add(durationMs: number): Time {
    if (this.monotonic != null) {
      return new Time(this.unix + durationMs, this.monotonic + durationMs);
    }
    return new Time(this.unix + durationMs);
  }

  public equal(other: Time): boolean {
    if (this.monotonic != null && other.monotonic != null) {
      return this.monotonic === other.monotonic;
    }
    return this.unix === other.unix;
  }

  public toISOString() {
    return this.toDate().toISOString();
  }
}
