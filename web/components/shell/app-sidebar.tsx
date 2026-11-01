"use client";

import { Suspense, use } from "react";
import Link from "next/link";
import { usePathname } from "next/navigation";
import { HonestyLabel } from "@/components/wasl/honesty-label";
import { cn } from "@/lib/utils";

type NavItem = { href: string; label: string; icon: React.ReactNode; countKey?: "today" | "review" | "referrals"; gapBefore?: boolean };

const icon = (children: React.ReactNode) => (
  <svg aria-hidden width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
    {children}
  </svg>
);

const NAV: NavItem[] = [
  { href: "/app", label: "Programme", icon: icon(<path d="M3 13.5V8.5M8 13.5V2.5M13 13.5V6" />) },
  { href: "/app/today", label: "Today", countKey: "today", icon: icon(<path d="M2.5 4h11M2.5 8h11M2.5 12h7" />) },
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
];

/** Who is signed in depends on whose screen it is (design-brief-v2, canonical facts). */
const USERS: { prefix: string; initials: string; name: string; role: string }[] = [
  { prefix: "/app/today", initials: "HA", name: "Hessa Al Ali", role: "Nurse coordinator" },
  { prefix: "/app/patients", initials: "HA", name: "Hessa Al Ali", role: "Nurse coordinator" },
  { prefix: "/app/review", initials: "OK", name: "Dr Omar Al Kaabi", role: "Medical director" },
  { prefix: "/app/referrals", initials: "LH", name: "Dr Layla Haddad", role: "Unit lead, SKMC endoscopy" },
  { prefix: "/app", initials: "ST", name: "Dr Salama Al Tamimi", role: "Programme director" },
];

export type NavCounts = Partial<Record<NonNullable<NavItem["countKey"]>, number>>;

function isActive(pathname: string, href: string) {
  return href === "/app" ? pathname === "/app" : pathname === href || pathname.startsWith(`${href}/`);
}

function Brand() {
  return (
    <Link href="/" title="Back to the Wasl site" className="flex items-center gap-2.5 rounded-control px-2.5 text-ink no-underline hover:text-ink">
      <span aria-hidden className="flex size-6 flex-none items-center justify-center rounded-control bg-ink text-[15px] font-semibold leading-none text-white">
        <span lang="ar" dir="rtl">
          و
        </span>
      </span>
      <span className="flex flex-col">
        <span className="text-section font-semibold leading-5">Wasl</span>
        <span className="text-small text-muted">ADPHC · CRC programme</span>
      </span>
    </Link>
  );
}

function Count({ counts, k }: { counts: Promise<NavCounts>; k: NonNullable<NavItem["countKey"]> }) {
  const n = use(counts)[k];
  return n ? <span className="ml-auto text-small text-muted">{n}</span> : null;
}

function Nav({ pathname, counts }: { pathname: string; counts: Promise<NavCounts> }) {
  return (
    <nav aria-label="Main" className="flex flex-col gap-0.5">
      {NAV.map((item) => {
        const active = isActive(pathname, item.href);
        return (
          <Link
            key={item.href}
            href={item.href}
            aria-current={active ? "page" : undefined}
            className={cn(
              "flex h-8 items-center gap-2.5 rounded-control px-2.5 text-ink no-underline hover:bg-active hover:text-ink",
              active ? "bg-active [&_svg]:text-ink-soft" : "[&_svg]:text-muted",
              item.gapBefore && "mt-6",
            )}
          >
            {item.icon}
            <span>{item.label}</span>
            {item.countKey && (
              <Suspense fallback={null}>
                <Count counts={counts} k={item.countKey} />
              </Suspense>
            )}
          </Link>
        );
      })}
    </nav>
  );
}

/** Queue counts arrive as a promise so the shell renders before the API answers. */
export function AppSidebar({ counts }: { counts: Promise<NavCounts> }) {
  const pathname = usePathname();
  const user = USERS.find((u) => pathname.startsWith(u.prefix)) ?? USERS.at(-1)!;

  return (
    <>
      <aside aria-label="Wasl" className="sticky top-0 hidden h-dvh w-sidebar flex-none flex-col px-3 pb-5 pt-7 md:flex">
        <Brand />
        <div className="mt-7">
          <Nav pathname={pathname} counts={counts} />
        </div>
        <div className="mt-auto flex flex-col gap-3 px-2.5">
          <div className="flex items-center gap-2.5">
            <span aria-hidden className="flex size-7 flex-none items-center justify-center rounded-full bg-active text-small font-medium text-ink-soft">
              {user.initials}
            </span>
            <span className="flex flex-col">
              <span className="text-body font-medium leading-[18px]">{user.name}</span>
              <span className="text-small text-muted">{user.role}</span>
            </span>
          </div>
          <HonestyLabel kind="simulated">Synthetic data · demo</HonestyLabel>
        </div>
      </aside>

      <header className="flex h-12 items-center justify-between border-b border-line px-4 md:hidden">
        <Brand />
        <details className="relative">
          <summary className="flex h-9 cursor-pointer list-none items-center rounded-control px-3 hover:bg-active">Menu</summary>
          <div className="absolute right-0 top-11 z-20 flex w-60 flex-col gap-3 rounded-card border border-line bg-surface p-3">
            <Nav pathname={pathname} counts={counts} />
            <HonestyLabel kind="simulated">Synthetic data · demo</HonestyLabel>
          </div>
        </details>
      </header>
    </>
  );
}
