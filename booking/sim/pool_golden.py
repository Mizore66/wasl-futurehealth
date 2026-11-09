"""Select only an exact booking-simulation golden verified on the current numerical platform."""
import platform


def output_name(system=None, machine=None):
    system = platform.system() if system is None else system
    machine = platform.machine() if machine is None else machine
    if (system, machine) == ("Linux", "x86_64"):
        return "pool_test.linux-amd64.json"
    if (system, machine) == ("Darwin", "arm64"):
        return "pool_test.json"
    raise RuntimeError(f"no exact booking-simulation golden for {system}/{machine}")
