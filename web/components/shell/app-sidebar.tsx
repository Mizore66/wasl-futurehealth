"use client";

import { Suspense, use, useCallback, useEffect, useRef, useState } from "react";
import Link from "next/link";
import { usePathname, useSearchParams } from "next/navigation";
import { Menu, X } from "lucide-react";
import { Mark } from "@/components/brand/mark";
import { HonestyChip } from "@/components/wasl/honesty-chip";
import { SIDEBAR_COOKIE } from "./sidebar-cookie";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { cx } from "@/lib/cx";
import type { SessionPersona } from "@/lib/auth/session-persona";
import { PERSONA_PREFIX, shellPersona, usesReviewSession, type ShellPersona } from "./persona";
import { LAST_CASE_KEY } from "./remember-case";
import { isTodayQueue, TODAY_QUEUES, useLiveTodayCounts, type TodayCounts, type TodayQueue } from "./today-counts";
import { TopActions } from "./top-bar";

type NavItem = {
  href: string;
  label: string;
  icon: React.ReactNode;
  countKey?: "review" | "referrals";
  /** Shown as text next to the label, so it never relies on colour. */
  badge?: string;
  /** Active on these paths too (the audit trail lives under a case). */
  activeOn?: RegExp;
  /** Never active on these paths (a case's audit is the Audit trail, not Patients). */
  inactiveOn?: RegExp;
};

const icon = (children: React.ReactNode) => (
  <svg
    aria-hidden
    width="16"
    height="16"
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    strokeWidth="1.6"
    strokeLinecap="round"
    strokeLinejoin="round"
  >
    {children}
  </svg>
);

const AUDIT_PATH = /^\/app\/patients\/[^/]+\/audit(\/|$)/;

const TODAY: NavItem = {
  href: "/app/today",
  label: "Today",
  icon: icon(<path d="M3 5h18v16H3z M7 3v4m10-4v4M3 10h18" />),
};

/** Grouped as the approved design: the coordinator's workspace, then oversight, then the evidence behind it. */
const GROUPS: { title: string; items: NavItem[] }[] = [
  {
    title: "Workspace",
    items: [
      TODAY,
      {
        href: "/app/patients",
        label: "Patients",
        inactiveOn: AUDIT_PATH,
        icon: icon(<path d="M13 20v-2a4 4 0 0 0-4-4H7a4 4 0 0 0-4 4v2 M8 10a3 3 0 1 0 0-6 3 3 0 0 0 0 6 M16 4.5a3 3 0 0 1 0 5.5 M21 20v-2a4 4 0 0 0-3-3.9" />),
      },
      {
        href: "/app/referrals",
        label: "Referrals",
        countKey: "referrals",
        icon: icon(<path d="M3 7h13m-4-4 5 4-5 4m9 6H8m4-4-5 4 5 4" />),
      },
      {
        href: "/app/recovery",
        label: "Recovery",
        icon: icon(<path d="M5 9a8 8 0 1 1-1 6M4 3v6h6" />),
      },
    ],
  },
  {
    title: "Oversight",
    items: [
      {
        href: "/app",
        label: "Programme",
        icon: icon(<path d="M3 20h18M5 17v-5h3v5m3 0V7h3v10m3 0V3h3v14" />),
      },
      {
        href: "/app/review",
        label: "Review desk",
        countKey: "review",
        icon: icon(<path d="M4 4h12v16H4z M8 9h5m-5 4h3m5-4 4 1v11H8" />),
      },
      {
        href: "/app/patients",
        label: "Audit trail",
        activeOn: AUDIT_PATH,
        icon: icon(<path d="m12 3 8 3v6c0 5-8 9-8 9s-8-4-8-9V6z m-4 8 3 3 5-6" />),
      },
    ],
  },
  {
    title: "Evidence",
    items: [
      {
        href: "/app/pipeline",
        label: "Pipeline",
        icon: icon(<path d="M3 12h4m4 0h2m4 0h4 M7 12a2 2 0 1 0 4 0 2 2 0 0 0-4 0 M13 12a2 2 0 1 0 4 0 2 2 0 0 0-4 0" />),
      },
      {
        href: "/app/results",
        label: "Results",
        icon: icon(<path d="M3 20h18 M4 15l5-5 4 4 7-8" />),
      },
      {
        href: "/app/agent-review",
        label: "Agent review",
        badge: "Experimental",
        icon: icon(<><circle cx="12" cy="12" r="9" /><path d="M12 7v5l3 2" /></>),
      },
    ],
  },
];

