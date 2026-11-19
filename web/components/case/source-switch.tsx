"use client";

import { FileText } from "lucide-react";
import { Children, useState } from "react";

/**
 * The Evidence File's source documents, one at a time: a pressed-button switch above the document. The server renders
 * every document; without script the first stays shown, so the latest report is always readable.
 */
export function SourceSwitch({ labels, children }: { labels: string[]; children: React.ReactNode }) {
  const [shown, setShown] = useState(0);
  const docs = Children.toArray(children);
  return (
    <>
      {labels.length > 1 && (
        <div className="rs-source-buttons" role="group" aria-label="Choose a source document">
          {labels.map((label, i) => (
            <button key={label} type="button" aria-pressed={i === shown} onClick={() => setShown(i)}>
              <FileText aria-hidden="true" />
              {label}
            </button>
          ))}
        </div>
      )}
      {docs.map((doc, i) => (
        <div key={i} hidden={i !== shown} className="rs-document-slot">
          {doc}
        </div>
      ))}
    </>
  );
}
