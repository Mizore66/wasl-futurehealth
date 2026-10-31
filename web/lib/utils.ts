import { createCn } from "cn/config";

/** Class merging that knows the ledger's font-size tokens, so `text-small` and `text-muted` do not collide. */
export const cn = createCn({
  extend: { classGroups: { "font-size": [{ text: ["title", "section", "lead", "body", "small"] }] } },
});
