import { Noto_Nastaliq_Urdu } from "next/font/google";

const nastaliq = Noto_Nastaliq_Urdu({ variable: "--font-nastaliq", subsets: ["arabic"], weight: ["400", "600"] });

export default function PatientLayout({ children }: LayoutProps<"/patient">) {
  return <div className={nastaliq.variable}>{children}</div>;
}
