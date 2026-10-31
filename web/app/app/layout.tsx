import { AppSidebar, type NavCounts } from "@/components/shell/app-sidebar";
import { getDashboard } from "@/lib/api/source";

async function navCounts(): Promise<NavCounts> {
  const open = (await getDashboard()).review.open;
  return { review: typeof open === "number" ? open : undefined };
}

export default function AppLayout({ children }: LayoutProps<"/app">) {
  return (
    <div className="flex min-h-dvh flex-col bg-ground md:flex-row">
      <AppSidebar counts={navCounts()} />
      <main className="min-w-0 flex-1 px-6 py-8">{children}</main>
    </div>
  );
}
