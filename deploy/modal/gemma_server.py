"""Serve the on-device reader model, gemma-3-4b-it-Q4_K_M.gguf, on a Modal GPU for the hosted demo.

The file and the llama-server flags are the ones the repository's local reader uses (README, "Live local reader"), so
the hosted demo runs the same bytes a hospital would run on its own machine. Modal stands in for that machine; only
synthetic data is sent to it. The OpenAI-compatible API needs the key in the Modal secret `wasl-gemma-key`.

  python -m modal run    deploy/modal/gemma_server.py::download   # once: fetch and verify the weights
  python -m modal deploy deploy/modal/gemma_server.py             # WASL_GEMMA_WARM=1 keeps one GPU container up
"""
import os
import subprocess

import modal

MODEL_REPO = "ggml-org/gemma-3-4b-it-GGUF"
MODEL_REVISION = "d0976223747697cb51e056d85c532013931fe52e"
MODEL_FILE = "gemma-3-4b-it-Q4_K_M.gguf"
MODEL_SHA256 = "882e8d2db44dc554fb0ea5077cb7e4bc49e7342a1f0da57901c0802ea21a0863"
MODEL_ALIAS = "gemma-3-4b-it-Q4_K_M"
MODEL_DIR = "/models"
PORT = 8080

image = (
    modal.Image.from_registry("ghcr.io/ggml-org/llama.cpp:server-cuda12-b12030", add_python="3.12")
    .entrypoint([])
)
weights = modal.Volume.from_name("wasl-gemma-weights", create_if_missing=True)
app = modal.App("wasl-gemma")


@app.function(image=image, volumes={MODEL_DIR: weights}, timeout=30 * 60)
def download():
    import hashlib
    import urllib.request

    path = f"{MODEL_DIR}/{MODEL_FILE}"
    url = f"https://huggingface.co/{MODEL_REPO}/resolve/{MODEL_REVISION}/{MODEL_FILE}"
    digest = hashlib.sha256()
    with urllib.request.urlopen(url) as src, open(path + ".part", "wb") as dst:
        while chunk := src.read(1 << 20):
            digest.update(chunk)
            dst.write(chunk)
    if digest.hexdigest() != MODEL_SHA256:
        os.remove(path + ".part")
        raise RuntimeError(f"sha256 mismatch: {digest.hexdigest()}")
    os.replace(path + ".part", path)
    weights.commit()
    print(f"{MODEL_FILE} verified: {MODEL_SHA256}")


@app.function(
    image=image,
    gpu="T4",
    volumes={MODEL_DIR: weights},
    secrets=[modal.Secret.from_name("wasl-gemma-key")],
    min_containers=int(os.environ.get("WASL_GEMMA_WARM", "0")),
    scaledown_window=15 * 60,
    timeout=24 * 60 * 60,
)
@modal.concurrent(max_inputs=4)
@modal.web_server(port=PORT, startup_timeout=5 * 60)
def serve():
    subprocess.Popen([
        "/app/llama-server", "-m", f"{MODEL_DIR}/{MODEL_FILE}", "--alias", MODEL_ALIAS,
        "--host", "0.0.0.0", "--port", str(PORT),
        "-c", "8192", "-ngl", "99", "-np", "4",
        "--api-key", os.environ["LLAMA_API_KEY"],
    ])
    # llama-server opens its port before the weights are loaded and answers 503 until then; Modal starts routing when
    # this function returns, so hold it until /health reports ready and cold requests wait instead of failing.
    import time
    import urllib.request

    deadline = time.monotonic() + 4 * 60
    while time.monotonic() < deadline:
        try:
            with urllib.request.urlopen(f"http://127.0.0.1:{PORT}/health", timeout=5) as r:
                if r.status == 200:
                    return
        except OSError:
            pass
        time.sleep(1)
    raise RuntimeError("llama-server did not become healthy")
