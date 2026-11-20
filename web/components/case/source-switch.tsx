"use client";

import { FileText } from "lucide-react";
import { Children, useState } from "react";
import { SegmentedToggle } from "@/components/ui/segmented-toggle";

/**
 * The Evidence File's source documents, one at a time: a single-select switch above the document. The server renders
 * every document; without script the first stays shown, so the latest report is always readable.
 */
export function SourceSwitch({ labels, children }: { labels: string[]; children: React.ReactNode }) {
  const [shown, setShown] = useState(0);
  const docs = Children.toArray(children);
  return (
    <>
      {labels.length > 1 && (
        <SegmentedToggle
          label="Choose a source document"
          value={String(shown)}
          onChange={(i) => setShown(Number(i))}
          items={labels.map((label, i) => ({
            value: String(i),
            label: (
              <>
                <FileText aria-hidden="true" className="size-[15px]" />
                {label}
              </>
            ),
          }))}
        />
      )}
      {docs.map((doc, i) => (
        <div key={i} hidden={i !== shown} className="rs-document-slot">
          {doc}
        </div>
      ))}
    </>
  );
}
