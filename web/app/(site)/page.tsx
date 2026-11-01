import { SiteFooter } from "@/components/site/footer";
import { missedOfFive, published } from "@/components/site/figures";
import { Story } from "@/components/story/story";
import { result } from "@/lib/results";

export const metadata = { title: { absolute: "Wasl · Nobody lost between hospitals" } };

export default function Page() {
  const closures = result("v2.safety.crc_false_closures");
  const wait = published("crc_followup_median_wait_days");
  return (
    <>
      <Story
        figures={{
          missedOfFive: missedOfFive(),
          waitDays: wait,
          standardDays: published("crc_followup_standard_days"),
          closuresDone: closures.denominator!,
          closuresWrong: closures.value,
        }}
      />
      <div id="notes">
        <SiteFooter
          big
          honesty="simulated"
          qualifier="Fatima and every other patient shown here are made up."
          rest={`Test results come from simulated patients; Wasl claims no results on real patients yet. The ${wait}-day figure is from a published Abu Dhabi study (BMC Cancer, 2021).`}
        />
      </div>
    </>
  );
}
