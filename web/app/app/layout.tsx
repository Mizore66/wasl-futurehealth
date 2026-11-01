import { AppSidebar, type NavCounts } from "@/components/shell/app-sidebar";
import { getCases } from "@/lib/api/source";

/** The Review queue is CRC only, so the badge counts open CRC reviews, not the dashboard's all-programme total. */
async function navCounts(): Promise<NavCounts> {
  const { total } = await getCases({ review: "open", programme: "crc", limit: 1 });
  return { review: total > 0 ? total : undefined };
}

export default function AppLayout({ children }: LayoutProps<"/app">) {
  return (
    <div className="flex min-h-dvh flex-col bg-ground md:flex-row">
      <AppSidebar counts={navCounts()} />
      <main className="min-w-0 flex-1 px-6 py-8">{children}</main>
    </div>
  );
}
