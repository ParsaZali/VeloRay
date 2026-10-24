export type Notice = { message: string; tone: "success" | "error" | "info" };
export function notify(message: string, tone: Notice["tone"] = "info") {
  window.dispatchEvent(
    new CustomEvent<Notice>("veloray:notice", { detail: { message, tone } }),
  );
}
