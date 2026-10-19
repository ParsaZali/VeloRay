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
    uid = useId();
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
  useEffect(() => {
    if (!open) return;
    const close = (e: MouseEvent) => {
      if (
        !ref.current?.contains(e.target as Node) &&
        !list.current?.contains(e.target as Node)
      )
        setOpen(false);
    };
    const layout = () => setRect(ref.current?.getBoundingClientRect() ?? null);
    document.addEventListener("mousedown", close);
    window.addEventListener("resize", layout);
    window.addEventListener("scroll", layout, true);
    return () => {
      document.removeEventListener("mousedown", close);
      window.removeEventListener("resize", layout);
      window.removeEventListener("scroll", layout, true);
    };
  }, [open]);
  useEffect(() => {
    if (open)
      document
        .getElementById(`${uid}-${active}`)
        ?.scrollIntoView({ block: "nearest" });
  }, [open, active, uid]);
  const toggle = () => {
    setRect(ref.current?.getBoundingClientRect() ?? null);
    setActive(
      Math.max(
        0,
        options.findIndex((o) => o.value === selected),
      ),
    );
    setOpen((v) => !v);
  };
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
        className={`custom-select ${className}`}
        aria-label={controlLabel}
        aria-activedescendant={open ? `${uid}-${active}` : undefined}
        role="combobox"
        aria-expanded={open}
        aria-controls={uid}
        aria-haspopup="listbox"
        aria-required={required}
        onClick={toggle}
        onKeyDown={(e) => {
          if (e.key === "Tab") setOpen(false);
          if (e.key === "Escape") {
            if (open) e.stopPropagation();
            setOpen(false);
            return;
          }
          if (["ArrowDown", "ArrowUp", "Home", "End"].includes(e.key)) {
            e.preventDefault();
            if (!open) toggle();
            setActive((a) =>
              e.key === "Home"
                ? 0
                : e.key === "End"
                  ? options.length - 1
                  : Math.max(
                      0,
                      Math.min(
                        options.length - 1,
                        a + (e.key === "ArrowDown" ? 1 : -1),
                      ),
                    ),
            );
          }
          if (open && (e.key === "Enter" || e.key === " ")) {
            e.preventDefault();
            if (options[active]) choose(options[active]);
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
            style={{
              position: "fixed",
              left: Math.max(
                8,
                Math.min(
                  rect.left,
                  window.innerWidth - Math.max(rect.width, 160) - 8,
                ),
              ),
              top:
                rect.bottom + Math.min(280, options.length * 38) >
                window.innerHeight
                  ? undefined
                  : rect.bottom + 6,
              bottom:
                rect.bottom + Math.min(280, options.length * 38) >
                window.innerHeight
                  ? window.innerHeight - rect.top + 6
                  : undefined,
              width: Math.min(
                Math.max(rect.width, 160),
                window.innerWidth - 16,
              ),
            }}
          >
            {options.map((o, i) => (
              <button
                type="button"
                key={`${o.value}-${i}`}
                data-value={o.value}
                id={`${uid}-${i}`}
                role="option"
                aria-selected={o.value === selected}
                disabled={o.disabled}
                className={`select-option ${i === active ? "highlighted" : ""}`}
                onMouseEnter={() => setActive(i)}
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
  const { type, ...rest } = props;
  if (type === "datetime-local" || type === "date")
    return <DateInput {...props} />;
  return <input type={type} {...rest} ref={ref} aria-label={label} />;
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
    if (type === "date") setOpen(false);
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
        className={`date-trigger ${className}`}
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
                aria-label="Hour"
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
                aria-label="Minute"
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
            <button
              type="button"
              onClick={() => {
                change("");
                setOpen(false);
              }}
            >
              {t("Clear")}
            </button>
            <button type="button" onClick={() => setOpen(false)}>
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
  return <textarea {...props} ref={ref} aria-label={label} />;
}
