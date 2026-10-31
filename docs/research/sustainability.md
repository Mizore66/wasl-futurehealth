# Sustainability impact (DoH AI Policy 3.1.8)

An upper-bound estimate of Wasl's compute energy and emissions. It is an estimate from the measured GPU time, not a metered report.

| Step | Value | Source |
|---|---|---|
| GPU time | 0.74 H100-hours per 1,000 loops (both readers, parsers and the encoder, at the measured cache-miss rate) | [`v7-gate-offline-results.md`](v7-gate-offline-results.md) |
| GPU power | 700 W, the H100 SXM maximum (the PCIe/NVL card is 350–400 W), so this is an upper bound | [NVIDIA H100 specifications](https://www.nvidia.com/en-us/data-center/h100/) |
| GPU energy | 0.74 h × 0.7 kW = **0.52 kWh per 1,000 loops** | – |
| Facility overhead | × 1.4 (ASSUMPTION: power usage effectiveness of a hot-climate data centre; host CPU and storage are folded in) | assumption |
| Total energy | **≈ 0.73 kWh per 1,000 loops** | – |
| Grid intensity | 0.468 kg CO₂/kWh, UAE 2024 (68% gas, 23% nuclear, 8.6% solar); the national target is 0.27 by 2030 | [Global Grid (Ember data)](https://globalgrid.uk/country/united-arab-emirates); [UAE Energy Strategy 2050](https://u.ae/en/about-the-uae/strategies-initiatives-and-awards/strategies-plans-and-visions/environment-and-energy/uae-energy-strategy-2050) |
| Emissions | **≈ 0.34 kg CO₂ per 1,000 loops** | – |

**At emirate scale:** 50–110k loops a year (all loop types, [`WRITEUP.md`](../WRITEUP.md)) is about 37–80 kWh and 17–38 kg CO₂ a year. That is small next to the clinical activity it tracks: one US endoscopy unit (about 40 procedures a day) measured 111.55 kWh of electricity in a single day, more than the pipeline would use in a year (Gayam 2020, as tabulated in the [ESGE 2022 guideline](https://www.esge.com/assets/downloads/pdfs/guidelines/2022_a-1859-3726.pdf) and the [2025 systematic review in *Endoscopy*](https://pmc.ncbi.nlm.nih.gov/articles/PMC13305580/)). The pipeline itself (NATS, linker, closure) runs on CPU at about 750 events per second on one laptop, so its share is smaller still.

**Levers already in the design:** cached reader answers (a report body is read once, not once per loop that cites it), the local 4B option for the colonoscopy reader only (not for teach-back, which it fails; see `pipeline/RESULTS.md`), and hosting in a UAE region so the grid factor falls with the 2030 target.

**Limits:** the GPU time is measured on the synthetic cohort's report mix; real reports may be longer. The facility factor is assumed. No embodied (manufacturing) emissions are counted.