export type NavCounts = Partial<Record<NonNullable<NavItem["countKey"]>, number>>;

function isActive(pathname: string, item: NavItem) {
  if (item.activeOn) return item.activeOn.test(pathname);
  if (item.inactiveOn?.test(pathname)) return false;
  const href = item.href;
  return href === "/app"
    ? pathname === "/app"
    : pathname === href || pathname.startsWith(`${href}/`);
}

function Brand({ collapsed = false }: { collapsed?: boolean }) {
  return (
    <Link
      href="/"
      prefetch={false}
      title="Back to the Wasl site"
      className={cx(
        "flex min-h-11 min-w-0 items-center gap-2 rounded-control text-limestone-50 no-underline hover:text-limestone-50",
        collapsed ? "justify-center" : "px-2",
      )}
    >
      <Mark size={22} tone="dark" />
      <span className={cx("flex min-w-0 items-center gap-2", collapsed && "sr-only")}>
        <span className="staff-display text-[28px] leading-[1.15] text-limestone-50">Wasl</span>
        <span
          lang="ar"
          dir="rtl"
          className="flex-none border-l border-jade-500 pl-2 text-[21px] leading-[1.5] text-jade-100"
        >
          وصل
        </span>
      </span>
    </Link>
  );
}

function Count({
  counts,
  k,
  collapsed,
}: {
  counts: Promise<NavCounts>;
  k: NonNullable<NavItem["countKey"]>;
  collapsed?: boolean;
}) {
  const n = use(counts)[k];
  if (!n) return null;
  if (collapsed) {
    return (
      <>
        <span aria-hidden className="absolute right-2 top-2 size-2 rounded-full bg-jade-200" />
        <span className="sr-only">{n} open</span>
      </>
    );
  }
  return <span className="ml-auto text-[12px] tabular-nums opacity-80">{n}</span>;
}

function TipCount({
  counts,
  k,
}: {
  counts: Promise<NavCounts>;
  k: NonNullable<NavItem["countKey"]>;
}) {
  const n = use(counts)[k];
  return n ? <> · {n} open</> : null;
}

const linkClass = (active: boolean, collapsed: boolean) =>
  cx(
    "relative flex h-11 items-center gap-3 rounded-[4px] text-[14px] no-underline md:h-10",
    collapsed ? "justify-center" : "px-3",
    active
      ? "bg-limestone-50 font-semibold text-jade-800 hover:bg-limestone-100 hover:text-jade-900"
      : "text-limestone-100 hover:bg-jade-800 hover:text-limestone-50",
  );

/** The sidebar's Audit trail opens the audit of the case opened last in this tab; with none yet, the case list. */
function useAuditHref(fallback: string) {
  const [href, setHref] = useState(fallback);
  useEffect(() => {
    const read = () => {
      try {
        const id = sessionStorage.getItem(LAST_CASE_KEY);
        setHref(id ? `/app/patients/${encodeURIComponent(id)}/audit` : fallback);
      } catch {}
    };
    read();
    window.addEventListener("wasl:last-case", read);
    return () => window.removeEventListener("wasl:last-case", read);
  }, [fallback]);
  return href;
}

function NavLink({
  item,
  active,
  counts,
  collapsed,
}: {
  item: NavItem;
  active: boolean;
  counts: Promise<NavCounts>;
  collapsed: boolean;
}) {
  const auditHref = useAuditHref(item.href);
  const href = item.activeOn ? auditHref : item.href;
  const link = (
    <Link href={href} aria-current={active ? "page" : undefined} className={linkClass(active, collapsed)}>
      {item.icon}
      <span className={cx("whitespace-nowrap", collapsed && "sr-only")}>{item.label}</span>
      {item.badge && (
        <span
          className={cx(
            "ml-auto whitespace-nowrap rounded-full border border-current px-1.5 -mr-1 text-[10px] font-medium leading-4 opacity-80",
            collapsed && "sr-only",
          )}
        >
          {item.badge}
        </span>
      )}
      {item.countKey && (
        <Suspense fallback={null}>
          <Count counts={counts} k={item.countKey} collapsed={collapsed} />
        </Suspense>
      )}
    </Link>
  );
  // Radix opens on hover and on keyboard focus; the open prop pins it shut while the label is already visible.
  return (
    <Tooltip key={String(collapsed)} open={collapsed ? undefined : false}>
      <TooltipTrigger asChild>{link}</TooltipTrigger>
      <TooltipContent side="right" sideOffset={8}>
        {item.label}
        {item.badge && <> · {item.badge}</>}
        {item.countKey && (
          <Suspense fallback={null}>
            <TipCount counts={counts} k={item.countKey} />
          </Suspense>
        )}
      </TooltipContent>
    </Tooltip>
  );
}

