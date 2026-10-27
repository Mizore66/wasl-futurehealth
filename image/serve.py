"""Live image-reader sidecar (mirrors the llama-server pattern for the text reader).

  python image/serve.py <model_dir> <image_dir> [--port 8792]

model_dir: landmark.onnx/.pt + prep.onnx/.pt + metrics.json from image/train.py (weights are NOT in the repo; see DATA.md)
image_dir: directory tree of JPEGs; images are addressed by sha256 of the file bytes (the pipeline only sees hashes)

POST /v1/image/read {"images": ["<sha256>", ...]} -> {"model_version", "results": [{"sha", "landmark_p", "prep_p"}]}
GET  /health
Unknown sha -> HTTP 404 (the Go client treats any failure as "photo unavailable": fail-safe, never a closure).
"""
import argparse, hashlib, json, os, sys
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import numpy as np
import torch
from PIL import Image
from torchvision import models, transforms

ap = argparse.ArgumentParser()
ap.add_argument("model_dir"); ap.add_argument("image_dir"); ap.add_argument("--port", type=int, default=8792)
a = ap.parse_args()
TF = transforms.Compose([transforms.Resize(256), transforms.CenterCrop(224), transforms.ToTensor(),
                         transforms.Normalize([0.485, 0.456, 0.406], [0.229, 0.224, 0.225])])
M = {}
for task in ("landmark", "prep"):
    ck = torch.load(os.path.join(a.model_dir, f"{task}.pt"), map_location="cpu")
    m = models.mobilenet_v3_small()
    m.classifier[3] = torch.nn.Linear(m.classifier[3].in_features, 1)
    m.load_state_dict(ck["state_dict"]); m.eval()
    M[task] = (m, ck["temperature"])
    VERSION = ck["model_version"]
INDEX = {}
for root, _, fs in os.walk(a.image_dir):
    for f in fs:
        if f.lower().endswith((".jpg", ".jpeg", ".png")):
            p = os.path.join(root, f)
            INDEX[hashlib.sha256(open(p, "rb").read()).hexdigest()] = p
print(f"image reader {VERSION}: {len(INDEX)} images indexed on :{a.port}", file=sys.stderr, flush=True)


@torch.no_grad()
def read(sha):
    im = Image.open(INDEX[sha]).convert("RGB")
    im.thumbnail((320, 320))  # same preprocessing as image/train.py (decode, 320-px thumbnail, resize 256, centre crop 224)
    x = TF(im).unsqueeze(0)
    out = {"sha": sha}
    for task, (m, T) in M.items():
        out[f"{task}_p"] = round(float(1 / (1 + np.exp(-m(x).item() / T))), 4)
    return out


class H(BaseHTTPRequestHandler):
    def _send(self, code, obj):
        b = json.dumps(obj).encode()
        self.send_response(code); self.send_header("Content-Type", "application/json"); self.send_header("Content-Length", str(len(b)))
        self.end_headers(); self.wfile.write(b)

    def do_GET(self):
        self._send(200, {"status": "ok", "model_version": VERSION, "images": len(INDEX)}) if self.path == "/health" else self._send(404, {})

    def do_POST(self):
        if self.path != "/v1/image/read":
            return self._send(404, {})
        try:
            req = json.loads(self.rfile.read(int(self.headers.get("Content-Length", 0))))
            shas = req["images"]
            if any(s not in INDEX for s in shas):
                return self._send(404, {"error": "unknown image"})
            self._send(200, {"model_version": VERSION, "results": [read(s) for s in shas]})
        except Exception as e:  # malformed request
            self._send(400, {"error": str(e)})

    def log_message(self, *args):
        pass


ThreadingHTTPServer(("127.0.0.1", a.port), H).serve_forever()
