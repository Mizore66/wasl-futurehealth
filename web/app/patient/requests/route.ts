import { proxyPatientRights } from "@/lib/api/patient-proxy";

export function GET(request: Request) {
  return proxyPatientRights(request, "/api/patient/requests");
}

export function POST(request: Request) {
  return proxyPatientRights(request, "/api/patient/requests");
}
