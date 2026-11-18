import { Pill } from "@/components/review/marks";

/**
 * Where an open case rests and whether it is late, as two marks. The stage is a position in the pipeline and stays
 * neutral; lateness is a separate negative mark, so "Received" is never coloured as an alarm.
 */
export function StageMarks({ stage, overdue }: { stage: string; overdue?: boolean }) {
  return (
    <>
      <Pill tone="muted">{stage}</Pill>
      {overdue && (
        <Pill tone="negative" shape="cross">
          Overdue
        </Pill>
      )}
    </>
  );
}
