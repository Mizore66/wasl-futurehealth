/**
 * The guided tour, page by page. `target` is a CSS selector; `page` pins a step to one route (omit it for the shell,
 * which is on every page); `goTo` makes it a click step that finishes when the user lands on that route.
 */
export type TourStep = {
  section: string;
  target: string;
  title: string;
  body: string;
  page?: string;
  goTo?: string;
};

const sideLink = (href: string) => `#app-sidebar a[href="${href}"]`;

export const TOUR_STEPS: TourStep[] = [
  {
    section: "Start",
    target: '[data-tour="workspace"]',
    title: "Where your cases live",
    body: "Today is your worklist for the shift. Patients, Referrals and Recovery hold every case you follow up. We'll visit the four screens you'll use most.",
  },
  {
    section: "Today",
    target: sideLink("/app/today"),
    goTo: "/app/today",
    title: "Open Today",
    body: "Start where your shift starts: the list of people to contact.",
  },
  {
    section: "Today",
    page: "/app/today",
    target: '[data-tour="workspace"] ul',
    title: "Three queues",
    body: "To call: patients you need to reach now. Waiting: cases with someone else, such as pathology, the doctors or a report still to be matched. No owner: cases nobody has picked up yet.",
  },
  {
    section: "Today",
    page: "/app/today",
    target: '[data-testid="today-sheet"]',
    title: "Your worklist",
    body: "One row per task: the patient, what needs doing next, and how far past the deadline it is. Pick a row to work on it.",
  },
  {
    section: "Today",
    page: "/app/today",
    target: 'section[aria-label="Selected case"]',
    title: "The case you picked",
    body: "Who the patient is, how to reach them and what has happened so far. Log the call here once you've made it, and it moves to the right queue.",
  },
  {
    section: "Patients",
    target: sideLink("/app/patients"),
    goTo: "/app/patients",
    title: "Open Patients",
    body: "Every case in the programme, not just today's.",
  },
  {
    section: "Patients",
    page: "/app/patients",
    target: 'form[role="search"]',
    title: "Find any case",
    body: "Search by case, task or owner. The menus beside it narrow the list by programme, provider, consent, review state and owner.",
  },
  {
    section: "Patients",
    page: "/app/patients",
    target: "#pf-results",
    title: "Referrals by date",
    body: "Cases are laid out by the day their referral arrived, so a late one stands out. Open a case for its full file.",
  },
  {
    section: "Review desk",
    target: sideLink("/app/review"),
    goTo: "/app/review",
    title: "Open the Review desk",
    body: "Where Wasl hands a case to a person instead of deciding itself.",
  },
  {
    section: "Review desk",
    page: "/app/review",
    target: 'ol[aria-label="Cases waiting"] > li:first-child',
    title: "A held case",
    body: "Wasl held this case rather than close it on its own. The middle says why. The tags on the right say what needs checking: the patient's identity, a report, or both.",
  },
  {
    section: "Review desk",
    page: "/app/review",
    target: "[data-queue-question]",
    title: "Where each question stands",
    body: "Each open question shows its status, such as Waiting for a first read, and who has claimed it. Open the case and sign in to claim it or answer.",
  },
  {
    section: "Programme",
    target: sideLink("/app"),
    goTo: "/app",
    title: "Open Programme",
    body: "How the whole screening programme is doing.",
  },
  {
    section: "Programme",
    page: "/app",
    target: 'section[aria-labelledby="kpi-h"]',
    title: "The headline number",
    body: "Share of positive FIT referrals that reached a colonoscopy within 31 days, the DoH standard. It is counted three ways: by Emirates ID alone, by Wasl's matching, and against the simulated truth.",
  },
  {
    section: "Programme",
    page: "/app",
    target: '[data-testid="programme-workload"]',
    title: "Where the cases are",
    body: "Every case in the run, split into overdue, other open and closed. Open Today takes you straight to the ones to call.",
  },
  {
    section: "Programme",
    page: "/app",
    target: 'section[aria-labelledby="stall-h"]',
    title: "Where patients stall",
    body: "The three steps where most patients get stuck. Each one links to the patients stuck there.",
  },
  {
    section: "Finish",
    target: '[data-tour="help"]',
    title: "Come back anytime",
    body: "Lost? Click here to replay this tour from any page.",
  },
];