function QueueCount({ fetched, q }: { fetched: TodayCounts | null; q: TodayQueue }) {
  const live = useLiveTodayCounts();
  const n = live?.[q] ?? fetched?.[q];
  return n === undefined ? null : <span className="ml-auto text-[12px] tabular-nums">{n}</span>;
}

/**
 * The server's queue counts, read from the browser after the shell paints. Pages that act with a reviewer's own
 * session skip the read, so they never trigger a shared-credential read of every case.
 */
function useFetchedTodayCounts(pathname: string): TodayCounts | null {
  const skip = usesReviewSession(pathname);
  const [counts, setCounts] = useState<TodayCounts | null>(null);
  useEffect(() => {
    if (skip) return;
    const ctl = new AbortController();
    fetch("/app/today-counts", { signal: ctl.signal, cache: "no-store" })
      .then((r) => (r.ok ? r.json() : null))
      .then((c: TodayCounts | null) => {
        if (c) setCounts(c);
      })
      .catch(() => {});
    return () => ctl.abort();
  }, [skip, pathname]);
  return counts;
}

function QueueLinks({ pathname }: { pathname: string }) {
  const params = useSearchParams();
  const fetched = useFetchedTodayCounts(pathname);
  const raw = params.get("queue");
  const current = pathname === "/app/today" ? (isTodayQueue(raw) ? raw : "call") : null;
  /* On Today, a queue link keeps the chosen programme and provider. */
  const scoped = (q: TodayQueue) => {
    const next = new URLSearchParams();
    if (current) for (const k of ["programme", "provider"]) if (params.has(k)) next.set(k, params.get(k)!);
    next.set("queue", q);
    return `/app/today?${next.toString()}`;
  };
  return (
    <ul className="m-0 mb-1 ml-[19px] list-none border-l border-jade-500 p-0">
      {TODAY_QUEUES.map(([q, label]) => {
        const on = current === q;
        return (
          <li key={q} className="relative">
            <span aria-hidden className="absolute left-0 top-1/2 h-px w-2.5 bg-jade-500" />
            <Link
              href={scoped(q)}
              aria-current={on ? "page" : undefined}
              className={cx(
                "ml-2.5 flex h-11 items-center gap-2 rounded-[4px] px-2.5 text-[13px] no-underline md:h-9",
                on ? "bg-jade-900 font-semibold text-limestone-50 hover:text-limestone-50" : "text-jade-100 hover:bg-jade-800 hover:text-limestone-50",
              )}
            >
              <span>{label}</span>
              <QueueCount fetched={fetched} q={q} />
            </Link>
          </li>
        );
      })}
    </ul>
  );
}

/** The desktop aside and the mobile menu each render this nav; they carry different names so the DOM never holds two "Main" landmarks (issue #20). */
function Nav({
  pathname,
  counts,
  collapsed = false,
  label = "Main",
}: {
  pathname: string;
  counts: Promise<NavCounts>;
  collapsed?: boolean;
  label?: "Main" | "Main (mobile)";
}) {
  return (
    <nav aria-label={label} className="flex flex-col">
      {GROUPS.map((group, gi) => (
        <div key={group.title} className={cx(gi > 0 && "mt-5")}>
          <p
            className={cx(
              "m-0 mb-2 px-3 text-[10px] font-semibold uppercase tracking-[1.45px] text-jade-200",
              collapsed && "sr-only",
            )}
          >
            {group.title}
          </p>
          <div className="flex flex-col gap-[3px]">
            {group.items.map((item) => {
              const active = isActive(pathname, item);
              if (item === TODAY) {
                return (
                  <div key={item.label}>
                    <NavLink item={item} active={active && collapsed} counts={counts} collapsed={collapsed} />
                    {!collapsed && <QueueLinks pathname={pathname} />}
                  </div>
                );
              }
              return <NavLink key={item.label} item={item} active={active} counts={counts} collapsed={collapsed} />;
            })}
          </div>
        </div>
      ))}
    </nav>
  );
}

