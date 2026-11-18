"use client";

import { createContext, useContext, type ReactNode } from "react";
import { dictFor, PATIENT_DICTS, type DictSet, type Lang, type PatientDict } from "./index";

const DictsContext = createContext<DictSet>(PATIENT_DICTS);

/** The dictionaries a phone surface may render; patient routes pass English only. */
export function DictsProvider({ dicts, children }: { dicts: DictSet; children?: ReactNode }) {
  return <DictsContext.Provider value={dicts}>{children}</DictsContext.Provider>;
}

export function useDicts(): DictSet {
  return useContext(DictsContext);
}

export function usePatientDict(lang: Lang): PatientDict {
  return dictFor(useContext(DictsContext), lang);
}
