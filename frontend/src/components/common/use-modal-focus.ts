import { useLayoutEffect, useRef, type RefObject } from "react";

const layers: HTMLElement[] = [];
let bodyOverflow = "";
let rootWasInert = false;
const focusable =
  'button:not(:disabled), input:not(:disabled):not([type="hidden"]), select:not(:disabled), textarea:not(:disabled), a[href], [tabindex="0"]';

/** Shared by portaled dialogs and the mobile navigation drawer. */
export function useModalFocus(
  root: RefObject<HTMLElement | null>,
  enabled: boolean,
  onClose: () => void,
) {
  const close = useRef(onClose);
  close.current = onClose;
  useLayoutEffect(() => {
    const el = root.current;
    if (!enabled || !el) return;
    const previous = document.activeElement as HTMLElement | null;
    const app = document.getElementById("root");
    if (!layers.length) {
      bodyOverflow = document.body.style.overflow;
      rootWasInert = app?.inert ?? false;
      if (app) app.inert = true;
      document.body.style.overflow = "hidden";
    }
    if (layers.at(-1)) layers.at(-1)!.inert = true;
    layers.push(el);
    const items = () =>
      Array.from(el.querySelectorAll<HTMLElement>(focusable)).filter(
        (item) =>
          item.tabIndex >= 0 &&
          item.getClientRects().length &&
          !item.closest("[hidden], [inert]"),
      );
    el.focus({ preventScroll: true });
    const owned = (target: Node | null) => {
      if (target && el.contains(target)) return true;
      return Array.from(el.querySelectorAll("[aria-controls]")).some(
        (trigger) =>
          trigger
            .getAttribute("aria-controls")
            ?.split(/\s+/)
            .some((id) => document.getElementById(id)?.contains(target)),
      );
    };
    const key = (event: KeyboardEvent) => {
      if (layers.at(-1) !== el || event.defaultPrevented) return;
      // A nested listbox/menu/calendar handles its own Escape first.
      if (
        event.key === "Escape" &&
        !el.querySelector('[aria-expanded="true"], .calendar-panel')
      ) {
        event.preventDefault();
        close.current();
      }
      if (event.key === "Tab") {
        const controls = items(),
          first = controls[0],
          last = controls.at(-1);
        if (!first) {
          event.preventDefault();
          el.focus();
          return;
        }
        if (
          event.shiftKey &&
          (document.activeElement === first ||
            document.activeElement === el ||
            !owned(document.activeElement))
        ) {
          event.preventDefault();
          last?.focus();
        } else if (
          !event.shiftKey &&
          (document.activeElement === last || !owned(document.activeElement))
        ) {
          event.preventDefault();
          first.focus();
        }
      }
    };
    const focus = (event: FocusEvent) => {
      if (layers.at(-1) === el && !owned(event.target as Node))
        el.focus({ preventScroll: true });
    };
    document.addEventListener("keydown", key);
    document.addEventListener("focusin", focus);
    return () => {
      document.removeEventListener("keydown", key);
      document.removeEventListener("focusin", focus);
      layers.splice(layers.indexOf(el), 1);
      if (layers.at(-1)) layers.at(-1)!.inert = false;
      else {
        document.body.style.overflow = bodyOverflow;
        if (app) app.inert = rootWasInert;
      }
      if (previous?.isConnected && !previous.closest("[inert]"))
        previous.focus({ preventScroll: true });
    };
  }, [enabled, root]);
}
