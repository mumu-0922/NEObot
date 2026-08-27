import { describe, expect, it } from "vitest";

import {
  describeMessageDuration,
  getLiveMessageDuration,
} from "../lib/utils/messageDuration";

describe("message duration descriptor", () => {
  it("formats sub-second, second, minute, and hour boundaries", () => {
    expect(describeMessageDuration(999)).toEqual({ kind: "lessThanSecond" });
    expect(describeMessageDuration(8_000)).toEqual({
      kind: "seconds",
      seconds: 8,
    });
    expect(describeMessageDuration(120_000)).toEqual({
      kind: "minutes",
      minutes: 2,
    });
    expect(describeMessageDuration(165_000)).toEqual({
      kind: "minutesSeconds",
      minutes: 2,
      seconds: 45,
    });
    expect(describeMessageDuration(3_600_000)).toEqual({
      kind: "hours",
      hours: 1,
    });
    expect(describeMessageDuration(3_780_000)).toEqual({
      kind: "hoursMinutes",
      hours: 1,
      minutes: 3,
    });
  });

  it("rejects invalid and negative durations", () => {
    expect(describeMessageDuration(Number.NaN)).toBeNull();
    expect(describeMessageDuration(Number.POSITIVE_INFINITY)).toBeNull();
    expect(describeMessageDuration(-1)).toBeNull();
  });

  it("derives a non-negative live duration from a stable run start", () => {
    expect(getLiveMessageDuration(10_000, 12_500)).toBe(2_500);
    expect(getLiveMessageDuration(12_500, 10_000)).toBe(0);
    expect(getLiveMessageDuration(Number.NaN, 12_500)).toBeNull();
    expect(getLiveMessageDuration(10_000, Number.POSITIVE_INFINITY)).toBeNull();
  });
});
