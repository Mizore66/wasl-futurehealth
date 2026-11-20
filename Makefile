# `make verify`: check the frozen evaluation (hashes, exact recompute, safety tests); about 1-2 minutes, no Docker.
# `make verify SEED=<any integer>`: also run a fresh held-out cohort end to end with the frozen code (5-10 minutes).
SEED ?=

.PHONY: verify
verify:
	@scripts/verify.sh $(if $(SEED),--seed $(SEED))
