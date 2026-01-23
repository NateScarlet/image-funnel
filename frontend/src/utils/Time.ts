export type TimeInput = Date | Time | number | string;
export type TimeSource =
  | TimeInput
  | null
  | undefined
  | Iterable<TimeInput | null | undefined>;

export default class Time {
  constructor(
    private readonly unix: number,
    private readonly monotonic?: number,
  ) {}

  public static from(input: TimeInput | null | undefined): Time | undefined {
    if (input == null) {
      return;
    }
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

  public static *collect(source: TimeSource): Iterable<Time> {
    if (source == null) {
      return;
    }
    switch (typeof source) {
      case "string":
      case "number": {
        const v = this.from(source);
        if (v) {
          yield v;
        }
        return;
      }
      case "object":
        if (Symbol.iterator in source) {
          for (const i of source) {
            const v = this.from(i);
            if (v) {
              yield v;
            }
          }
        } else {
          const v = this.from(source);
          if (v) {
            yield v;
          }
        }
        break;
      default:
    }
  }

  public static now() {
    return new this(Date.now(), performance.timeOrigin + performance.now());
  }

  public static max(
    values: Iterable<Time | null | undefined>,
  ): Time | undefined {
    let ret: Time | undefined;
    for (const value of values) {
      if (value == null) {
        continue;
      }
      if (ret == null || value.compare(ret) > 0) {
        ret = value;
      }
    }
    return ret;
  }

  public static min(
    values: Iterable<Time | null | undefined>,
  ): Time | undefined {
    let ret: Time | undefined;
    for (const value of values) {
      if (value == null) {
        continue;
      }
      if (ret == null || value.compare(ret) < 0) {
        ret = value;
      }
    }
    return ret;
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

  public compare(other: Time): -1 | 0 | 1 {
    if (this.monotonic != null && other.monotonic != null) {
      if (this.monotonic > other.monotonic) {
        return 1;
      }
      if (this.monotonic < other.monotonic) {
        return -1;
      }
      return 0;
    }
    if (this.unix > other.unix) {
      return 1;
    }
    if (this.unix < other.unix) {
      return -1;
    }
    return 0;
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
    return this.compare(other) === 0;
  }

  public toISOString() {
    return this.toDate().toISOString();
  }
}
