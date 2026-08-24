export type MessageDurationDescriptor =
  | { kind: "lessThanSecond" }
  | { kind: "seconds"; seconds: number }
  | { kind: "minutes"; minutes: number }
  | { kind: "minutesSeconds"; minutes: number; seconds: number }
  | { kind: "hours"; hours: number }
  | { kind: "hoursMinutes"; hours: number; minutes: number };

export function describeMessageDuration(
  durationMs: number,
): MessageDurationDescriptor | null {
  if (!Number.isFinite(durationMs) || durationMs < 0) return null;
  if (durationMs < 1000) return { kind: "lessThanSecond" };

  const totalSeconds = Math.round(durationMs / 1000);
  if (totalSeconds < 60) return { kind: "seconds", seconds: totalSeconds };

  const totalMinutes = Math.floor(totalSeconds / 60);
  const seconds = totalSeconds % 60;
  if (totalMinutes < 60) {
    return seconds === 0
      ? { kind: "minutes", minutes: totalMinutes }
      : { kind: "minutesSeconds", minutes: totalMinutes, seconds };
  }

  const hours = Math.floor(totalMinutes / 60);
  const minutes = totalMinutes % 60;
  return minutes === 0
    ? { kind: "hours", hours }
    : { kind: "hoursMinutes", hours, minutes };
}
