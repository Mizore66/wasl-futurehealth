/** Loop ids contain slashes (`ahs-mushrif/ServiceRequest/AHS-sr-1007`); route segments carry them encoded. */
export function caseHref(base: "/app/patients" | "/app/review", id: string): string {
  return `${base}/${encodeURIComponent(id)}`;
}

export function caseIdFromParam(param: string): string {
  return param.includes("%") ? decodeURIComponent(param) : param;
}

/** The short, human-facing part of a loop id: `AHS-sr-1007`. */
export function shortId(id: string): string {
  return id.slice(id.lastIndexOf("/") + 1);
}
