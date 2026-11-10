"""Select the shared golden only on platforms verified against deterministic numerics."""
import platform


def output_name(system=None, machine=None):
    system = platform.system() if system is None else system
    machine = platform.machine() if machine is None else machine
    if (system, machine) in {("Linux", "x86_64"), ("Linux", "aarch64"), ("Darwin", "arm64")}:
        return "pool_test.json"
    raise RuntimeError(f"no deterministic booking-simulation golden for {system}/{machine}")
