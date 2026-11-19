"use client";

import Link from "next/link";
import { ArrowLeft } from "lucide-react";
import { useEffect, useState } from "react";

/** Where the Referral calendar was when a folder was opened: written while a folder is open, read by the record page. */
export const CALENDAR_KEY = "wasl:patients-files";
type Remembered = { open: string; files: string };

/** Remembers the open folder's case files view, so the record page (after the Audit trail) can return to it. */
export function RememberFiles({ open, files }: { open: string; files: string }) {
  useEffect(() => {
    try {
      sessionStorage.setItem(CALENDAR_KEY, JSON.stringify({ open, files } satisfies Remembered));
    } catch {}
  }, [open, files]);
  return null;
}

/** The folder tab above an open Evidence File. */
export function FolderTab({ href }: { href: string }) {
  return (
    <Link href={href} className="pf-return-tab" data-testid="back-to-files">
      <ArrowLeft aria-hidden="true" />
      Back to files
    </Link>
  );
}

/**
 * On /app/patients/[id]: back to the case files this record was opened from (search and filters kept), or, when it was
 * opened some other way, to the files of the date it was referred.
 */
export function BackToFiles({ id, date }: { id: string; date: string }) {
  const [href, setHref] = useState(`/app/patients?files=${date}`);
  useEffect(() => {
    try {
      const saved = JSON.parse(sessionStorage.getItem(CALENDAR_KEY) ?? "null") as Remembered | null;
      // eslint-disable-next-line react-hooks/set-state-in-effect -- sessionStorage is only readable after hydration
      if (saved?.open === id && saved.files.startsWith("/app/patients")) setHref(saved.files);
    } catch {}
  }, [id]);
  return <FolderTab href={href} />;
}
