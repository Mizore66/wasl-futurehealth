/** Concise text for the screen-reader status region after the user presses Step. */
export function stepAnnouncement(view: { step: number; steps: number; feed: { step: number; who: { name: string }; text: string }[] }): string {
  const newest = view.feed[0];
  const where = `Step ${view.step} of ${view.steps}.`;
  return newest ? `${where} Newest decision: ${newest.who.name}, ${newest.text}.` : `${where} No decision recorded yet.`;
}
