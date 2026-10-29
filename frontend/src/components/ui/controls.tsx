import { cn } from "@/lib/cn";
import { datePreferences } from "@/lib/dates";
import {
  calendarMonth,
  civilToday,
  addCivilDays,
  inputCalendar,
} from "@/lib/calendar";
import { localeName } from "@/lib/i18n";
import {
  Children,
  isValidElement,
  useEffect,
  useLayoutEffect,
  useId,
  useRef,
  useState,
  type ChangeEvent,
  type InputHTMLAttributes,
  type TextareaHTMLAttributes,
  type SelectHTMLAttributes,
  type ReactNode,
} from "react";
import { createPortal } from "react-dom";
import {
  Check,
  ChevronDown,
  ChevronLeft,
  ChevronRight,
  CalendarDays,
  X,
} from "lucide-react";
import { t } from "@/lib/i18n";

function useControlLabel(
  ref: React.RefObject<
    HTMLButtonElement | HTMLInputElement | HTMLTextAreaElement | null
  >,
  provided?: string,
) {
  const [label, setLabel] = useState(provided);
  useEffect(() => {
    if (provided) {
      setLabel(provided);
      return;
    }
    const source = ref.current?.labels?.[0];
    if (!source) return;
    const clone = source.cloneNode(true) as HTMLElement;
    clone
      .querySelectorAll(
        "button,input,select,textarea,.date-control,.field-hint,.label > span",
      )
      .forEach((el) => el.remove());
    setLabel(clone.textContent?.trim());
  });
  return label;
}
type Option = { value: string; label: ReactNode; disabled: boolean };
export function Select({
  children,
  value,
  defaultValue,
  onChange,
  className = "",
  disabled,
  id,
  name,
  required,
  ...rest
}: SelectHTMLAttributes<HTMLSelectElement>) {
  const options: Option[] = [];
  const collect = (nodes: ReactNode) =>
    Children.forEach(nodes, (node) => {
      if (
        !isValidElement<{
          value?: string | number;
          children?: ReactNode;
          disabled?: boolean;
        }>(node)
      )
        return;
      if (node.type === "option")
        options.push({
          value: String(node.props.value ?? node.props.children ?? ""),
          label: node.props.children,
          disabled: !!node.props.disabled,
        });
      else collect(node.props.children);
    });
  collect(children);
  const [own, setOwn] = useState(
      String(defaultValue ?? options[0]?.value ?? ""),
    ),
    [open, setOpen] = useState(false),
    [active, setActive] = useState(0),
    [rect, setRect] = useState<DOMRect | null>(null);
  const selected = String(value ?? own),
    ref = useRef<HTMLButtonElement>(null),
    list = useRef<HTMLDivElement>(null),
    uid = useId(),
    search = useRef({ text: "", at: 0 });
  const controlLabel = useControlLabel(ref, rest["aria-label"]);
  const choose = (o: Option) => {
    if (o.disabled) return;
    setOwn(o.value);
    onChange?.({
      target: { value: o.value, name },
      currentTarget: { value: o.value, name },
    } as ChangeEvent<HTMLSelectElement>);
    setOpen(false);
    ref.current?.focus();
  };
  useLayoutEffect(() => {
    if (!open) return;
    const close = (e: PointerEvent) => {
      if (
        !ref.current?.contains(e.target as Node) &&
        !list.current?.contains(e.target as Node)
      )
        setOpen(false);
    };
    const layout = () => setRect(ref.current?.getBoundingClientRect() ?? null);
    document.addEventListener("pointerdown", close);
    window.addEventListener("resize", layout);
    window.addEventListener("scroll", layout, true);
    return () => {
      document.removeEventListener("pointerdown", close);
      window.removeEventListener("resize", layout);
      window.removeEventListener("scroll", layout, true);
    };
  }, [open]);
  useEffect(() => {
    if (!open) return;
    const option = document.getElementById(`${uid}-${active}`);
    const menu = list.current;
    if (option && menu) {
      // Scroll only the list; scrollIntoView can move the entire modal/page.
      if (option.offsetTop < menu.scrollTop) menu.scrollTop = option.offsetTop;
      else if (
        option.offsetTop + option.offsetHeight >
        menu.scrollTop + menu.clientHeight
      )
        menu.scrollTop =
          option.offsetTop + option.offsetHeight - menu.clientHeight;
    }
  }, [open, active, uid]);
  useEffect(() => {
    if (disabled) setOpen(false);
  }, [disabled]);
  const enabled = options
    .map((o, index) => (o.disabled ? -1 : index))
    .filter((index) => index >= 0);
  const selectedIndex = options.findIndex(
    (o) => o.value === selected && !o.disabled,
  );
  const initialIndex = selectedIndex >= 0 ? selectedIndex : (enabled[0] ?? -1);
  const toggle = () => {
    setRect(ref.current?.getBoundingClientRect() ?? null);
    setActive(initialIndex);
    search.current = { text: "", at: 0 };
    setOpen((v) => !v);
  };
  const below = rect ? Math.max(0, window.innerHeight - rect.bottom - 14) : 0;
  const above = rect ? Math.max(0, rect.top - 14) : 0;
  const wantedHeight = Math.min(280, options.length * 40 + 12);
  const opensAbove = below < wantedHeight && above > below;
  const menuWidth = rect
    ? Math.min(Math.max(rect.width, 160), window.innerWidth - 16)
    : 160;
  return (
    <>
      <button
        {...Object.fromEntries(
          Object.entries(rest).filter(
            ([k]) => k.startsWith("aria-") || k === "title",
          ),
        )}
        ref={ref}
        id={id}
        type="button"
        disabled={disabled}
        className={cn("input custom-select", className)}
        aria-label={controlLabel}
        aria-activedescendant={
          open && options[active] ? `${uid}-${active}` : undefined
        }
        role="combobox"
        aria-expanded={open}
        aria-controls={open ? uid : undefined}
        aria-haspopup="listbox"
        aria-required={required}
        onClick={toggle}
        onKeyDown={(e) => {
          if (e.key === "Tab") {
            setOpen(false);
            return;
          }
          if (e.key === "Escape" && open) {
            e.preventDefault();
            e.stopPropagation();
            setOpen(false);
            return;
          }
          if (["ArrowDown", "ArrowUp", "Home", "End"].includes(e.key)) {
            e.preventDefault();
            if (!open) {
              setRect(ref.current?.getBoundingClientRect() ?? null);
              setOpen(true);
              setActive(
                e.key === "End"
                  ? (enabled.at(-1) ?? -1)
                  : e.key === "Home"
                    ? (enabled[0] ?? -1)
                    : initialIndex,
              );
            } else {
              const index = enabled.indexOf(active);
              setActive(
                e.key === "Home"
                  ? (enabled[0] ?? -1)
                  : e.key === "End"
                    ? (enabled.at(-1) ?? -1)
                    : (enabled[
                        Math.max(
                          0,
                          Math.min(
                            enabled.length - 1,
                            index + (e.key === "ArrowDown" ? 1 : -1),
                          ),
                        )
                      ] ?? -1),
              );
            }
            return;
          }
          if (open && (e.key === "Enter" || e.key === " ")) {
            e.preventDefault();
            if (options[active]) choose(options[active]);
            return;
          }
          if (
            e.key.length === 1 &&
            e.key !== " " &&
            !e.ctrlKey &&
            !e.metaKey &&
            !e.altKey
          ) {
            e.preventDefault();
            const now = Date.now();
            const text =
              now - search.current.at < 700
                ? search.current.text + e.key
                : e.key;
            search.current = { text, at: now };
            const query = text.toLocaleLowerCase();
            const start = open ? active : initialIndex;
            const order = [
              ...enabled.filter((index) => index > start),
              ...enabled.filter((index) => index <= start),
            ];
            const match = order.find((index) => {
              const label = options[index].label;
              const text =
                typeof label === "string" || typeof label === "number"
                  ? String(label)
                  : document.getElementById(`${uid}-${index}`)?.textContent ||
                    "";
              return text.trim().toLocaleLowerCase().startsWith(query);
            });
            if (match !== undefined) {
              setRect(ref.current?.getBoundingClientRect() ?? null);
              setOpen(true);
              setActive(match);
            }
          }
        }}
      >
        <span className="truncate">
          {options.find((o) => o.value === selected)?.label ?? t("Select")}
        </span>
        <ChevronDown size={14} className={open ? "rotate-180" : ""} />
      </button>
      {name && <input type="hidden" name={name} value={selected} />}{" "}
      {open &&
        rect &&
        createPortal(
          <div
            ref={list}
            id={uid}
            className="select-menu"
            role="listbox"
            aria-label={controlLabel}
            style={{
              position: "fixed",
              left: Math.max(
                8,
                Math.min(
                  document.documentElement.dir === "rtl"
                    ? rect.right - menuWidth
                    : rect.left,
                  window.innerWidth - menuWidth - 8,
                ),
              ),
              top: opensAbove ? undefined : rect.bottom + 6,
              bottom: opensAbove
                ? window.innerHeight - rect.top + 6
                : undefined,
              width: menuWidth,
              maxHeight: Math.min(280, opensAbove ? above : below),
            }}
          >
            {options.map((o, i) => (
              <button
                type="button"
                key={`${o.value}-${i}`}
                data-value={o.value}
                id={`${uid}-${i}`}
                role="option"
                tabIndex={-1}
                onMouseDown={(e) => e.preventDefault()}
                aria-selected={o.value === selected}
                disabled={o.disabled}
                className={`select-option ${i === active ? "highlighted" : ""}`}
                onMouseEnter={() => {
                  if (!o.disabled) setActive(i);
                }}
                onClick={() => choose(o)}
              >
                <span>{o.label}</span>
                {o.value === selected && <Check size={14} />}
              </button>
            ))}
          </div>,
          document.body,
        )}
    </>
  );
}
export function Input(props: InputHTMLAttributes<HTMLInputElement>) {
  const ref = useRef<HTMLInputElement>(null);
  const label = useControlLabel(ref, props["aria-label"]);
  const { type, className, ...rest } = props;
  if (type === "datetime-local" || type === "date")
    return <DateInput {...props} />;
  return (
    <input
      type={type}
      {...rest}
      className={cn(
        !["checkbox", "radio", "hidden", "file", "range", "color"].includes(
          type || "text",
        ) && "input",
        className,
      )}
      ref={ref}
      aria-label={label}
    />
  );
}
function DateInput({
  value,
  onChange,
  type,
  className = "",
  disabled,
  required,
  id,
  name,
  ...rest
}: InputHTMLAttributes<HTMLInputElement>) {
  const trigger = useRef<HTMLButtonElement>(null),
    label = useControlLabel(trigger, rest["aria-label"]);
  const date = String(value ?? ""),
    [open, setOpen] = useState(false),
    [month, setMonth] = useState(() => date.slice(0, 10) || civilToday()),
    [focusDay, setFocusDay] = useState(date.slice(0, 10) || civilToday()),
    root = useRef<HTMLDivElement>(null);
  const calendar = inputCalendar(datePreferences().calendar, localeName());
  const grid = calendarMonth(month, calendar, localeName());
  useEffect(() => {
    if (open)
      root.current
        ?.querySelector<HTMLButtonElement>(`[data-date="${focusDay}"]`)
        ?.focus();
  }, [open, focusDay, month]);
  const change = (s: string) =>
    onChange?.({
      target: { value: s, name },
      currentTarget: { value: s, name },
    } as ChangeEvent<HTMLInputElement>);
  useEffect(() => {
    if (!open) return;
    const close = (e: MouseEvent) => {
      if (
        !root.current?.contains(e.target as Node) &&
        !(e.target as Element).closest(".select-menu")
      )
        setOpen(false);
    };
    document.addEventListener("mousedown", close);
    return () => document.removeEventListener("mousedown", close);
  }, [open]);
  const day = (s: string) => {
    change(
      type === "datetime-local" ? s + "T" + (date.split("T")[1] || "00:00") : s,
    );
    if (type === "date") {
      setOpen(false);
      trigger.current?.focus();
    }
  };
  const parsed = new Date(type === "date" ? date + "T12:00:00" : date);
  const display =
    date && Number.isFinite(parsed.getTime())
      ? new Intl.DateTimeFormat(localeName(), {
          calendar,
          year: "numeric",
          month: "long",
          day: "numeric",
          ...(type === "datetime-local"
            ? { hour: "2-digit", minute: "2-digit" }
            : {}),
        }).format(parsed)
      : t("Select date");
  const move = (next: string) => {
    setMonth(next);
    setFocusDay(next);
  };
  return (
    <div className="date-control" ref={root}>
      <button
        {...Object.fromEntries(
          Object.entries(rest).filter(([k]) => k.startsWith("aria-")),
        )}
        id={id}
        type="button"
        disabled={disabled}
        aria-expanded={open}
        aria-required={required}
        ref={trigger}
        aria-label={label}
        className={cn("input date-trigger", className)}
        onClick={() => {
          if (!open) {
            const selected = date.slice(0, 10) || civilToday();
            setMonth(selected);
            setFocusDay(selected);
          }
          setOpen((v) => !v);
        }}
        onKeyDown={(e) => {
          if (e.key === "Escape") setOpen(false);
        }}
      >
        <span>{display}</span>
        <CalendarDays size={15} />
      </button>
      {name && <input type="hidden" name={name} value={date} />}{" "}
      {open && (
        <div
          className="calendar-panel"
          role="dialog"
          aria-label={t("Choose date")}
          onKeyDown={(e) => {
            if (e.key === "Escape") {
              e.preventDefault();
              e.stopPropagation();
              setOpen(false);
              trigger.current?.focus();
            }
          }}
        >
          <div className="calendar-heading">
            <button
              type="button"
              aria-label={t("Previous month")}
              onClick={() => move(grid.previous)}
            >
              <ChevronLeft size={16} />
            </button>
            <span>{grid.title}</span>
            <button
              type="button"
              aria-label={t("Next month")}
              onClick={() => move(grid.next)}
            >
              <ChevronRight size={16} />
            </button>
          </div>
          <div
            className="calendar-grid"
            aria-label={grid.title}
            onKeyDown={(e) => {
              const shifts: Record<string, number> = {
                ArrowLeft: document.documentElement.dir === "rtl" ? 1 : -1,
                ArrowRight: document.documentElement.dir === "rtl" ? -1 : 1,
                ArrowUp: -7,
                ArrowDown: 7,
              };
              if (e.key in shifts) {
                e.preventDefault();
                move(addCivilDays(focusDay, shifts[e.key]));
              } else if (e.key === "PageUp" || e.key === "PageDown") {
                e.preventDefault();
                move(e.key === "PageUp" ? grid.previous : grid.next);
              } else if (e.key === "Escape") {
                e.preventDefault();
                e.stopPropagation();
                setOpen(false);
                trigger.current?.focus();
              }
            }}
          >
            {grid.weekdays.map((d, i) => (
              <span key={i}>{d}</span>
            ))}
            {Array.from({ length: grid.offset }, (_, i) => (
              <span key={`empty${i}`} />
            ))}
            {grid.dates.map((value, i) => (
              <button
                type="button"
                key={value}
                data-date={value}
                aria-label={grid.fullLabels[i]}
                aria-pressed={date.startsWith(value)}
                tabIndex={focusDay === value ? 0 : -1}
                className={date.startsWith(value) ? "selected" : ""}
                onFocus={() => setFocusDay(value)}
                onClick={() => day(value)}
              >
                {grid.labels[i]}
              </button>
            ))}
          </div>
          {type === "datetime-local" && (
            <div
              className="calendar-time"
              aria-label={
                t("Browser timezone") +
                ": " +
                Intl.DateTimeFormat().resolvedOptions().timeZone
              }
            >
              <Select
                aria-label={t("Hour", "ساعت")}
                value={date.split("T")[1]?.slice(0, 2) ?? "00"}
                onChange={(e) =>
                  change(
                    (date.split("T")[0] || grid.first) +
                      "T" +
                      e.target.value +
                      ":" +
                      (date.split("T")[1]?.slice(3, 5) || "00"),
                  )
                }
              >
                {Array.from({ length: 24 }, (_, i) => (
                  <option key={i} value={String(i).padStart(2, "0")}>
                    {String(i).padStart(2, "0")}
                  </option>
                ))}
              </Select>
              <span>:</span>
              <Select
                aria-label={t("Minute", "دقیقه")}
                value={date.split("T")[1]?.slice(3, 5) ?? "00"}
                onChange={(e) =>
                  change(
                    (date.split("T")[0] || grid.first) +
                      "T" +
                      (date.split("T")[1]?.slice(0, 2) || "00") +
                      ":" +
                      e.target.value,
                  )
                }
              >
                {Array.from({ length: 60 }, (_, i) => (
                  <option key={i} value={String(i).padStart(2, "0")}>
                    {String(i).padStart(2, "0")}
                  </option>
                ))}
              </Select>
            </div>
          )}
          <div className="calendar-actions">
            {!required && (
              <button
                type="button"
                onClick={() => {
                  change("");
                  setOpen(false);
                  trigger.current?.focus();
                }}
              >
                {t("Clear")}
              </button>
            )}
            <button
              type="button"
              onClick={() => {
                setOpen(false);
                trigger.current?.focus();
              }}
            >
              {t("Done")}
              <Check size={13} />
            </button>
          </div>
        </div>
      )}
    </div>
  );
}

export function Textarea(props: TextareaHTMLAttributes<HTMLTextAreaElement>) {
  const ref = useRef<HTMLTextAreaElement>(null),
    label = useControlLabel(ref, props["aria-label"]);
  return (
    <textarea
      {...props}
      className={cn("input", props.className)}
      ref={ref}
      aria-label={label}
    />
  );
}
