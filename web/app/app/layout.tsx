import { Suspense } from "react";
import { cookies } from "next/headers";
import { AppSidebar, type NavCounts } from "@/components/shell/app-sidebar";
import { SIDEBAR_COOKIE } from "@/components/shell/sidebar-cookie";
import { SkipLink } from "@/components/shell/skip-link";
import { TopBar } from "@/components/shell/top-bar";
import { todayQueueCounts } from "@/components/worklist/today-items";
import { RunMetaScope } from "@/components/shell/run-meta-scope";
import { RunMeta, RunMetaFallback } from "@/components/shell/run-meta";
import { getCases } from "@/lib/api/source";
import { reviewerPersona } from "@/lib/api/reviewer-session";

/** The Review queue is CRC only, so the badge counts open CRC reviews, not the dashboard's all-programme total. */
async function navCounts(): Promise<NavCounts> {
  const [{ total }, today] = await Promise.all([
    getCases({ review: "open", programme: "crc", limit: 1 }),
    todayQueueCounts().catch(() => undefined),
  ]);
  return { review: total > 0 ? total : undefined, today };
}

export default async function AppLayout({ children }: LayoutProps<"/app">) {
  // Read on the server so the first paint already has the right sidebar width.
  const collapsed = (await cookies()).get(SIDEBAR_COOKIE)?.value === "collapsed";

  return (
    <div className="staff flex min-h-dvh flex-col md:flex-row">
      <SkipLink />
      <AppSidebar counts={navCounts()} session={reviewerPersona()} defaultCollapsed={collapsed} />
      <div className="flex min-w-0 flex-1 flex-col">
        <TopBar />
        <main id="main" tabIndex={-1} className="min-w-0 flex-1 px-[clamp(12px,2.4vw,32px)] py-[clamp(12px,2vw,24px)] focus:outline-none">
          <RunMetaScope>
            <Suspense fallback={<RunMetaFallback />}>
              <RunMeta />
            </Suspense>
          </RunMetaScope>
          {children}
        </main>
      </div>
    </div>
  );
}
