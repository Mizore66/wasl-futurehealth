import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { expect, it, vi } from "vitest";
import { ScopeMenus } from "@/components/worklist/scope-menus";

const live = vi.hoisted(() => ({ search: "" }));
vi.mock("next/navigation", () => ({
  useSearchParams: () => new URLSearchParams(live.search),
  useRouter: () => ({ push: () => {} }),
}));

const menus = [
  {
    label: "Programme",
    value: "crc",
    options: [{ value: "crc", label: "Colorectal screening", count: 3, href: "/app/today?programme=crc&provider=&queue=call" }],
    clearHref: "/app/today?programme=&provider=&queue=call",
  },
];
const render = () => renderToStaticMarkup(createElement(ScopeMenus, { menus, clearHref: "/app/today?programme=&provider=&queue=call", total: 3 }));

it("re-points the server's links at the queue the sidebar switched to in the browser", () => {
  live.search = "programme=crc&provider=&queue=waiting";
  expect(render()).toContain('href="/app/today?programme=&amp;provider=&amp;queue=waiting"');
});

it("keeps the server's queue when the live URL names none or an unknown one", () => {
  for (const search of ["", "queue=everything"]) {
    live.search = search;
    expect(render()).toContain('href="/app/today?programme=&amp;provider=&amp;queue=call"');
  }
});