function UserBlock({
  user,
  collapsed = false,
}: {
  user: ShellPersona;
  collapsed?: boolean;
}) {
  return (
    <div
      className={cx("flex items-center gap-2.5", collapsed && "justify-center")}
      data-testid="shell-persona"
      data-persona-source={user.source}
    >
      <span
        aria-hidden
        className="flex size-8 flex-none items-center justify-center rounded-full bg-jade-200 text-[12px] font-bold text-jade-900"
      >
        {user.initials}
      </span>
      <span
        className={cx(
          "flex min-w-0 flex-col",
          collapsed && "sr-only",
        )}
      >
        <span className="text-small text-jade-200">{PERSONA_PREFIX[user.source]}</span>
        <span className="truncate text-body text-limestone-50 font-medium leading-[18px]">
          {user.name}
        </span>
        <span className="text-small text-jade-200">{user.role}</span>
      </span>
    </div>
  );
}

/** The session resolves on the server after the shell paints; until then the screen's "Viewing as" persona shows. */
function SessionUserBlock({
  pathname,
  session,
  collapsed,
}: {
  pathname: string;
  session: Promise<SessionPersona | null>;
  collapsed?: boolean;
}) {
  return <UserBlock user={shellPersona(pathname, use(session))} collapsed={collapsed} />;
}

function PersonaSlot({
  pathname,
  session,
  collapsed,
}: {
  pathname: string;
  session?: Promise<SessionPersona | null>;
  collapsed?: boolean;
}) {
  const fallback = <UserBlock user={shellPersona(pathname, null)} collapsed={collapsed} />;
  if (!session) return fallback;
  return (
    <Suspense fallback={fallback}>
      <SessionUserBlock pathname={pathname} session={session} collapsed={collapsed} />
    </Suspense>
  );
}

const SYNTHETIC = "Synthetic data · demo";

function typingTarget(t: EventTarget | null) {
  return (
    t instanceof HTMLElement &&
    (t.isContentEditable || ["INPUT", "TEXTAREA", "SELECT"].includes(t.tagName))
  );
}

