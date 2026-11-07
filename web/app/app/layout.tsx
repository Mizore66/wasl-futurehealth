import { Suspense } from "react";
import { cookies } from "next/headers";
import { AppSidebar, type NavCounts } from "@/components/shell/app-sidebar";
import { SIDEBAR_COOKIE } from "@/components/shell/sidebar-cookie";
import { SkipLink } from "@/components/shell/skip-link";
import { RunMeta, RunMetaFallback } from "@/components/shell/run-meta";
import { getCases } from "@/lib/api/source";

/** The Review queue is CRC only, so the badge counts open CRC reviews, not the dashboard's all-programme total. */
async function navCounts(): Promise<NavCounts> {
  const { total } = await getCases({ review: "open", programme: "crc", limit: 1 });
  return { review: total > 0 ? total : undefined };
}

export default async function AppLayout({ children }: LayoutProps<"/app">) {
  // Read on the server so the first paint already has the right sidebar width.
  const collapsed = (await cookies()).get(SIDEBAR_COOKIE)?.value === "collapsed";

  return (
    <div className="flex min-h-dvh flex-col bg-ground md:flex-row">
      <SkipLink />
      <AppSidebar counts={navCounts()} defaultCollapsed={collapsed} />
      <main id="main" tabIndex={-1} className="min-w-0 flex-1 px-6 py-8 focus:outline-none">
        <Suspense fallback={<RunMetaFallback />}>
          <RunMeta />
        </Suspense>
        {children}
      </main>
    </div>
  );
}
