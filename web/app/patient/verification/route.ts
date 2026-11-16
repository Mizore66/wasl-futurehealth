import { proxyPatientRights } from "@/lib/api/patient-proxy";

export function POST(request: Request) {
  return proxyPatientRights(request, "/api/patient/verification");
}
