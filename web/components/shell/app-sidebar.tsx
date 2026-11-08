"use client";

import { Suspense, use, useCallback, useEffect, useRef, useState } from "react";
import Link from "next/link";
import { usePathname } from "next/navigation";
import { Mark } from "@/components/brand/mark";
import { HonestyChip } from "@/components/wasl/honesty-chip";
import { SIDEBAR_COOKIE } from "./sidebar-cookie";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { cx } from "@/lib/cx";

type NavItem = {
  href: string;
  label: string;
  icon: React.ReactNode;
  countKey?: "today" | "review" | "referrals";
  gapBefore?: boolean;
  /** Shown as text next to the label, so it never relies on colour. */
  badge?: string;
};

const icon = (children: React.ReactNode) => (
  <svg
    aria-hidden
    width="16"
    height="16"
    viewBox="0 0 16 16"
    fill="none"
    stroke="currentColor"
    strokeWidth="1.5"
    strokeLinecap="round"
    strokeLinejoin="round"
  >
    {children}
  </svg>
);

const NAV: NavItem[] = [
  {
    href: "/app",
    label: "Programme",
    icon: icon(<path d="M3 13.5V8.5M8 13.5V2.5M13 13.5V6" />),
  },
  {
    href: "/app/today",
    label: "Today",
    countKey: "today",
    icon: icon(<path d="M2.5 4h11M2.5 8h11M2.5 12h7" />),
  },
  {
    href: "/app/review",
    label: "Review",
    countKey: "review",
    icon: icon(
      <>
        <rect x="2.5" y="2.5" width="11" height="11" rx="2" />
        <path d="M5.5 8.2l1.8 1.8 3.2-4" />
      </>,
    ),
  },
  {
    href: "/app/patients",
    label: "Patients",
    icon: icon(
      <>
        <circle cx="8" cy="5.5" r="2.5" />
        <path d="M3 13.5c.8-2.4 2.7-3.5 5-3.5s4.2 1.1 5 3.5" />
      </>,
    ),
  },
  {
    href: "/app/referrals",
    label: "Referrals",
    countKey: "referrals",
    icon: icon(
      <>
        <path d="M2.5 9.5h3l1 2h3l1-2h3" />
        <path d="M4 3.5h8l1.5 6v3h-11v-3z" />
      </>,
    ),
  },
  {
    href: "/app/pipeline",
    label: "Pipeline",
    gapBefore: true,
    icon: icon(
      <>
        <circle cx="3" cy="8" r="1.5" />
        <circle cx="8" cy="8" r="1.5" />
        <circle cx="13" cy="8" r="1.5" />
        <path d="M4.5 8h2M9.5 8h2" />
      </>,
    ),
  },
  {
    href: "/app/results",
    label: "Results",
    icon: icon(
      <>
        <path d="M2.5 13.5h11" />
        <path d="M3 10.5l3.5-3.5 2.5 2.5 4.5-5" />
      </>,
    ),
  },
  {
    href: "/app/agent-review",
    label: "Agent review",
    badge: "Experimental",
    icon: icon(
      <>
        <circle cx="8" cy="8" r="5.5" />
        <path d="M8 5v3.2l2 1.3" />
      </>,
    ),
  },
];

/** Who is signed in depends on whose screen it is (design-brief-v2, canonical facts). */
const USERS: {
  prefix: string;
  initials: string;
  name: string;
  role: string;
}[] = [
  {
    prefix: "/app/today",
    initials: "HA",
    name: "Hessa Al Ali",
    role: "Nurse coordinator",
  },
  {
    prefix: "/app/patients",
    initials: "HA",
    name: "Hessa Al Ali",
    role: "Nurse coordinator",
  },
  {
    prefix: "/app/review",
    initials: "OK",
    name: "Dr Omar Al Kaabi",
    role: "Medical director",
  },
  {
    prefix: "/app/referrals",
    initials: "LH",
    name: "Dr Layla Haddad",
    role: "Unit lead, SKMC endoscopy",
  },
  {
    prefix: "/app",
    initials: "ST",
    name: "Dr Salama Al Tamimi",
    role: "Programme director",
  },
];

export type NavCounts = Partial<
  Record<NonNullable<NavItem["countKey"]>, number>
>;

