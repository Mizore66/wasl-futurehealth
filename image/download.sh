#!/usr/bin/env bash
# Download the six HyperKvasir lower-GI classes used by the image reader (~770 MB) + labels + official split.
# HyperKvasir is CC BY 4.0; its Terms of Use require prior written permission for competition or commercial use.
# Permission was granted for our hackathon submission (see image/DATA.md); other uses need a new request.
# Usage: image/download.sh <data_dir>
set -euo pipefail
D="${1:?data dir}"; mkdir -p "$D/zips" "$D/img"
OSF=https://files.osf.io/v1/resources/mh9sj/providers/osfstorage
declare -a CLS=(cecum:5dfb5b530236b8000c746a8e retroflex-rectum:5dfb5bb70236b8000c746b70 bbps-0-1:5dfb5a890236b8000c7468b9
                bbps-2-3:5dfb5a9cfad251000c6a2e8d polyps:5dfb5ba6fad251000c6a3034 ulcerative-colitis-grade-2:5dfb5c740236b8000c746d21)
for k in "${CLS[@]}"; do
  n=${k%%:*}; id=${k##*:}
  curl -sL --retry 3 -o "$D/zips/$n.zip" "$OSF/$id/?zip="   # HEAD returns 501; GET works
  mkdir -p "$D/img/$n" && unzip -q -o "$D/zips/$n.zip" -d "$D/img/$n"
done
curl -sL -o "$D/image-labels.csv" https://osf.io/download/67y5h/
curl -sL -o "$D/2_fold_split.csv" https://raw.githubusercontent.com/simula/hyper-kvasir/master/official_splits/2_fold_split.csv
(cd "$D" && shasum -a 256 zips/*.zip image-labels.csv 2_fold_split.csv)
rm -rf "$D/zips"
