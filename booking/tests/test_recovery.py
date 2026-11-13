"""Focused deterministic tests for the no-show recovery input projection."""
import contextlib, hashlib, io, json, os, sys, tempfile, unittest

sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
import recovery, suggest


class RecoveryTests(unittest.TestCase):
    def run_dir(self, root):
        run = os.path.join(root, "run")
        os.mkdir(run)
        specs = [
            ("ahs-mushrif/ServiceRequest/sr-source", "crc", "no_show", 10, 20),
            ("ahs-mushrif/ServiceRequest/sr-open", "crc", "open", 11, 20),
            ("ahs-mushrif/ServiceRequest/sr-late", "crc", "no_show", 12, 20),
            ("medlab/ServiceRequest/sr-cervical", "cervical", "no_show", 13, 20),
            ("medlab/ServiceRequest/sr-closed", "cervical", "closed", 14, 20),
        ]
        with open(os.path.join(run, "out_log.ndjson"), "w") as f:
            for case, prog, state, now, sr_day in specs:
                dec = {"loop": case, "prog": prog, "state": state, "now": now, "sr_day": sr_day,
                       "prov": case.split("/")[0], "sig": "sig-" + case.rsplit("/", 1)[1], "deadline": 31}
                f.write(json.dumps({"data": {"dec": dec}}) + "\n")
        with open(os.path.join(run, "fhir_log.ndjson"), "w") as f:
            for i, (case, prog, state, now, sr_day) in enumerate(specs):
                provider, _, sid = case.partition("/ServiceRequest/")
                pt = {"gender": "female", "communication": [{"language": {"coding": [{"code": "ar"}]}}]}
                f.write(json.dumps({"subj": f"fhir.{provider}.res", "data": {"resourceType": "ServiceRequest", "id": sid, "contained": [pt]}}) + "\n")
                if state == "no_show":
                    for version, status in [(1, "booked"), (2, "noshow")]:
                        appt = {"resourceType": "Appointment", "id": "ap-" + sid, "_v": version, "status": status,
                                "start": f"day:{now}", "basedOn": [{"reference": "ServiceRequest/" + sid}]}
                        f.write(json.dumps({"subj": f"fhir.{provider}.res", "data": appt}) + "\n")
        with contextlib.redirect_stdout(io.StringIO()):
            suggest.main([run], quiet=True)
        return run

    def test_deterministic_projection_and_safety(self):
        with tempfile.TemporaryDirectory() as root:
            run = self.run_dir(root)
            with contextlib.redirect_stdout(io.StringIO()):
                recovery.main([run])
            first = open(os.path.join(run, "recovery.json"), "rb").read()
            with contextlib.redirect_stdout(io.StringIO()):
                recovery.main([run])
            self.assertEqual(first, open(os.path.join(run, "recovery.json"), "rb").read())
            result = json.loads(first)
            self.assertEqual(result["schema"], "wasl-recovery-input/1")
            self.assertEqual(result["as_of_day"], 14)
            self.assertEqual(len(result["waitlist"]), 4)  # closed cases have no booking input
            self.assertGreaterEqual(len(result["slots"]), 2)
            for slot in result["slots"]:
                self.assertTrue(slot["regular"])
                self.assertFalse(slot["overbooked"])
                self.assertGreater(slot["day"], result["as_of_day"])
                self.assertEqual(len(slot["base_head"]), 64)
                self.assertNotEqual(slot["session_id"], slot["source_appointment"])
                self.assertEqual(slot["projection_basis"], "no_show_plus_future_synthetic_capacity_window")
                self.assertTrue(all(s["slot_id"] != slot["slot_id"] or s["source_case"] == slot["source_case"] for s in result["slots"]))
            booking_hash = hashlib.sha256(open(os.path.join(run, "booking.json"), "rb").read()).hexdigest()
            with contextlib.redirect_stdout(io.StringIO()):
                suggest.main([run])
            self.assertEqual(booking_hash, hashlib.sha256(open(os.path.join(run, "booking.json"), "rb").read()).hexdigest())

    def test_later_appointment_state_stales_release(self):
        with tempfile.TemporaryDirectory() as root:
            run = self.run_dir(root)
            with open(os.path.join(run, "fhir_log.ndjson"), "a") as f:
                appt = {"resourceType": "Appointment", "id": "ap-sr-source", "_v": 3, "status": "fulfilled",
                        "basedOn": [{"reference": "ServiceRequest/sr-source"}]}
                f.write(json.dumps({"subj": "fhir.ahs-mushrif.res", "data": appt}) + "\n")
            result = recovery.generate(run)
            self.assertFalse(any(s["source_case"].endswith("sr-source") for s in result["slots"]))

    def test_unsupported_programme_never_projects(self):
        with tempfile.TemporaryDirectory() as root:
            run = self.run_dir(root)
            lines = open(os.path.join(run, "out_log.ndjson")).read().replace('"prog":"crc"', '"prog":"lung_nodule"')
            open(os.path.join(run, "out_log.ndjson"), "w").write(lines)
            # Booking remains the explicit capacity input; unsupported cases cannot create releases.
            result = recovery.generate(run)
            self.assertFalse(any(s["programme"] == "lung_nodule" for s in result["slots"]))


if __name__ == "__main__":
    unittest.main()