/** Queue counts arrive as a promise so the shell renders before the API answers. */
export function AppSidebar({
  counts,
  session,
  defaultCollapsed = false,
}: {
  counts: Promise<NavCounts>;
  /** The signed-in review session's actor, if any (lib/api/reviewer-session.ts). */
  session?: Promise<SessionPersona | null>;
  defaultCollapsed?: boolean;
}) {
  const pathname = usePathname();

  const [collapsed, setCollapsed] = useState(defaultCollapsed);
  const toggle = useCallback(() => {
    setCollapsed((c) => !c);
  }, []);

  // The layout reads this cookie on the server, so the next load renders at the right width.
  const first = useRef(true);
  useEffect(() => {
    if (first.current) {
      first.current = false;
      return;
    }
    document.cookie = `${SIDEBAR_COOKIE}=${collapsed ? "collapsed" : "expanded"}; path=/; max-age=31536000; samesite=lax`;
  }, [collapsed]);

  // "[" toggles the rail, except while typing or with a modifier held.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (
        e.key !== "[" ||
        e.ctrlKey ||
        e.metaKey ||
        e.altKey ||
        typingTarget(e.target)
      )
        return;
      toggle();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [toggle]);

  // Mobile menu: open for the path it was opened on, so a route change closes it without an effect.
  const [openFor, setOpenFor] = useState<string | null>(null);
  const menuOpen = openFor === pathname;
  const triggerRef = useRef<HTMLButtonElement>(null);
  const panelRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!menuOpen) return;
    panelRef.current?.querySelector<HTMLElement>("a")?.focus();
    const onKey = (e: KeyboardEvent) => {
      if (e.key !== "Escape") return;
      setOpenFor(null);
      triggerRef.current?.focus();
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [menuOpen]);

  const toggleLabel = collapsed ? "Expand sidebar" : "Collapse sidebar";

  return (
    <>
      <aside
        id="app-sidebar"
        aria-label="Wasl"
        data-collapsed={collapsed}
        className={cx(
          "staff-rail sticky top-0 hidden h-dvh flex-none flex-col overflow-y-auto overflow-x-hidden border-r border-jade-800 bg-jade-700 pb-5 pt-7 text-limestone-50 motion-safe:transition-[width] motion-safe:duration-200 md:flex",
          collapsed ? "w-16 px-2" : "w-[228px] px-3",
        )}
      >
        <Brand collapsed={collapsed} />
        <div className="mt-7">
          <Nav pathname={pathname} counts={counts} collapsed={collapsed} />
        </div>
        <div
          className={cx(
            "mt-auto flex flex-col gap-3 border-t border-jade-500 pt-4",
            collapsed && "items-center",
          )}
        >
          <Tooltip>
            <TooltipTrigger asChild>
              <button
                type="button"
                onClick={toggle}
                aria-expanded={!collapsed}
                aria-controls="app-sidebar"
                aria-label={toggleLabel}
                aria-keyshortcuts="["
                className={cx(
                  "flex h-10 items-center rounded-control text-jade-100 hover:bg-jade-800 hover:text-limestone-50",
                  collapsed ? "w-10 justify-center" : "w-full gap-2.5 px-2.5",
                )}
              >
                {icon(
                  <>
                    <rect x="3.5" y="4.5" width="17" height="15" rx="2" />
                    <path d="M9 4.5v15" />
                  </>,
                )}
                {!collapsed && <span>Collapse</span>}
              </button>
            </TooltipTrigger>
            <TooltipContent side="right" sideOffset={8}>
              {toggleLabel} ([)
            </TooltipContent>
          </Tooltip>
          <div
            className={cx(
              "flex flex-col gap-3",
              collapsed ? "items-center" : "px-2.5",
            )}
          >
            <PersonaSlot pathname={pathname} session={session} collapsed={collapsed} />
            <HonestyChip kind="simulated" compact={collapsed}>
              <span className={cx(collapsed && "sr-only")}>{SYNTHETIC}</span>
            </HonestyChip>
          </div>
        </div>
      </aside>

      <header className="sticky top-0 z-30 flex min-h-14 items-center justify-between gap-2 border-b border-line bg-surface px-3 md:hidden">
        {menuOpen && (
          <div
            aria-hidden
            onClick={() => setOpenFor(null)}
            className="fixed inset-x-0 bottom-0 top-14 -z-10 bg-ink/20"
          />
        )}
        <div className="flex min-w-0 items-center gap-1.5">
          <button
            ref={triggerRef}
            type="button"
            aria-expanded={menuOpen}
            aria-controls="mobile-menu"
            aria-label={menuOpen ? "Close menu" : "Menu"}
            onClick={() => setOpenFor(menuOpen ? null : pathname)}
            className="flex size-11 items-center justify-center rounded-control border border-limestone-400 text-ink hover:bg-jade-50"
          >
            {menuOpen ? <X aria-hidden className="size-5" strokeWidth={1.6} /> : <Menu aria-hidden className="size-5" strokeWidth={1.6} />}
          </button>
          <Link href="/app" className="flex items-center gap-1.5 text-jade-700 no-underline hover:text-jade-800">
            <span className="staff-display text-[22px] leading-none">Wasl</span>
            <span lang="ar" dir="rtl" className="text-[18px] leading-none max-[375px]:hidden">
              وصل
            </span>
          </Link>
        </div>
        <TopActions />
        <div
          id="mobile-menu"
          ref={panelRef}
          hidden={!menuOpen}
          className="absolute left-2 top-full w-[min(280px,calc(100%-16px))]"
        >
          <div className="flex max-h-[min(75dvh,calc(100dvh-4.5rem))] flex-col gap-3 overflow-y-auto rounded-b-[7px] border border-t-0 border-jade-800 bg-jade-700 p-2.5 text-limestone-50 shadow-[0_5px_18px_#20372e33] staff-rail">
            <Nav pathname={pathname} counts={counts} label="Main (mobile)" />
            <div className="flex flex-col gap-3 px-2.5 pb-1">
              <PersonaSlot pathname={pathname} session={session} />
              <HonestyChip kind="simulated">{SYNTHETIC}</HonestyChip>
            </div>
          </div>
        </div>
      </header>
    </>
  );
}