function isActive(pathname: string, href: string) {
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
        "flex min-h-11 items-center gap-2.5 rounded-control text-ink no-underline hover:text-ink",
        collapsed ? "justify-center" : "px-2.5",
      )}
    >
      <Mark size={24} />
      <span
        className={cx(
          "flex flex-col whitespace-nowrap",
          collapsed && "sr-only",
        )}
      >
        <span className="text-section font-semibold leading-5">Wasl</span>
        <span className="text-small text-muted">ADPHC · screening follow-up</span>
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
        <span
          aria-hidden
          className="absolute right-2 top-2 size-2 rounded-full bg-hue"
        />
        <span className="sr-only">{n} open</span>
      </>
    );
  }
  return <span className="ml-auto text-small text-muted">{n}</span>;
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
  const link = (
    <Link
      href={item.href}
      aria-current={active ? "page" : undefined}
      className={cx(
        "relative flex h-11 items-center gap-2.5 rounded-control text-ink no-underline hover:bg-active hover:text-ink md:h-10",
        collapsed ? "justify-center" : "px-2.5",
        active ? "bg-active [&_svg]:text-ink-soft" : "[&_svg]:text-muted",
        item.gapBefore && "mt-6",
      )}
    >
      {item.icon}
      <span className={cx("whitespace-nowrap", collapsed && "sr-only")}>{item.label}</span>
      {item.badge && (
        <span
          className={cx(
            "ml-auto whitespace-nowrap rounded-full border border-control px-1.5 -mr-1 text-[11px] font-medium leading-4 text-muted",
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

function Nav({
  pathname,
  counts,
  collapsed = false,
}: {
  pathname: string;
  counts: Promise<NavCounts>;
  collapsed?: boolean;
}) {
  return (
    <nav aria-label="Main" className="flex flex-col gap-0.5">
      {NAV.map((item) => (
        <NavLink
          key={item.href}
          item={item}
          active={isActive(pathname, item.href)}
          counts={counts}
          collapsed={collapsed}
        />
      ))}
    </nav>
  );
}

function UserBlock({
  user,
  collapsed = false,
}: {
  user: (typeof USERS)[number];
  collapsed?: boolean;
}) {
  return (
    <div
      className={cx("flex items-center gap-2.5", collapsed && "justify-center")}
    >
      <span
        aria-hidden
        className="flex size-7 flex-none items-center justify-center rounded-full bg-active text-small font-medium text-ink-soft"
      >
        {user.initials}
      </span>
      <span
        className={cx(
          "flex flex-col whitespace-nowrap",
          collapsed && "sr-only",
        )}
      >
        <span className="text-body font-medium leading-[18px]">
          {user.name}
        </span>
        <span className="text-small text-muted">{user.role}</span>
      </span>
    </div>
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
  defaultCollapsed = false,
}: {
  counts: Promise<NavCounts>;
  defaultCollapsed?: boolean;
}) {
  const pathname = usePathname();
  const user =
    USERS.find((u) => pathname.startsWith(u.prefix)) ?? USERS.at(-1)!;

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
          "sticky top-0 hidden h-dvh flex-none flex-col overflow-x-hidden pb-5 pt-5 motion-safe:transition-[width] motion-safe:duration-200 md:flex",
          collapsed ? "w-16 px-2" : "w-sidebar px-3",
        )}
      >
        <Brand collapsed={collapsed} />
        <div className="mt-5">
          <Nav pathname={pathname} counts={counts} collapsed={collapsed} />
        </div>
        <div
          className={cx(
            "mt-auto flex flex-col gap-3",
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
                  "flex h-10 items-center rounded-control text-muted hover:bg-active hover:text-ink",
                  collapsed ? "w-10 justify-center" : "w-full gap-2.5 px-2.5",
                )}
              >
                {icon(
                  <>
                    <rect x="2.5" y="3" width="11" height="10" rx="2" />
                    <path d="M6 3v10" />
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
            <UserBlock user={user} collapsed={collapsed} />
            <HonestyChip kind="simulated" compact={collapsed}>
              <span className={cx(collapsed && "sr-only")}>{SYNTHETIC}</span>
            </HonestyChip>
          </div>
        </div>
      </aside>

      <header className="sticky top-0 z-30 flex min-h-14 items-center justify-between border-b border-line bg-ground px-4 md:hidden">
        {menuOpen && (
          <div
            aria-hidden
            onClick={() => setOpenFor(null)}
            className="fixed inset-x-0 bottom-0 top-14 -z-10 bg-ink/20"
          />
        )}
        <Brand />
        <button
          ref={triggerRef}
          type="button"
          aria-expanded={menuOpen}
          aria-controls="mobile-menu"
          onClick={() => setOpenFor(menuOpen ? null : pathname)}
          className="flex h-11 min-w-11 items-center justify-center rounded-control px-3 text-ink hover:bg-active"
        >
          {menuOpen ? "Close" : "Menu"}
          <span className="sr-only"> navigation</span>
        </button>
        <div
          id="mobile-menu"
          ref={panelRef}
          hidden={!menuOpen}
          className="absolute inset-x-0 top-full"
        >
          <div className="flex max-h-[calc(100dvh-3.5rem)] flex-col gap-3 overflow-y-auto border-b border-line bg-surface p-3">
            <Nav pathname={pathname} counts={counts} />
            <div className="flex flex-col gap-3 px-2.5 pb-1">
              <UserBlock user={user} />
              <HonestyChip kind="simulated">{SYNTHETIC}</HonestyChip>
            </div>
          </div>
        </div>
      </header>
    </>
  );
}
