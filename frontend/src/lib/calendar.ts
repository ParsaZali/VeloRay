export type InputCalendar = "gregory" | "persian";
const dayMs = 86400000;
export function civilToday() {
  const d = new Date();
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;
}
export function civilDate(value: string) {
  const date = new Date(`${value.slice(0, 10)}T12:00:00Z`);
  return Number.isFinite(date.getTime())
    ? date
    : new Date(`${civilToday()}T12:00:00Z`);
}
export const civilISO = (date: Date) => date.toISOString().slice(0, 10);
export function addCivilDays(value: string, days: number) {
  return civilISO(new Date(civilDate(value).getTime() + days * dayMs));
}
// UTC civil noon avoids DST shifts while input values stay Gregorian YYYY-MM-DD.
export function calendarMonth(
  value: string,
  calendar: InputCalendar,
  locale: string,
) {
  const anchor = civilDate(value),
    parts = new Intl.DateTimeFormat("en", {
      calendar,
      numberingSystem: "latn",
      timeZone: "UTC",
      year: "numeric",
      month: "numeric",
      day: "numeric",
    }).formatToParts(anchor);
  const number = (key: string) =>
    Number(parts.find((p) => p.type === key)?.value || 1);
  const first = new Date(anchor.getTime() - (number("day") - 1) * dayMs);
  const monthId = new Intl.DateTimeFormat("en", {
    calendar,
    numberingSystem: "latn",
    timeZone: "UTC",
    year: "numeric",
    month: "numeric",
  });
  const expected = monthId.format(first);
  const dates: string[] = [];
  for (let i = 0; i < 32; i++) {
    const date = new Date(first.getTime() + i * dayMs);
    if (monthId.format(date) !== expected) break;
    dates.push(civilISO(date));
  }
  const weekday = calendar === "persian" ? 6 : 0;
  const dayFormat = new Intl.DateTimeFormat(locale, {
    calendar,
    timeZone: "UTC",
    day: "numeric",
  });
  const fullFormat = new Intl.DateTimeFormat(locale, {
    calendar,
    timeZone: "UTC",
    dateStyle: "full",
  });
  const title = new Intl.DateTimeFormat(locale, {
    calendar,
    timeZone: "UTC",
    year: "numeric",
    month: "long",
  }).format(first);
  return {
    title,
    dates,
    offset: (first.getUTCDay() + 7 - weekday) % 7,
    first: dates[0],
    previous: addCivilDays(dates[0], -1),
    next: addCivilDays(dates[dates.length - 1], 1),
    labels: dates.map((value) => dayFormat.format(civilDate(value))),
    fullLabels: dates.map((value) => fullFormat.format(civilDate(value))),
    weekdays: Array.from({ length: 7 }, (_, i) =>
      new Intl.DateTimeFormat(locale, {
        weekday: "short",
        timeZone: "UTC",
      }).format(new Date(Date.UTC(2024, 0, 7 + weekday + i, 12))),
    ),
  };
}
export function inputCalendar(
  preference: string,
  locale: string,
): InputCalendar {
  return preference === "persian" ||
    (preference === "auto" && locale.startsWith("fa"))
    ? "persian"
    : "gregory";
}
