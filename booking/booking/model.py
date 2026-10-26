"""Data types and the no-show model shared by the booking engine and the simulator.

Interface contract (what the Wasl pipeline would send / receive):
  Event in   : {"kind": "result_accepted"|"no_show"|"bumped"|"cancel", "loop_id", "day", "prefs"?, "deadline"?}
  Booking out: {"loop_id", "session_id", "unit", "day", "endoscopist_female", "overbooked",
                "predicted_no_show", "nurse_call", "prep_send_day", "deadline", "within_deadline", "booked_on"}
"""
from __future__ import annotations
from dataclasses import dataclass, asdict, field
import math
import numpy as np

REGIONS = ["AD", "AA", "DH"]  # Abu Dhabi city, Al Ain, Al Dhafra


@dataclass
class Prefs:
    female_endoscopist: str = "none"  # none | soft | hard
    arabic_only: bool = False
    region: str = "AD"
    travel_ok: bool = False
    after_hours: bool = False  # needs a weekend / after-hours list
    avoid_ramadan: bool = False
    prior_no_show: bool = False


@dataclass
class Event:
    kind: str
    loop_id: str
    day: int
    prefs: Prefs | None = None
    deadline: int | None = None

    @staticmethod
    def from_json(d: dict) -> "Event":
        p = d.get("prefs")
        return Event(d["kind"], d["loop_id"], int(d["day"]), Prefs(**p) if p else None, d.get("deadline"))


@dataclass
class Session:
    sid: int
    unit: str
    region: str
    day: int
    cap: int
    female: bool
    arabic: bool
    after_hours: bool
    ramadan: bool


@dataclass
class Booking:
    loop_id: str
    session_id: int
    unit: str
    day: int
    endoscopist_female: bool
    overbooked: bool
    predicted_no_show: float
    nurse_call: bool
    prep_send_day: int
    deadline: int
    within_deadline: bool
    booked_on: int

    def to_json(self) -> dict:
        return asdict(self)

    def to_fhir_appointment(self) -> dict:
        """Minimal FHIR R4 Appointment the Wasl pipeline can ingest as a downstream event."""
        return {"resourceType": "Appointment", "id": f"{self.loop_id}-{self.session_id}",
                "status": "booked", "serviceType": [{"text": "Colonoscopy (FIT+ follow-up)"}],
                "basedOn": [{"reference": f"ServiceRequest/{self.loop_id}"}],
                "start": f"day:{self.day}", "comment": "overbooked" if self.overbooked else "",
                "extension": [{"url": "wasl:deadline-day", "valueInteger": self.deadline},
                              {"url": "wasl:p-no-show", "valueDecimal": round(self.predicted_no_show, 3)}]}


def logit(p):
    return math.log(p / (1 - p))


@dataclass
class NoShowModel:
    """logit p = logit(p0) + ln(OR_lead)*(lead-14) + ln(OR_prior)*prior + ln(OR_soft)*female_mismatch
                 + ln(OR_lang)*lang_mismatch + ln(OR_hours)*hours_mismatch + ln(OR_sms)*sms + ln(OR_nurse)*nurse_call
       Ramadan: a fasting patient booked inside Ramadan -> p = max(p, p_ramadan).
    Default (engine) coefficients = published point estimates; the simulator draws the 'true' ones."""
    p0: float = 0.174          # SEHA AHS: 92 scheduled-not-done / 528 scheduled (PMC8343888, Table 5); at lead 14 d, SMS on
    or_lead: float = 1.006     # per day of lead time (Yilmaz & Kocyigit, PeerJ 2022, n=5,938)
    or_prior: float = 3.8      # prior no-show (endoscopy no-show cohorts: 3.8 [1.7-8.5]; 6.4 [2.4-17.5])
    or_soft: float = 1.5       # assumed
    or_lang: float = 1.2       # assumed
    or_hours: float = 1.5      # assumed
    or_sms: float = 0.70       # automated reminder vs none (Parikh, Am J Med 2010: 17.3% vs 23.1%). Reference level
                               # sms=1 (all arms assumed to send SMS), so it only matters if a reminder is switched off
    or_nurse: float = 0.67     # nurse call 7 d before (GIE 2016: OR 0.67 [0.50-0.91])
    p_ramadan: float = 0.5     # assumed
    or_travel: float = 1.2     # assumed (booked outside home region)

    def p(self, lead, prior, fem_mm, lang_mm, hours_mm, sms, nurse, ram_conflict, travel_mm=0):
        lead = np.asarray(lead, dtype=float)
        z = (logit(self.p0) + math.log(self.or_lead) * (lead - 14) + math.log(self.or_prior) * prior
             + math.log(self.or_soft) * fem_mm + math.log(self.or_lang) * lang_mm
             + math.log(self.or_hours) * hours_mm + math.log(self.or_sms) * sms + math.log(self.or_nurse) * nurse
             + math.log(self.or_travel) * travel_mm)
        p = 1 / (1 + np.exp(-z))
        return np.where(ram_conflict, np.maximum(p, self.p_ramadan), p)
